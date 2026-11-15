package usage

import (
	"lookr/internal/model"
	"testing"
	"time"
)

func TestRules(t *testing.T) {
	now := time.Date(2026, 11, 15, 0, 0, 0, 0, time.UTC)
	ancient := now.Add(-31 * 24 * time.Hour)
	boundary := now.Add(-30 * 24 * time.Hour)
	tests := []struct {
		name, typ string
		c         map[string]any
		m         Metrics
		created   *time.Time
		want      model.Verdict
		reason    string
	}{
		{"stopped", "ec2.instance", map[string]any{"state": "stopped"}, nil, nil, model.VerdictIdle, "instance is stopped"},
		{"pending", "ec2.instance", map[string]any{"state": "pending"}, nil, nil, model.VerdictUnknown, "instance state is pending"},
		{"cpu missing", "ec2.instance", map[string]any{"state": "running"}, nil, nil, model.VerdictUnknown, "no CPU metrics"},
		{"cpu idle", "ec2.instance", map[string]any{"state": "running"}, Metrics{"cpu_avg": 0.8, "cpu_max": 9, "net_in": 0, "net_out": 0}, nil, model.VerdictIdle, "avg CPU 0.8% over 30d"},
		{"cpu max boundary", "ec2.instance", map[string]any{"state": "running"}, Metrics{"cpu_avg": 1.99, "cpu_max": 10}, nil, model.VerdictInUse, "avg CPU 2.0% over 30d"},
		{"network active", "ec2.instance", map[string]any{"state": "running"}, Metrics{"cpu_avg": 1, "net_in": 30 * 6 * (1 << 20)}, nil, model.VerdictInUse, "avg CPU 1.0% over 30d"},
		{"volume orphan", "ebs.volume", map[string]any{"state": "available"}, nil, nil, model.VerdictOrphan, "not attached to any instance"},
		{"volume stopped", "ebs.volume", map[string]any{"attached_instance_state": "stopped", "attached_instance_id": "i-1"}, nil, nil, model.VerdictIdle, "attached to stopped instance i-1"},
		{"volume used", "ebs.volume", map[string]any{"attached_instance_id": "i-1"}, nil, nil, model.VerdictInUse, "attached to i-1"},
		{"snapshot orphan", "ebs.snapshot", nil, nil, &ancient, model.VerdictOrphan, "source volume no longer exists"},
		{"snapshot boundary", "ebs.snapshot", nil, nil, &boundary, model.VerdictNotEvaluated, "inventory only"},
		{"snapshot protected", "ebs.snapshot", map[string]any{"used_by_ami": true}, nil, &ancient, model.VerdictNotEvaluated, "inventory only"},
		{"eip orphan", "ec2.eip", nil, nil, nil, model.VerdictOrphan, "not associated"},
		{"eip used", "ec2.eip", map[string]any{"association_id": "a"}, nil, nil, model.VerdictInUse, "associated"},
		{"nat idle", "ec2.natgateway", nil, Metrics{"bytes_in": 0, "bytes_out": 0}, nil, model.VerdictIdle, "0 bytes over 30d"},
		{"nat used", "ec2.natgateway", nil, Metrics{"bytes_in": 1}, nil, model.VerdictInUse, "1 bytes over 30d"},
		{"gateway", "elb.loadbalancer", map[string]any{"lb_type": "gateway"}, nil, nil, model.VerdictUnknown, "no usage rule for gateway load balancers"},
		{"elb orphan", "elb.loadbalancer", nil, nil, nil, model.VerdictOrphan, "no registered targets"},
		{"elb idle", "elb.loadbalancer", map[string]any{"target_count": 1}, Metrics{"activity": 0}, nil, model.VerdictIdle, "0 requests over 30d"},
		{"elb used", "elb.loadbalancer", map[string]any{"target_count": 1}, Metrics{"activity": 2}, nil, model.VerdictInUse, "2 requests over 30d"},
		{"rds stopped", "rds.instance", map[string]any{"status": "stopped"}, nil, nil, model.VerdictIdle, "instance is stopped"},
		{"rds missing", "rds.instance", nil, nil, nil, model.VerdictUnknown, "no connection metrics"},
		{"rds idle", "rds.instance", nil, Metrics{"conn_max": 0}, nil, model.VerdictIdle, "0 connections over 30d"},
		{"rds used", "rds.instance", nil, Metrics{"conn_max": 14}, nil, model.VerdictInUse, "max 14 connections"},
		{"aurora orphan", "rds.cluster", nil, nil, nil, model.VerdictOrphan, "cluster has no instances"},
		{"aurora used", "rds.cluster", map[string]any{"member_count": 1}, Metrics{"conn_max": 14}, nil, model.VerdictInUse, "max 14 connections"},
		{"cache missing", "elasticache.cluster", nil, Metrics{"cmds_get": 0}, nil, model.VerdictUnknown, "no command metrics"},
		{"cache idle", "elasticache.cluster", map[string]any{"engine": "memcached"}, Metrics{"cmds_get": 0, "cmds_set": 0}, nil, model.VerdictIdle, "0 commands over 30d"},
		{"cache used", "elasticache.cluster", nil, Metrics{"cmds_get": 1, "cmds_set": 2}, nil, model.VerdictInUse, "3 commands over 30d"},
		{"eks idle", "eks.cluster", nil, nil, nil, model.VerdictIdle, "no node groups, Fargate profiles or tagged instances"},
		{"eks used", "eks.cluster", map[string]any{"tagged_instance_count": 1}, nil, nil, model.VerdictInUse, "cluster has compute resources"},
		{"lambda idle", "lambda.function", nil, Metrics{"invocations": 0}, nil, model.VerdictIdle, "0 invocations over 30d"},
		{"lambda used", "lambda.function", nil, Metrics{"invocations": 2}, nil, model.VerdictInUse, "2 invocations over 30d"},
		{"dynamo idle", "dynamodb.table", nil, Metrics{"read_units": 0, "write_units": 0}, nil, model.VerdictIdle, "0 read/write units over 30d"},
		{"dynamo used", "dynamodb.table", nil, Metrics{"write_units": 2}, nil, model.VerdictInUse, "2 read/write units over 30d"},
		{"sqs idle", "sqs.queue", nil, Metrics{"sent": 0, "received": 0}, nil, model.VerdictIdle, "0 messages sent or received over 30d"},
		{"sqs used", "sqs.queue", nil, Metrics{"received": 2}, nil, model.VerdictInUse, "2 messages sent or received over 30d"},
		{"cdn disabled", "cloudfront.distribution", nil, nil, nil, model.VerdictIdle, "distribution is disabled"},
		{"cdn idle", "cloudfront.distribution", map[string]any{"enabled": true}, Metrics{"requests": 0}, nil, model.VerdictIdle, "0 requests over 30d"},
		{"cdn used", "cloudfront.distribution", map[string]any{"enabled": true}, Metrics{"requests": 2}, nil, model.VerdictInUse, "2 requests over 30d"},
		{"s3 missing", "s3.bucket", nil, nil, nil, model.VerdictUnknown, "no storage metrics"},
		{"s3 empty", "s3.bucket", nil, Metrics{"object_count": 0}, nil, model.VerdictIdle, "bucket is empty"},
		{"s3 populated", "s3.bucket", nil, Metrics{"object_count": 12}, nil, model.VerdictUnknown, "S3 publishes no access metrics by default"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rule := Rules()["aws."+tt.typ]
			if rule.Evaluate == nil {
				t.Fatal("missing rule")
			}
			v, s := rule.Evaluate(model.Resource{Config: tt.c, CreatedAt: tt.created}, tt.m, now, 30)
			if v != tt.want || s != tt.reason {
				t.Fatalf("got %s %q, want %s %q", v, s, tt.want, tt.reason)
			}
		})
	}
}
func TestRulesCoverContract(t *testing.T) {
	for _, typ := range []string{"ec2.instance", "ebs.volume", "ebs.snapshot", "ec2.eip", "ec2.natgateway", "elb.loadbalancer", "rds.instance", "rds.cluster", "elasticache.cluster", "eks.cluster", "lambda.function", "dynamodb.table", "sqs.queue", "cloudfront.distribution", "s3.bucket"} {
		if _, ok := Rules()["aws."+typ]; !ok {
			t.Error(typ)
		}
	}
	for _, typ := range []string{"ec2.securitygroup", "iam.user", "iam.group", "iam.role", "acm.certificate", "route53.hostedzone"} {
		if _, ok := Rules()["aws."+typ]; ok {
			t.Error(typ)
		}
	}
}
func TestQueriesFor(t *testing.T) {
	tests := []struct {
		typ         string
		c           map[string]any
		keys        []string
		zero        bool
		dim, region string
	}{
		{"ec2.instance", nil, []string{"cpu_avg", "cpu_max", "net_in", "net_out"}, false, "InstanceId", ""},
		{"ec2.natgateway", nil, []string{"bytes_out", "bytes_in"}, true, "NatGatewayId", ""},
		{"elb.loadbalancer", map[string]any{"lb_type": "application", "metric_dimension": "app/x"}, []string{"activity"}, true, "LoadBalancer", ""},
		{"elb.loadbalancer", map[string]any{"lb_type": "network", "metric_dimension": "net/x"}, []string{"activity"}, true, "LoadBalancer", ""},
		{"elb.loadbalancer", map[string]any{"lb_type": "classic", "metric_dimension": "classic-x"}, []string{"activity"}, true, "LoadBalancerName", ""},
		{"rds.instance", nil, []string{"conn_max"}, false, "DBInstanceIdentifier", ""}, {"rds.cluster", nil, []string{"conn_max"}, false, "DBClusterIdentifier", ""},
		{"elasticache.cluster", map[string]any{"engine": "memcached"}, []string{"cmds_get", "cmds_set"}, false, "CacheClusterId", ""},
		{"lambda.function", nil, []string{"invocations", "duration_ms"}, true, "FunctionName", ""},
		{"dynamodb.table", nil, []string{"read_units", "write_units"}, true, "TableName", ""},
		{"sqs.queue", nil, []string{"sent", "received"}, true, "QueueName", ""},
		{"cloudfront.distribution", nil, []string{"requests"}, true, "DistributionId", "us-east-1"},
		{"s3.bucket", map[string]any{"bucket_region": "eu-west-1"}, []string{"object_count", "size_bytes"}, false, "BucketName", "eu-west-1"},
	}
	for _, tt := range tests {
		t.Run(tt.typ+tt.dim, func(t *testing.T) {
			qs := QueriesFor(model.Resource{Type: "aws." + tt.typ, ID: "native", Config: tt.c})
			if len(qs) != len(tt.keys) {
				t.Fatalf("%+v", qs)
			}
			for i, q := range qs {
				zero := tt.zero
				if q.Key == "net_in" || q.Key == "net_out" {
					zero = true
				}
				val := "native"
				if tt.typ == "elb.loadbalancer" {
					val = tt.c["metric_dimension"].(string)
				}
				if q.Key != tt.keys[i] || q.ZeroIfMissing != zero || q.Dimensions[tt.dim] != val || q.Region != tt.region || q.Namespace == "" || q.Metric == "" || q.Stat == "" {
					t.Fatalf("%+v", q)
				}
				if tt.typ == "cloudfront.distribution" && q.Dimensions["Region"] != "Global" {
					t.Fatal(q)
				}
			}
		})
	}
	for _, typ := range []string{"aws.iam.role", "aws.ebs.volume"} {
		if QueriesFor(model.Resource{Type: typ}) != nil {
			t.Error(typ)
		}
	}
	if QueriesFor(model.Resource{Type: "aws.elb.loadbalancer", Config: map[string]any{"lb_type": "gateway"}}) != nil {
		t.Fatal("gateway queries")
	}
}

