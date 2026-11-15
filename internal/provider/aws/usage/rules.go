package usage

import (
	"fmt"
	"lookr/internal/model"
	"time"
)

func queries(namespace, dimension string, specs ...Query) func(model.Resource) []Query {
	return func(r model.Resource) []Query {
		out := make([]Query, len(specs))
		for i, s := range specs {
			s.Namespace = namespace
			s.Dimensions = map[string]string{dimension: r.ID}
			out[i] = s
		}
		return out
	}
}
func counter(key, metric string) Query {
	return Query{Key: key, Metric: metric, Stat: "Sum", ZeroIfMissing: true}
}
func gauge(key, metric, stat string) Query { return Query{Key: key, Metric: metric, Stat: stat} }
func activity(m Metrics, days int, noun string, keys ...string) (model.Verdict, string) {
	total := 0.0
	complete := true
	for _, k := range keys {
		v, ok := m[k]
		complete = complete && ok
		total += v
	}
	if !complete && total == 0 {
		return model.VerdictUnknown, "no activity metrics"
	}
	verdict := model.VerdictInUse
	if total == 0 {
		verdict = model.VerdictIdle
	}
	return verdict, fmt.Sprintf("%g %s over %dd", total, noun, days)
}
func database(r model.Resource, m Metrics, _ time.Time, days int) (model.Verdict, string) {
	if r.Str("status") == "stopped" {
		return model.VerdictIdle, "instance is stopped"
	}
	v, ok := m["conn_max"]
	if !ok {
		return model.VerdictUnknown, "no connection metrics"
	}
	if v == 0 {
		return model.VerdictIdle, fmt.Sprintf("0 connections over %dd", days)
	}
	return model.VerdictInUse, fmt.Sprintf("max %g connections", v)
}

