package cost

import (
	"context"
	"errors"
	"math"
	"strings"
	"testing"

	"lookr/internal/model"
	"lookr/internal/provider/aws/usage"
)

type fakePrices struct {
	queries []PriceQuery
	err     error
}

func (p *fakePrices) Price(_ context.Context, q PriceQuery) (float64, error) {
	p.queries = append(p.queries, q)
	if p.err != nil {
		return 0, p.err
	}
	switch q.ServiceCode {
	case "AWSELB":
		return .084, nil
	case "AWSLambda":
		if q.Filters["group"] == "AWS-Lambda-Requests" {
			return 0.0000002, nil
		}
		return 0.0000166667, nil
	case "AmazonEC2":
		if q.Filters["productFamily"] == "Storage" || q.Filters["productFamily"] == "Storage Snapshot" {
			return .08, nil
		}
		return .084, nil
	case "AmazonRDS":
		if q.Filters["productFamily"] == "Database Storage" {
			return .10, nil
		}
		return .2, nil
	case "AmazonElastiCache":
		return .1, nil
	case "AmazonDynamoDB":
		if q.Filters["group"] == "DDB-ReadUnits" {
			return .00013, nil
		}
		return .00065, nil
	case "AmazonS3":
		return .023, nil
	default:
		return 0, ErrNoPrice
	}
}
func TestEstimateTable(t *testing.T) {
	tests := []struct {
		typ  string
		c    map[string]any
		m    usage.Metrics
		days int
		want float64
		note string
		none bool
	}{
		{"ec2.instance", map[string]any{"state": "running", "instance_type": "t3.large"}, nil, 30, 61.32, "on-demand list price", false},
		{"ec2.instance", map[string]any{"state": "stopped"}, nil, 30, 0, "stopped; attached volumes billed separately", false},
		{"ebs.volume", map[string]any{"size_gb": 1000, "volume_type": "gp3"}, nil, 30, 80, "storage only; provisioned IOPS and throughput excluded", false},
		{"ebs.snapshot", map[string]any{"volume_size_gb": 100}, nil, 30, 8, "upper bound: assumes full volume size", false},
		{"ec2.eip", nil, nil, 30, 3.65, "on-demand list price", false},
		{"ec2.natgateway", nil, nil, 30, 61.32, "data processing excluded", false},
		{"elb.loadbalancer", map[string]any{"lb_type": "application"}, nil, 30, 61.32, "capacity units excluded", false},
		{"rds.instance", map[string]any{"instance_class": "db.t3.small", "engine": "postgres", "multi_az": true, "storage_type": "gp3", "allocated_storage_gb": 100}, nil, 30, 156, "backups and I/O excluded", false},
		{"rds.instance", map[string]any{"instance_class": "db.t3.small", "engine": "aurora-postgresql", "cluster_id": "cluster", "allocated_storage_gb": 100}, nil, 30, 146, "backups and I/O excluded", false},
		{"rds.cluster", nil, nil, 30, 0, "instances are billed separately; cluster storage not estimated", true},
		{"elasticache.cluster", map[string]any{"node_type": "cache.t3.small", "num_nodes": 2, "engine": "redis"}, nil, 30, 146, "on-demand list price", false},
		{"eks.cluster", nil, nil, 30, 73, "control plane only", false},
		{"lambda.function", map[string]any{"memory_mb": 512}, usage.Metrics{"invocations": 3e6, "duration_ms": 6e8}, 30, 5.60, "free tier ignored", false},
		{"lambda.function", map[string]any{"memory_mb": 512}, usage.Metrics{"invocations": 3e6, "duration_ms": 6e8}, 15, 11.20, "free tier ignored", false},
		{"dynamodb.table", map[string]any{"billing_mode": "PROVISIONED", "read_capacity": 10, "write_capacity": 5}, nil, 30, 3.32, "provisioned capacity only; storage excluded", false},
		{"dynamodb.table", map[string]any{"billing_mode": "PAY_PER_REQUEST"}, nil, 30, 0, "no estimate for usage-based pricing", true},
		{"sqs.queue", nil, nil, 30, 0, "no estimate for usage-based pricing", true},
		{"cloudfront.distribution", nil, nil, 30, 0, "no estimate for usage-based pricing", true},
		{"s3.bucket", map[string]any{"bucket_region": "sa-east-1"}, usage.Metrics{"size_bytes": 100 * (1 << 30)}, 30, 2.30, "Standard storage class only", false},
		{"s3.bucket", nil, nil, 30, 0, "no storage metrics", true},
		{"iam.role", nil, nil, 30, 0, "not priced", true},
		{"acm.certificate", nil, nil, 30, 0, "not priced", true},
		{"ec2.securitygroup", nil, nil, 30, 0, "not priced", true},
		{"route53.hostedzone", nil, nil, 30, .50, "hosted zone fee only", false},
	}
	for _, tt := range tests {
		t.Run(tt.typ+tt.note, func(t *testing.T) {
			r := model.Resource{Type: "aws." + tt.typ, Region: "us-east-1", Config: tt.c}
			p := &fakePrices{}
			Estimate(context.Background(), &r, tt.m, tt.days, p)
			if r.Cost.Note != tt.note {
				t.Errorf("note %q want %q", r.Cost.Note, tt.note)
			}
			if tt.none {
				if r.Cost.Source != model.CostNone || r.Cost.MonthlyUSD != nil {
					t.Fatal(r.Cost)
				}
				return
			}
			if r.Cost.Source != model.CostEstimate || r.Cost.MonthlyUSD == nil || math.Abs(*r.Cost.MonthlyUSD-tt.want) > .00001 {
				t.Fatalf("%+v want %.2f", r.Cost, tt.want)
			}
			if tt.typ == "rds.instance" && r.Bool("multi_az") && p.queries[0].Filters["deploymentOption"] != "Multi-AZ" {
				t.Fatal(p.queries)
			}
			if tt.typ == "s3.bucket" && p.queries[0].Region != "sa-east-1" {
				t.Fatal(p.queries)
			}
		})
	}
}
func TestEstimateLookupErrors(t *testing.T) {
	for _, err := range []error{ErrNoPrice, errors.New("denied")} {
		r := model.Resource{Type: "aws.ebs.volume", Config: map[string]any{"size_gb": 100, "volume_type": "gp3"}}
		Estimate(context.Background(), &r, nil, 30, &fakePrices{err: err})
		if r.Cost.Source != model.CostNone || r.Cost.MonthlyUSD != nil {
			t.Fatal(r.Cost)
		}
		want := "pricing unavailable: denied"
		if errors.Is(err, ErrNoPrice) {
			want = "no list price found for "
		}
		if !strings.HasPrefix(r.Cost.Note, want) {
			t.Fatal(r.Cost.Note)
		}
	}
}
func TestEstimateMissingLambdaMetrics(t *testing.T) {
	r := model.Resource{Type: "aws.lambda.function", Config: map[string]any{"memory_mb": 512}}
	Estimate(context.Background(), &r, usage.Metrics{"invocations": 100}, 30, &fakePrices{})
	if r.Cost.Source != model.CostNone || r.Cost.MonthlyUSD != nil {
		t.Fatal("unknown duration must not become free:", r.Cost)
	}
}

func TestEstimateLoadBalancerHourlyProduct(t *testing.T) {
	for _, typ := range []string{"application", "network", "gateway", "classic"} {
		r := model.Resource{Type: "aws.elb.loadbalancer", Region: "us-east-1", Config: map[string]any{"lb_type": typ}}
		p := &fakePrices{}
		Estimate(context.Background(), &r, nil, 30, p)
		title := strings.ToUpper(typ[:1]) + typ[1:]
		if len(p.queries) != 1 || p.queries[0].Filters["groupDescription"] != "LoadBalancer hourly usage by "+title+" Load Balancer" {
			t.Fatal("hourly dimension must exclude LCUs/data:", p.queries)
		}
	}
}
