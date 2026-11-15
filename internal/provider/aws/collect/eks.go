package collect

import (
	"context"
	"fmt"
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/ec2"
	"github.com/aws/aws-sdk-go-v2/service/ec2/types"
	"github.com/aws/aws-sdk-go-v2/service/eks"
	"lookr/internal/model"
)

type clusterAPI interface {
	ListClusters(context.Context, *eks.ListClustersInput, ...func(*eks.Options)) (*eks.ListClustersOutput, error)
	DescribeCluster(context.Context, *eks.DescribeClusterInput, ...func(*eks.Options)) (*eks.DescribeClusterOutput, error)
	ListNodegroups(context.Context, *eks.ListNodegroupsInput, ...func(*eks.Options)) (*eks.ListNodegroupsOutput, error)
	ListFargateProfiles(context.Context, *eks.ListFargateProfilesInput, ...func(*eks.Options)) (*eks.ListFargateProfilesOutput, error)
}
type clusterInstanceAPI interface {
	DescribeInstances(context.Context, *ec2.DescribeInstancesInput, ...func(*ec2.Options)) (*ec2.DescribeInstancesOutput, error)
}

func collectClusters(ctx context.Context, c clusterAPI, instances clusterInstanceAPI, s Scope) ([]model.Resource, error) {
	var result []model.Resource
	in := &eks.ListClustersInput{}
	for {
		o, e := c.ListClusters(ctx, in)
		if e != nil {
			return result, e
		}
		for _, id := range o.Clusters {
			d, e := c.DescribeCluster(ctx, &eks.DescribeClusterInput{Name: aws.String(id)})
			if e != nil {
				return result, e
			}
			v := d.Cluster
			if v == nil {
				return result, fmt.Errorf("cluster %s: missing description", id)
			}
			ng, fp, nodes := 0, 0, 0
			ni := &eks.ListNodegroupsInput{ClusterName: aws.String(id)}
			for {
				n, e := c.ListNodegroups(ctx, ni)
				if e != nil {
					return result, e
				}
				ng += len(n.Nodegroups)
				if aws.ToString(n.NextToken) == "" {
					break
				}
				ni.NextToken = n.NextToken
			}
			fi := &eks.ListFargateProfilesInput{ClusterName: aws.String(id)}
			for {
				f, e := c.ListFargateProfiles(ctx, fi)
				if e != nil {
					return result, e
				}
				fp += len(f.FargateProfileNames)
				if aws.ToString(f.NextToken) == "" {
					break
				}
				fi.NextToken = f.NextToken
			}
			ei := &ec2.DescribeInstancesInput{Filters: []types.Filter{{Name: aws.String("tag-key"), Values: []string{"kubernetes.io/cluster/" + id}}, {Name: aws.String("instance-state-name"), Values: []string{"running", "pending"}}}}
			for {
				n, e := instances.DescribeInstances(ctx, ei)
				if e != nil {
					return result, e
				}
				for _, r := range n.Reservations {
					for _, i := range r.Instances {
						if i.State != nil && (i.State.Name == types.InstanceStateNameRunning || i.State.Name == types.InstanceStateNamePending) {
							nodes++
						}
					}
				}
				if aws.ToString(n.NextToken) == "" {
					break
				}
				ei.NextToken = n.NextToken
			}
			public := false
			cidrs := []string{}
			if v.ResourcesVpcConfig != nil {
				public = v.ResourcesVpcConfig.EndpointPublicAccess
				cidrs = append(cidrs, v.ResourcesVpcConfig.PublicAccessCidrs...)
			}
			r := resource(s, "aws.eks.cluster", "eks", id, "", v.Tags, map[string]any{"version": aws.ToString(v.Version), "status": string(v.Status), "endpoint": aws.ToString(v.Endpoint), "nodegroup_count": ng, "fargate_profile_count": fp, "tagged_instance_count": nodes, "endpoint_public": public, "public_access_cidrs": cidrs})
			r.ARN = aws.ToString(v.Arn)
			r.CreatedAt = v.CreatedAt
			result = append(result, r)
		}
		if aws.ToString(o.NextToken) == "" {
			break
		}
		in.NextToken = o.NextToken
	}
	return result, nil
}