// Rules returns a fresh registry so callers cannot mutate subsequent scans.
func Rules() map[string]Rule {
	return map[string]Rule{
		"aws.ec2.instance": {Queries: queries("AWS/EC2", "InstanceId", gauge("cpu_avg", "CPUUtilization", "Average"), gauge("cpu_max", "CPUUtilization", "Maximum"), counter("net_in", "NetworkIn"), counter("net_out", "NetworkOut")), Evaluate: func(r model.Resource, m Metrics, _ time.Time, days int) (model.Verdict, string) {
			state := r.Str("state")
			if state == "stopped" {
				return model.VerdictIdle, "instance is stopped"
			}
			if state != "running" {
				return model.VerdictUnknown, "instance state is " + state
			}
			avg, ok := m["cpu_avg"]
			if !ok {
				return model.VerdictUnknown, "no CPU metrics"
			}
			reason := fmt.Sprintf("avg CPU %.1f%% over %dd", avg, days)
			if avg >= EC2IdleCPUAvg || m["cpu_max"] >= EC2IdleCPUMax || (days > 0 && (m["net_in"]+m["net_out"])/float64(days) >= EC2IdleNetBytesPerDay) {
				return model.VerdictInUse, reason
			}
			if _, ok := m["cpu_max"]; !ok {
				return model.VerdictUnknown, "no CPU metrics"
			}
			if _, ok := m["net_in"]; !ok {
				return model.VerdictUnknown, "no network metrics"
			}
			if _, ok := m["net_out"]; !ok {
				return model.VerdictUnknown, "no network metrics"
			}
			if days <= 0 {
				return model.VerdictUnknown, "invalid observation window"
			}
			return model.VerdictIdle, reason
		}},
		"aws.ebs.volume": {Evaluate: func(r model.Resource, _ Metrics, _ time.Time, _ int) (model.Verdict, string) {
			if r.Str("state") == "available" {
				return model.VerdictOrphan, "not attached to any instance"
			}
			if r.Str("attached_instance_state") == "stopped" {
				return model.VerdictIdle, "attached to stopped instance " + r.Str("attached_instance_id")
			}
			return model.VerdictInUse, "attached to " + r.Str("attached_instance_id")
		}},
		"aws.ebs.snapshot": {Evaluate: func(r model.Resource, _ Metrics, now time.Time, days int) (model.Verdict, string) {
			if !r.Bool("source_volume_exists") && !r.Bool("used_by_ami") && r.CreatedAt != nil && now.Sub(*r.CreatedAt) > time.Duration(days)*24*time.Hour {
				return model.VerdictOrphan, "source volume no longer exists"
			}
			return model.VerdictNotEvaluated, "inventory only"
		}},
		"aws.ec2.eip": {Evaluate: func(r model.Resource, _ Metrics, _ time.Time, _ int) (model.Verdict, string) {
			if r.Str("association_id") == "" {
				return model.VerdictOrphan, "not associated"
			}
			return model.VerdictInUse, "associated"
		}},
		"aws.ec2.natgateway": {Queries: queries("AWS/NATGateway", "NatGatewayId", counter("bytes_out", "BytesOutToDestination"), counter("bytes_in", "BytesInFromDestination")), Evaluate: func(_ model.Resource, m Metrics, _ time.Time, d int) (model.Verdict, string) {
			return activity(m, d, "bytes", "bytes_out", "bytes_in")
		}},
		"aws.elb.loadbalancer": {Queries: func(r model.Resource) []Query {
			ns, metric, dim := "AWS/ApplicationELB", "RequestCount", "LoadBalancer"
			switch r.Str("lb_type") {
			case "gateway":
				return nil
			case "network":
				ns = "AWS/NetworkELB"
				metric = "NewFlowCount"
			case "classic":
				ns = "AWS/ELB"
				dim = "LoadBalancerName"
			}
			q := counter("activity", metric)
			q.Namespace = ns
			q.Dimensions = map[string]string{dim: r.Str("metric_dimension")}
			return []Query{q}
		}, Evaluate: func(r model.Resource, m Metrics, _ time.Time, d int) (model.Verdict, string) {
			if r.Str("lb_type") == "gateway" {
				return model.VerdictUnknown, "no usage rule for gateway load balancers"
			}
			if r.Int("target_count") == 0 {
				return model.VerdictOrphan, "no registered targets"
			}
			return activity(m, d, "requests", "activity")
		}},
		"aws.rds.instance": {Queries: queries("AWS/RDS", "DBInstanceIdentifier", gauge("conn_max", "DatabaseConnections", "Maximum")), Evaluate: database},
		"aws.rds.cluster": {Queries: queries("AWS/RDS", "DBClusterIdentifier", gauge("conn_max", "DatabaseConnections", "Maximum")), Evaluate: func(r model.Resource, m Metrics, n time.Time, d int) (model.Verdict, string) {
			if r.Int("member_count") == 0 {
				return model.VerdictOrphan, "cluster has no instances"
			}
			return database(r, m, n, d)
		}},
		"aws.elasticache.cluster": {Queries: func(r model.Resource) []Query {
			get, set := "GetTypeCmds", "SetTypeCmds"
			if r.Str("engine") == "memcached" {
				get, set = "CmdGet", "CmdSet"
			}
			return queries("AWS/ElastiCache", "CacheClusterId", gauge("cmds_get", get, "Sum"), gauge("cmds_set", set, "Sum"))(r)
		}, Evaluate: func(_ model.Resource, m Metrics, _ time.Time, d int) (model.Verdict, string) {
			if _, ok := m["cmds_get"]; !ok {
				return model.VerdictUnknown, "no command metrics"
			}
			if _, ok := m["cmds_set"]; !ok {
				return model.VerdictUnknown, "no command metrics"
			}
			return activity(m, d, "commands", "cmds_get", "cmds_set")
		}},
		"aws.eks.cluster": {Evaluate: func(r model.Resource, _ Metrics, _ time.Time, _ int) (model.Verdict, string) {
			if r.Int("nodegroup_count") == 0 && r.Int("fargate_profile_count") == 0 && r.Int("tagged_instance_count") == 0 {
				return model.VerdictIdle, "no node groups, Fargate profiles or tagged instances"
			}
			return model.VerdictInUse, "cluster has compute resources"
		}},
		"aws.lambda.function": {Queries: queries("AWS/Lambda", "FunctionName", counter("invocations", "Invocations"), counter("duration_ms", "Duration")), Evaluate: func(_ model.Resource, m Metrics, _ time.Time, d int) (model.Verdict, string) {
			return activity(m, d, "invocations", "invocations")
		}},
		"aws.dynamodb.table": {Queries: queries("AWS/DynamoDB", "TableName", counter("read_units", "ConsumedReadCapacityUnits"), counter("write_units", "ConsumedWriteCapacityUnits")), Evaluate: func(_ model.Resource, m Metrics, _ time.Time, d int) (model.Verdict, string) {
			return activity(m, d, "read/write units", "read_units", "write_units")
		}},
		"aws.sqs.queue": {Queries: queries("AWS/SQS", "QueueName", counter("sent", "NumberOfMessagesSent"), counter("received", "NumberOfMessagesReceived")), Evaluate: func(_ model.Resource, m Metrics, _ time.Time, d int) (model.Verdict, string) {
			return activity(m, d, "messages sent or received", "sent", "received")
		}},
		"aws.cloudfront.distribution": {Queries: func(r model.Resource) []Query {
			q := queries("AWS/CloudFront", "DistributionId", counter("requests", "Requests"))(r)
			q[0].Region = "us-east-1"
			q[0].Dimensions["Region"] = "Global"
			return q
		}, Evaluate: func(r model.Resource, m Metrics, _ time.Time, d int) (model.Verdict, string) {
			if !r.Bool("enabled") {
				return model.VerdictIdle, "distribution is disabled"
			}
			return activity(m, d, "requests", "requests")
		}},
		"aws.s3.bucket": {Queries: func(r model.Resource) []Query {
			qs := queries("AWS/S3", "BucketName", gauge("object_count", "NumberOfObjects", "Average"), gauge("size_bytes", "BucketSizeBytes", "Average"))(r)
			for i := range qs {
				qs[i].Region = r.Str("bucket_region")
			}
			qs[0].Dimensions["StorageType"] = "AllStorageTypes"
			qs[1].Dimensions["StorageType"] = "StandardStorage"
			return qs
		}, Evaluate: func(_ model.Resource, m Metrics, _ time.Time, _ int) (model.Verdict, string) {
			v, ok := m["object_count"]
			if !ok {
				return model.VerdictUnknown, "no storage metrics"
			}
			if v == 0 {
				return model.VerdictIdle, "bucket is empty"
			}
			return model.VerdictUnknown, "S3 publishes no access metrics by default"
		}},
	}
}
