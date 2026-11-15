package collect

import (
	"context"
	"fmt"
	"sort"

	"lookr/internal/model"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/ec2"
	"github.com/aws/aws-sdk-go-v2/service/elasticache"
	v1 "github.com/aws/aws-sdk-go-v2/service/elasticloadbalancing"
	v2 "github.com/aws/aws-sdk-go-v2/service/elasticloadbalancingv2"
	"github.com/aws/aws-sdk-go-v2/service/rds"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/aws-sdk-go-v2/service/sqs"
)

type Scope struct {
	AccountID string
	Region    string
}
type Collector struct {
	Type    string
	Short   string
	Global  bool
	Collect func(context.Context, aws.Config, Scope) ([]model.Resource, error)
}

func All() []Collector {
	return []Collector{
		{"aws.rds.instance", "rds", false, func(ctx context.Context, c aws.Config, s Scope) ([]model.Resource, error) {
			return collectDBInstances(ctx, rds.NewFromConfig(c), s)
		}},
		{"aws.rds.cluster", "aurora", false, func(ctx context.Context, c aws.Config, s Scope) ([]model.Resource, error) {
			return collectDBClusters(ctx, rds.NewFromConfig(c), s)
		}},
		{"aws.elasticache.cluster", "elasticache", false, func(ctx context.Context, c aws.Config, s Scope) ([]model.Resource, error) {
			return collectCacheClusters(ctx, elasticache.NewFromConfig(c), s)
		}},
		{"aws.dynamodb.table", "dynamodb", false, func(ctx context.Context, c aws.Config, s Scope) ([]model.Resource, error) {
			return collectTables(ctx, dynamodb.NewFromConfig(c), s)
		}},
		{"aws.sqs.queue", "sqs", false, func(ctx context.Context, c aws.Config, s Scope) ([]model.Resource, error) {
			return collectQueues(ctx, sqs.NewFromConfig(c), s)
		}},
		{"aws.s3.bucket", "s3", true, func(ctx context.Context, c aws.Config, s Scope) ([]model.Resource, error) {
			return collectBuckets(ctx, s3.NewFromConfig(c), func(region string) bucketRegionAPI { rc := c; rc.Region = region; return s3.NewFromConfig(rc) }, s)
		}},
		{"aws.ec2.instance", "ec2", false, func(ctx context.Context, c aws.Config, s Scope) ([]model.Resource, error) {
			return collectInstances(ctx, ec2.NewFromConfig(c), s)
		}},
		{"aws.ebs.volume", "ebs", false, func(ctx context.Context, c aws.Config, s Scope) ([]model.Resource, error) {
			return collectVolumes(ctx, ec2.NewFromConfig(c), s)
		}},
		{"aws.ebs.snapshot", "snapshot", false, func(ctx context.Context, c aws.Config, s Scope) ([]model.Resource, error) {
			return collectSnapshots(ctx, ec2.NewFromConfig(c), s)
		}},
		{"aws.ec2.eip", "eip", false, func(ctx context.Context, c aws.Config, s Scope) ([]model.Resource, error) {
			return collectAddresses(ctx, ec2.NewFromConfig(c), s)
		}},
		{"aws.ec2.natgateway", "nat", false, func(ctx context.Context, c aws.Config, s Scope) ([]model.Resource, error) {
			return collectNatGateways(ctx, ec2.NewFromConfig(c), s)
		}},
		{"aws.ec2.securitygroup", "sg", false, func(ctx context.Context, c aws.Config, s Scope) ([]model.Resource, error) {
			return collectSecurityGroups(ctx, ec2.NewFromConfig(c), s)
		}},
		{"aws.elb.loadbalancer", "elb", false, func(ctx context.Context, c aws.Config, s Scope) ([]model.Resource, error) {
			return collectLoadBalancers(ctx, v2.NewFromConfig(c), v1.NewFromConfig(c), s)
		}}}
}
func Shorts() []string {
	m := map[string]bool{}
	for _, c := range All() {
		m[c.Short] = true
	}
	r := []string{}
	for x := range m {
		r = append(r, x)
	}
	sort.Strings(r)
	return r
}
func resource(s Scope, typ, short, id, kind string, tags map[string]string, config map[string]any) model.Resource {
	name := id
	if tags["Name"] != "" {
		name = tags["Name"]
	}
	return model.Resource{Provider: "aws", AccountID: s.AccountID, Region: s.Region, Type: typ, Short: short, ID: id, Name: name, ARN: fmt.Sprintf("arn:aws:ec2:%s:%s:%s/%s", s.Region, s.AccountID, kind, id), Tags: tags, Config: config}
}