func TestIncompleteMetricsNeverImplyIdle(t *testing.T) {
	for _, typ := range []string{"aws.ec2.natgateway", "aws.lambda.function", "aws.dynamodb.table", "aws.sqs.queue"} {
		v, _ := Rules()[typ].Evaluate(model.Resource{}, nil, time.Now(), 30)
		if v != model.VerdictUnknown {
			t.Errorf("%s: %s", typ, v)
		}
	}
	r := model.Resource{Config: map[string]any{"state": "running"}}
	v, _ := Rules()["aws.ec2.instance"].Evaluate(r, Metrics{"cpu_avg": 0.8, "cpu_max": 2}, time.Now(), 30)
	if v != model.VerdictUnknown {
		t.Fatalf("missing network: %s", v)
	}
}

func TestQueryMetricNames(t *testing.T) {
	for _, engine := range []string{"redis", "valkey", "memcached"} {
		qs := QueriesFor(model.Resource{Type: "aws.elasticache.cluster", Config: map[string]any{"engine": engine}})
		get, set := "GetTypeCmds", "SetTypeCmds"
		if engine == "memcached" {
			get, set = "CmdGet", "CmdSet"
		}
		if qs[0].Metric != get || qs[1].Metric != set {
			t.Fatal(qs)
		}
	}
	qs := QueriesFor(model.Resource{Type: "aws.s3.bucket"})
	if qs[0].Dimensions["StorageType"] != "AllStorageTypes" || qs[1].Dimensions["StorageType"] != "StandardStorage" {
		t.Fatal(qs)
	}
}
