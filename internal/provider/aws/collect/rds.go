package collect

import (
	"context"
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/rds"
	"lookr/internal/model"
)

type dbInstanceAPI interface {
	DescribeDBInstances(context.Context, *rds.DescribeDBInstancesInput, ...func(*rds.Options)) (*rds.DescribeDBInstancesOutput, error)
}
type dbClusterAPI interface {
	DescribeDBClusters(context.Context, *rds.DescribeDBClustersInput, ...func(*rds.Options)) (*rds.DescribeDBClustersOutput, error)
}

func collectDBInstances(ctx context.Context, c dbInstanceAPI, s Scope) ([]model.Resource, error) {
	var result []model.Resource
	in := &rds.DescribeDBInstancesInput{}
	for {
		o, e := c.DescribeDBInstances(ctx, in)
		if e != nil {
			return result, e
		}
		for _, d := range o.DBInstances {
			tags := map[string]string{}
			for _, t := range d.TagList {
				tags[aws.ToString(t.Key)] = aws.ToString(t.Value)
			}
			port := 0
			if d.Endpoint != nil {
				port = int(aws.ToInt32(d.Endpoint.Port))
			}
			r := resource(s, "aws.rds.instance", "rds", aws.ToString(d.DBInstanceIdentifier), "", tags, map[string]any{"engine": aws.ToString(d.Engine), "engine_version": aws.ToString(d.EngineVersion), "instance_class": aws.ToString(d.DBInstanceClass), "status": aws.ToString(d.DBInstanceStatus), "multi_az": aws.ToBool(d.MultiAZ), "storage_type": aws.ToString(d.StorageType), "allocated_storage_gb": int(aws.ToInt32(d.AllocatedStorage)), "az": aws.ToString(d.AvailabilityZone), "port": port, "cluster_id": aws.ToString(d.DBClusterIdentifier), "publicly_accessible": aws.ToBool(d.PubliclyAccessible), "storage_encrypted": aws.ToBool(d.StorageEncrypted), "backup_retention_days": int(aws.ToInt32(d.BackupRetentionPeriod))})
			if d.StorageEncrypted == nil {
				delete(r.Config, "storage_encrypted")
			}
			if d.BackupRetentionPeriod == nil {
				delete(r.Config, "backup_retention_days")
			}
			if d.PubliclyAccessible == nil {
				delete(r.Config, "publicly_accessible")
			}
			r.ARN = aws.ToString(d.DBInstanceArn)
			r.CreatedAt = d.InstanceCreateTime
			result = append(result, r)
		}
		if aws.ToString(o.Marker) == "" {
			break
		}
		in.Marker = o.Marker
	}
	return result, nil
}
func collectDBClusters(ctx context.Context, c dbClusterAPI, s Scope) ([]model.Resource, error) {
	var result []model.Resource
	in := &rds.DescribeDBClustersInput{}
	for {
		o, e := c.DescribeDBClusters(ctx, in)
		if e != nil {
			return result, e
		}
		for _, d := range o.DBClusters {
			tags := map[string]string{}
			for _, t := range d.TagList {
				tags[aws.ToString(t.Key)] = aws.ToString(t.Value)
			}
			r := resource(s, "aws.rds.cluster", "aurora", aws.ToString(d.DBClusterIdentifier), "", tags, map[string]any{"engine": aws.ToString(d.Engine), "engine_version": aws.ToString(d.EngineVersion), "status": aws.ToString(d.Status), "member_count": len(d.DBClusterMembers), "engine_mode": aws.ToString(d.EngineMode)})
			r.ARN = aws.ToString(d.DBClusterArn)
			r.CreatedAt = d.ClusterCreateTime
			result = append(result, r)
		}
		if aws.ToString(o.Marker) == "" {
			break
		}
		in.Marker = o.Marker
	}
	return result, nil
}
