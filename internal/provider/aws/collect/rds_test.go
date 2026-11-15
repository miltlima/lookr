package collect

import (
	"context"
	"errors"
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/rds"
	"github.com/aws/aws-sdk-go-v2/service/rds/types"
	"testing"
)

type rdsFake struct {
	calls int
	fail  bool
}

func (f *rdsFake) DescribeDBInstances(ctx context.Context, in *rds.DescribeDBInstancesInput, opt ...func(*rds.Options)) (*rds.DescribeDBInstancesOutput, error) {
	f.calls++
	if f.fail {
		return nil, errors.New("denied")
	}
	o := &rds.DescribeDBInstancesOutput{DBInstances: []types.DBInstance{{DBClusterIdentifier: aws.String("aurora"), PubliclyAccessible: aws.Bool(false), StorageEncrypted: aws.Bool(false), BackupRetentionPeriod: aws.Int32(0), TagList: []types.Tag{{Key: aws.String("Name"), Value: aws.String("named")}}}}}
	if f.calls == 1 {
		o.Marker = aws.String("next")
	}
	return o, nil
}
func TestCollectDBInstances(t *testing.T) {
	f := &rdsFake{}
	r, e := collectDBInstances(context.Background(), f, Scope{Region: "r"})
	if e != nil || len(r) != 2 || f.calls != 2 {
		t.Fatalf("%v %v", r, e)
	}
	if r[0].Str("cluster_id") != "aurora" || r[0].Int("port") != 0 || r[0].Name != "named" {
		t.Fatal(r[0])
	}
	f = &rdsFake{fail: true}
	if _, e = collectDBInstances(context.Background(), f, Scope{}); e == nil {
		t.Fatal("missing error")
	}
}

type clusterFake struct {
	calls int
	fail  bool
}

func (f *clusterFake) DescribeDBClusters(_ context.Context, in *rds.DescribeDBClustersInput, _ ...func(*rds.Options)) (*rds.DescribeDBClustersOutput, error) {
	f.calls++
	if f.fail {
		return nil, errors.New("denied")
	}
	o := &rds.DescribeDBClustersOutput{DBClusters: []types.DBCluster{{DBClusterIdentifier: aws.String("cluster"), DBClusterMembers: []types.DBClusterMember{{}, {}}}}}
	if f.calls == 1 {
		o.Marker = aws.String("next")
	}
	return o, nil
}
func TestCollectDBClusters(t *testing.T) {
	f := &clusterFake{}
	r, e := collectDBClusters(context.Background(), f, Scope{})
	if e != nil || len(r) != 2 || r[0].Int("member_count") != 2 {
		t.Fatalf("%v %v", r, e)
	}
	f = &clusterFake{fail: true}
	if _, e = collectDBClusters(context.Background(), f, Scope{}); e == nil {
		t.Fatal("missing error")
	}
}

func TestDataCollectorContracts(t *testing.T) {
	ctx := context.Background()
	rdsRows, _ := collectDBInstances(ctx, &rdsFake{}, Scope{})
	clusterRows, _ := collectDBClusters(ctx, &clusterFake{}, Scope{})
	cacheRows, _ := collectCacheClusters(ctx, &elasticacheFake{}, Scope{})
	tableRows, _ := collectTables(ctx, &tableFake{}, Scope{})
	queueRows, _ := collectQueues(ctx, &queueFake{t: t, attrs: map[string]string{"SqsManagedSseEnabled": "false"}}, Scope{})
	for _, tc := range []struct {
		config map[string]any
		keys   []string
	}{
		{rdsRows[0].Config, []string{"engine", "engine_version", "instance_class", "status", "multi_az", "storage_type", "allocated_storage_gb", "az", "port", "cluster_id", "publicly_accessible", "storage_encrypted", "backup_retention_days"}},
		{clusterRows[0].Config, []string{"engine", "engine_version", "status", "member_count", "engine_mode"}},
		{cacheRows[0].Config, []string{"engine", "engine_version", "node_type", "num_nodes", "status", "replication_group_id"}},
		{tableRows[0].Config, []string{"status", "billing_mode", "read_capacity", "write_capacity", "item_count", "size_bytes", "table_class"}},
		{queueRows[0].Config, []string{"url", "fifo", "visibility_timeout_s", "approximate_messages", "retention_s", "encrypted", "has_dlq"}},
	} {
		for _, key := range tc.keys {
			v, ok := tc.config[key]
			if !ok {
				t.Errorf("missing %s", key)
			}
			switch v.(type) {
			case string, bool, int, float64, []string:
			default:
				t.Errorf("invalid %s: %T", key, v)
			}
		}
	}
}
