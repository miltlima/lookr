package cost

import (
	"context"
	"errors"
	"math"
	"strings"

	"lookr/internal/model"
	"lookr/internal/provider/aws/usage"
)

const (
	HoursPerMonth         = 730.0
	EIPHourlyUSD          = 0.005
	EKSClusterHourlyUSD   = 0.10
	Route53ZoneMonthlyUSD = 0.50
)

// Estimate uses on-demand capacity and usage, excluding discounts and free
// tier. Product filters follow the AWS Price List attribute model:
// https://docs.aws.amazon.com/awsaccountbilling/latest/aboutv2/using-price-list-query-api.html
// https://aws.amazon.com/blogs/aws/new-aws-price-list-api/
// Unit-specific product families/groups keep storage separate from IOPS and
// throughput. Missing usage never becomes a zero-cost usage estimate.
func Estimate(ctx context.Context, r *model.Resource, m usage.Metrics, days int, p PriceLookup) {
	r.Cost = model.Cost{Source: model.CostNone, Note: "not priced"}
	var lookupErr error
	description := ""
	price := func(service, region string, filters map[string]string) float64 {
		if lookupErr != nil {
			return 0
		}
		description = r.Type + " in " + region
		if p == nil {
			lookupErr = errors.New("pricing client unavailable")
			return 0
		}
		v, err := p.Price(ctx, PriceQuery{ServiceCode: service, Region: region, Filters: filters})
		if err != nil {
			lookupErr = err
			return 0
		}
		if math.IsNaN(v) || math.IsInf(v, 0) || v < 0 {
			lookupErr = errors.New("invalid list price")
			return 0
		}
		return v
	}
	region := r.Region
	value := 0.0
	note := "on-demand list price"
	switch r.Type {
	case "aws.ec2.instance":
		if r.Str("state") == "stopped" {
			note = "stopped; attached volumes billed separately"
			break
		}
		os := "Linux"
		if strings.EqualFold(r.Str("platform"), "windows") {
			os = "Windows"
		}
		value = price("AmazonEC2", region, map[string]string{"productFamily": "Compute Instance", "instanceType": r.Str("instance_type"), "operatingSystem": os, "tenancy": "Shared", "preInstalledSw": "NA", "capacitystatus": "Used"}) * HoursPerMonth
	case "aws.ebs.volume":
		value = price("AmazonEC2", region, map[string]string{"productFamily": "Storage", "volumeApiName": r.Str("volume_type")}) * r.Float("size_gb")
		note = "storage only; provisioned IOPS and throughput excluded"
	case "aws.ebs.snapshot":
		value = price("AmazonEC2", region, map[string]string{"productFamily": "Storage Snapshot", "storageMedia": "Amazon S3"}) * r.Float("volume_size_gb")
		note = "upper bound: assumes full volume size"
	case "aws.ec2.eip":
		value = EIPHourlyUSD * HoursPerMonth
	case "aws.ec2.natgateway":
		value = price("AmazonEC2", region, map[string]string{"productFamily": "NAT Gateway", "group": "NGW:NatGateway"}) * HoursPerMonth
		note = "data processing excluded"
	case "aws.elb.loadbalancer":
		family := map[string]string{"application": "Load Balancer-Application", "network": "Load Balancer-Network", "gateway": "Load Balancer-Gateway", "classic": "Load Balancer"}[r.Str("lb_type")]
		if family == "" {
			r.Cost.Note = "no estimate for unknown load balancer type"
			return
		}
		typ := r.Str("lb_type")
		title := strings.ToUpper(typ[:1]) + typ[1:]
		// Verified against the public regional AWSELB catalog: operation differs
		// by type, while this dimension selects hourly usage and excludes LCUs,
		// data processing, Outposts and trust-store charges.
		value = price("AWSELB", region, map[string]string{"productFamily": family, "groupDescription": "LoadBalancer hourly usage by " + title + " Load Balancer"}) * HoursPerMonth
		note = "capacity units excluded"
	case "aws.rds.instance":
		engine := databaseEngine(r.Str("engine"))
		if engine == "" {
			r.Cost.Note = "no estimate for unknown database engine"
			return
		}
		deployment := "Single-AZ"
		if r.Bool("multi_az") {
			deployment = "Multi-AZ"
		}
		filters := map[string]string{"productFamily": "Database Instance", "instanceType": r.Str("instance_class"), "databaseEngine": engine, "deploymentOption": deployment}
		if edition := databaseEdition(r.Str("engine")); edition != "" {
			filters["databaseEdition"] = edition
		}
		stopped := r.Str("status") == "stopped"
		if !stopped {
			value = price("AmazonRDS", region, filters) * HoursPerMonth
		}
		if r.Str("cluster_id") == "" && r.Float("allocated_storage_gb") > 0 {
			storage := map[string]string{"gp2": "General Purpose", "gp3": "General Purpose-GP3", "io1": "Provisioned IOPS", "io2": "Provisioned IOPS-IO2", "standard": "Magnetic"}[r.Str("storage_type")]
			if storage == "" {
				r.Cost.Note = "no estimate for unknown database storage type"
				return
			}
			value += price("AmazonRDS", region, map[string]string{"productFamily": "Database Storage", "databaseEngine": engine, "deploymentOption": deployment, "volumeType": storage}) * r.Float("allocated_storage_gb")
		}
		note = "backups and I/O excluded"
		if stopped {
			note = "stopped; compute excluded until automatic restart (up to 7 days); backups and I/O excluded"
		}
	case "aws.rds.cluster":
		r.Cost.Note = "instances are billed separately; cluster storage not estimated"
		return
	case "aws.elasticache.cluster":
		engine := map[string]string{"redis": "Redis", "memcached": "Memcached", "valkey": "Valkey"}[r.Str("engine")]
		if engine == "" {
			r.Cost.Note = "no estimate for unknown cache engine"
			return
		}
		value = price("AmazonElastiCache", region, map[string]string{"productFamily": "Cache Instance", "instanceType": r.Str("node_type"), "cacheEngine": engine}) * r.Float("num_nodes") * HoursPerMonth
	case "aws.eks.cluster":
		value = EKSClusterHourlyUSD * HoursPerMonth
		note = "control plane only"
	case "aws.lambda.function":
		invocations, hasInvocations := m["invocations"]
		duration, hasDuration := m["duration_ms"]
		if !hasInvocations || !hasDuration || days <= 0 {
			r.Cost.Note = "usage metrics unavailable"
			return
		}
		group := "AWS-Lambda-Duration"
		if r.Str("architecture") == "arm64" {
			group += "-ARM"
		}
		req := price("AWSLambda", region, map[string]string{"group": "AWS-Lambda-Requests"})
		dur := price("AWSLambda", region, map[string]string{"group": group})
		value = (invocations*req + duration/1000*r.Float("memory_mb")/1024*dur) * 30 / float64(days)
		note = "free tier ignored"
	case "aws.dynamodb.table":
		if r.Str("billing_mode") != "PROVISIONED" {
			r.Cost.Note = "no estimate for usage-based pricing"
			return
		}
		read := price("AmazonDynamoDB", region, map[string]string{"group": "DDB-ReadUnits"})
		write := price("AmazonDynamoDB", region, map[string]string{"group": "DDB-WriteUnits"})
		value = (r.Float("read_capacity")*read + r.Float("write_capacity")*write) * HoursPerMonth
		note = "provisioned capacity only; storage excluded"
	case "aws.sqs.queue", "aws.cloudfront.distribution":
		r.Cost.Note = "no estimate for usage-based pricing"
		return
	case "aws.s3.bucket":
		size, ok := m["size_bytes"]
		if !ok {
			r.Cost.Note = "no storage metrics"
			return
		}
		region = r.Str("bucket_region")
		if region == "" {
			r.Cost.Note = "bucket region unavailable"
			return
		}
		value = price("AmazonS3", region, map[string]string{"productFamily": "Storage", "storageClass": "General Purpose", "volumeType": "Standard"}) * size / (1 << 30)
		note = "Standard storage class only"
	case "aws.route53.hostedzone":
		value = Route53ZoneMonthlyUSD
		note = "hosted zone fee only"
	default:
		return
	}
	if lookupErr != nil {
		if errors.Is(lookupErr, ErrNoPrice) {
			r.Cost.Note = "no list price found for " + description
		} else {
			r.Cost.Note = "pricing unavailable: " + lookupErr.Error()
		}
		return
	}
	if math.IsNaN(value) || math.IsInf(value, 0) || value < 0 {
		r.Cost.Note = "invalid cost inputs"
		return
	}
	value = math.Round(value*100) / 100
	r.Cost = model.Cost{MonthlyUSD: &value, Source: model.CostEstimate, Note: note}
}

func databaseEngine(engine string) string {
	switch engine {
	case "postgres":
		return "PostgreSQL"
	case "mysql":
		return "MySQL"
	case "mariadb":
		return "MariaDB"
	case "aurora", "aurora-mysql":
		return "Aurora MySQL"
	case "aurora-postgresql":
		return "Aurora PostgreSQL"
	default:
		if strings.HasPrefix(engine, "sqlserver-") {
			return "SQL Server"
		}
		if strings.HasPrefix(engine, "oracle-") {
			return "Oracle"
		}
		return ""
	}
}

func databaseEdition(engine string) string {
	switch engine {
	case "sqlserver-ee", "oracle-ee", "oracle-ee-cdb":
		return "Enterprise"
	case "sqlserver-se":
		return "Standard"
	case "sqlserver-ex":
		return "Express"
	case "sqlserver-web":
		return "Web"
	case "oracle-se2", "oracle-se2-cdb":
		return "Standard Two"
	case "oracle-se1":
		return "Standard One"
	case "oracle-se":
		return "Standard"
	default:
		return ""
	}
}
