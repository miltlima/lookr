package collect

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"lookr/internal/model"

	"github.com/aws/aws-sdk-go-v2/aws"
	v1 "github.com/aws/aws-sdk-go-v2/service/elasticloadbalancing"
	v2 "github.com/aws/aws-sdk-go-v2/service/elasticloadbalancingv2"
)

type elbv2API interface {
	DescribeLoadBalancers(context.Context, *v2.DescribeLoadBalancersInput, ...func(*v2.Options)) (*v2.DescribeLoadBalancersOutput, error)
	DescribeTargetGroups(context.Context, *v2.DescribeTargetGroupsInput, ...func(*v2.Options)) (*v2.DescribeTargetGroupsOutput, error)
	DescribeTargetHealth(context.Context, *v2.DescribeTargetHealthInput, ...func(*v2.Options)) (*v2.DescribeTargetHealthOutput, error)
	DescribeTags(context.Context, *v2.DescribeTagsInput, ...func(*v2.Options)) (*v2.DescribeTagsOutput, error)
}
type elbAPI interface {
	DescribeLoadBalancers(context.Context, *v1.DescribeLoadBalancersInput, ...func(*v1.Options)) (*v1.DescribeLoadBalancersOutput, error)
	DescribeTags(context.Context, *v1.DescribeTagsInput, ...func(*v1.Options)) (*v1.DescribeTagsOutput, error)
}

func collectLoadBalancers(ctx context.Context, api elbv2API, classic elbAPI, s Scope) ([]model.Resource, error) {
	modern, modernErr := collectV2LoadBalancers(ctx, api, s)
	legacy, legacyErr := collectClassicLoadBalancers(ctx, classic, s)
	return append(modern, legacy...), errors.Join(modernErr, legacyErr)
}
func collectV2LoadBalancers(ctx context.Context, api elbv2API, s Scope) ([]model.Resource, error) {
	var out []model.Resource
	pager := v2.NewDescribeLoadBalancersPaginator(api, &v2.DescribeLoadBalancersInput{})
	for pager.HasMorePages() {
		page, e := pager.NextPage(ctx)
		if e != nil {
			return out, e
		}
		for _, x := range page.LoadBalancers {
			count := 0
			groups := v2.NewDescribeTargetGroupsPaginator(api, &v2.DescribeTargetGroupsInput{LoadBalancerArn: x.LoadBalancerArn})
			for groups.HasMorePages() {
				gp, e := groups.NextPage(ctx)
				if e != nil {
					return out, e
				}
				for _, g := range gp.TargetGroups {
					h, e := api.DescribeTargetHealth(ctx, &v2.DescribeTargetHealthInput{TargetGroupArn: g.TargetGroupArn})
					if e != nil {
						return out, e
					}
					count += len(h.TargetHealthDescriptions)
				}
			}
			tags := map[string]string{}
			tp, e := api.DescribeTags(ctx, &v2.DescribeTagsInput{ResourceArns: []string{aws.ToString(x.LoadBalancerArn)}})
			if e != nil {
				return out, e
			}
			for _, d := range tp.TagDescriptions {
				for _, t := range d.Tags {
					tags[aws.ToString(t.Key)] = aws.ToString(t.Value)
				}
			}
			state := ""
			if x.State != nil {
				state = string(x.State.Code)
			}
			arn := aws.ToString(x.LoadBalancerArn)
			_, dimension, _ := strings.Cut(arn, "loadbalancer/")
			r := resource(s, "aws.elb.loadbalancer", "elb", aws.ToString(x.LoadBalancerName), "", tags, map[string]any{"lb_type": string(x.Type), "scheme": string(x.Scheme), "dns_name": aws.ToString(x.DNSName), "state": state, "vpc_id": aws.ToString(x.VpcId), "target_count": count, "metric_dimension": dimension})
			r.ARN = arn
			r.CreatedAt = x.CreatedTime
			out = append(out, r)
		}
	}
	return out, nil
}

func collectClassicLoadBalancers(ctx context.Context, classic elbAPI, s Scope) ([]model.Resource, error) {
	var out []model.Resource
	cp := v1.NewDescribeLoadBalancersPaginator(classic, &v1.DescribeLoadBalancersInput{})
	for cp.HasMorePages() {
		page, e := cp.NextPage(ctx)
		if e != nil {
			return out, e
		}
		for _, x := range page.LoadBalancerDescriptions {
			tags := map[string]string{}
			tp, e := classic.DescribeTags(ctx, &v1.DescribeTagsInput{LoadBalancerNames: []string{aws.ToString(x.LoadBalancerName)}})
			if e != nil {
				return out, e
			}
			for _, d := range tp.TagDescriptions {
				for _, t := range d.Tags {
					tags[aws.ToString(t.Key)] = aws.ToString(t.Value)
				}
			}
			name := aws.ToString(x.LoadBalancerName)
			r := resource(s, "aws.elb.loadbalancer", "elb", name, "", tags, map[string]any{"lb_type": "classic", "scheme": aws.ToString(x.Scheme), "dns_name": aws.ToString(x.DNSName), "state": "", "vpc_id": aws.ToString(x.VPCId), "target_count": len(x.Instances), "metric_dimension": name})
			r.ARN = fmt.Sprintf("arn:aws:elasticloadbalancing:%s:%s:loadbalancer/%s", s.Region, s.AccountID, name)
			r.CreatedAt = x.CreatedTime
			out = append(out, r)
		}
	}
	return out, nil
}
