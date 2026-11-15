package collect

import (
	"context"
	"errors"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	v1 "github.com/aws/aws-sdk-go-v2/service/elasticloadbalancing"
	t1 "github.com/aws/aws-sdk-go-v2/service/elasticloadbalancing/types"
	v2 "github.com/aws/aws-sdk-go-v2/service/elasticloadbalancingv2"
	t2 "github.com/aws/aws-sdk-go-v2/service/elasticloadbalancingv2/types"
)

type fakeV2 struct{ fail string }

func (f fakeV2) DescribeLoadBalancers(_ context.Context, in *v2.DescribeLoadBalancersInput, _ ...func(*v2.Options)) (*v2.DescribeLoadBalancersOutput, error) {
	if f.fail == "DescribeLoadBalancers" {
		return nil, errors.New("denied")
	}
	o := &v2.DescribeLoadBalancersOutput{LoadBalancers: []t2.LoadBalancer{{LoadBalancerArn: aws.String("arn:aws:elasticloadbalancing:us-east-1:123:loadbalancer/app/name/abc123"), LoadBalancerName: aws.String("name"), Type: t2.LoadBalancerTypeEnumApplication}}}
	if in.Marker == nil {
		o.NextMarker = aws.String("next")
		o.LoadBalancers = append(o.LoadBalancers, t2.LoadBalancer{LoadBalancerArn: aws.String("arn:aws:elasticloadbalancing:us-east-1:123:loadbalancer/net/second/def456"), LoadBalancerName: aws.String("second"), Type: t2.LoadBalancerTypeEnumNetwork})
	} else {
		o.LoadBalancers = []t2.LoadBalancer{{LoadBalancerArn: aws.String("arn:aws:elasticloadbalancing:us-east-1:123:loadbalancer/app/sparse/000"), LoadBalancerName: aws.String("sparse")}}
	}
	return o, nil
}
func (f fakeV2) DescribeTargetGroups(_ context.Context, in *v2.DescribeTargetGroupsInput, _ ...func(*v2.Options)) (*v2.DescribeTargetGroupsOutput, error) {
	if f.fail == "DescribeTargetGroups" {
		return nil, errors.New("denied")
	}
	o := &v2.DescribeTargetGroupsOutput{TargetGroups: []t2.TargetGroup{{TargetGroupArn: aws.String("tg")}}}
	if in.Marker == nil {
		o.NextMarker = aws.String("next")
	}
	return o, nil
}
func (f fakeV2) DescribeTargetHealth(context.Context, *v2.DescribeTargetHealthInput, ...func(*v2.Options)) (*v2.DescribeTargetHealthOutput, error) {
	if f.fail == "DescribeTargetHealth" {
		return nil, errors.New("denied")
	}
	return &v2.DescribeTargetHealthOutput{TargetHealthDescriptions: []t2.TargetHealthDescription{{}}}, nil
}
func (f fakeV2) DescribeTags(context.Context, *v2.DescribeTagsInput, ...func(*v2.Options)) (*v2.DescribeTagsOutput, error) {
	if f.fail == "DescribeTags" {
		return nil, errors.New("denied")
	}
	return &v2.DescribeTagsOutput{}, nil
}

type fakeV1 struct{ fail string }

func (f fakeV1) DescribeLoadBalancers(_ context.Context, in *v1.DescribeLoadBalancersInput, _ ...func(*v1.Options)) (*v1.DescribeLoadBalancersOutput, error) {
	if f.fail == "DescribeLoadBalancers" {
		return nil, errors.New("denied")
	}
	o := &v1.DescribeLoadBalancersOutput{LoadBalancerDescriptions: []t1.LoadBalancerDescription{{LoadBalancerName: aws.String("classic"), Instances: []t1.Instance{{InstanceId: aws.String("i-1")}}}}}
	if in.Marker == nil {
		o.NextMarker = aws.String("next")
		o.LoadBalancerDescriptions = append(o.LoadBalancerDescriptions, t1.LoadBalancerDescription{LoadBalancerName: aws.String("classic2")})
	} else {
		o.LoadBalancerDescriptions = []t1.LoadBalancerDescription{{LoadBalancerName: aws.String("classic-sparse")}}
	}
	return o, nil
}
func (f fakeV1) DescribeTags(context.Context, *v1.DescribeTagsInput, ...func(*v1.Options)) (*v1.DescribeTagsOutput, error) {
	if f.fail == "DescribeTags" {
		return nil, errors.New("denied")
	}
	return &v1.DescribeTagsOutput{}, nil
}
func TestCollectLoadBalancers(t *testing.T) {
	r, e := collectLoadBalancers(context.Background(), fakeV2{}, fakeV1{}, Scope{"123", "us-east-1"})
	if e != nil || len(r) != 6 {
		t.Fatalf("%v %v", r, e)
	}
	if r[0].Str("metric_dimension") != "app/name/abc123" || r[0].Int("target_count") != 2 || r[3].Str("lb_type") != "classic" || r[3].Int("target_count") != 1 {
		t.Fatal(r)
	}
}

func TestLoadBalancerErrors(t *testing.T) {
	for _, m := range []string{"DescribeLoadBalancers", "DescribeTargetGroups", "DescribeTargetHealth", "DescribeTags"} {
		r, e := collectLoadBalancers(context.Background(), fakeV2{fail: m}, fakeV1{}, Scope{})
		if e == nil || len(r) != 3 {
			t.Fatalf("%s: %v %v", m, r, e)
		}
	}
	for _, m := range []string{"DescribeLoadBalancers", "DescribeTags"} {
		r, e := collectLoadBalancers(context.Background(), fakeV2{}, fakeV1{fail: m}, Scope{})
		if e == nil || len(r) != 3 {
			t.Fatalf("classic %s: %v %v", m, r, e)
		}
	}
}
func TestLoadBalancerSparseContract(t *testing.T) {
	r, e := collectLoadBalancers(context.Background(), fakeV2{}, fakeV1{}, Scope{"123", "us-east-1"})
	if e != nil {
		t.Fatal(e)
	}
	keys := []string{"lb_type", "scheme", "dns_name", "state", "vpc_id", "target_count", "metric_dimension"}
	assertContract(t, r[2], "aws.elb.loadbalancer", "elb", "sparse", "arn:aws:elasticloadbalancing:us-east-1:123:loadbalancer/app/sparse/000", keys)
	assertContract(t, r[5], "aws.elb.loadbalancer", "elb", "classic-sparse", "arn:aws:elasticloadbalancing:us-east-1:123:loadbalancer/classic-sparse", keys)
}
