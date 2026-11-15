package collect

import (
	"context"
	"errors"
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/ec2"
	ec2types "github.com/aws/aws-sdk-go-v2/service/ec2/types"
	"github.com/aws/aws-sdk-go-v2/service/eks"
	"github.com/aws/aws-sdk-go-v2/service/eks/types"
	"testing"
)

var _ = aws.String
var _ = types.Cluster{}

type eksFake struct {
	err                   error
	ListClustersFn        func(*eks.ListClustersInput) (*eks.ListClustersOutput, error)
	DescribeClusterFn     func(*eks.DescribeClusterInput) (*eks.DescribeClusterOutput, error)
	ListNodegroupsFn      func(*eks.ListNodegroupsInput) (*eks.ListNodegroupsOutput, error)
	ListFargateProfilesFn func(*eks.ListFargateProfilesInput) (*eks.ListFargateProfilesOutput, error)
}

func (f eksFake) ListClusters(_ context.Context, in *eks.ListClustersInput, _ ...func(*eks.Options)) (*eks.ListClustersOutput, error) {
	if f.ListClustersFn != nil {
		return f.ListClustersFn(in)
	}
	return &eks.ListClustersOutput{Clusters: []string{"c"}}, f.err
}
func (f eksFake) DescribeCluster(_ context.Context, in *eks.DescribeClusterInput, _ ...func(*eks.Options)) (*eks.DescribeClusterOutput, error) {
	if f.DescribeClusterFn != nil {
		return f.DescribeClusterFn(in)
	}
	return &eks.DescribeClusterOutput{Cluster: &types.Cluster{}}, f.err
}
func (f eksFake) ListNodegroups(_ context.Context, in *eks.ListNodegroupsInput, _ ...func(*eks.Options)) (*eks.ListNodegroupsOutput, error) {
	if f.ListNodegroupsFn != nil {
		return f.ListNodegroupsFn(in)
	}
	return &eks.ListNodegroupsOutput{}, f.err
}
func (f eksFake) ListFargateProfiles(_ context.Context, in *eks.ListFargateProfilesInput, _ ...func(*eks.Options)) (*eks.ListFargateProfilesOutput, error) {
	if f.ListFargateProfilesFn != nil {
		return f.ListFargateProfilesFn(in)
	}
	return &eks.ListFargateProfilesOutput{}, f.err
}
func TestTask7EksSparse(t *testing.T) {
	r, e := collectClusters(context.Background(), eksFake{}, &ec2FakeForEKS{}, Scope{})
	if e != nil {
		t.Fatal(e)
	}
	_ = r
}

type ec2FakeForEKS struct {
	fn func(*ec2.DescribeInstancesInput) (*ec2.DescribeInstancesOutput, error)
}

func (f *ec2FakeForEKS) DescribeInstances(_ context.Context, in *ec2.DescribeInstancesInput, _ ...func(*ec2.Options)) (*ec2.DescribeInstancesOutput, error) {
	if f.fn != nil {
		return f.fn(in)
	}
	return &ec2.DescribeInstancesOutput{}, nil
}
func TestTask7EksError(t *testing.T) {
	ctx := context.Background()
	want := errors.New("denied")
	f := eksFake{err: want}
	_, e := collectClusters(ctx, f, &ec2FakeForEKS{}, Scope{})
	if !errors.Is(e, want) {
		t.Fatalf("got %v", e)
	}
}
func TestTask7ListClustersPages(t *testing.T) {
	ctx := context.Background()
	calls := 0
	f := eksFake{}
	f.ListClustersFn = func(in *eks.ListClustersInput) (*eks.ListClustersOutput, error) {
		calls++
		if calls == 1 {
			return &eks.ListClustersOutput{NextToken: aws.String("next"), Clusters: []string{"c"}}, nil
		}
		if aws.ToString(in.NextToken) != "next" {
			t.Fatal("missing page token")
		}
		return &eks.ListClustersOutput{}, nil
	}
	_, e := collectClusters(ctx, f, &ec2FakeForEKS{}, Scope{})
	if e != nil || calls != 2 {
		t.Fatalf("calls=%d error=%v", calls, e)
	}
}
func TestTask7ListNodegroupsPages(t *testing.T) {
	ctx := context.Background()
	calls := 0
	f := eksFake{}
	f.ListNodegroupsFn = func(in *eks.ListNodegroupsInput) (*eks.ListNodegroupsOutput, error) {
		calls++
		if calls == 1 {
			return &eks.ListNodegroupsOutput{NextToken: aws.String("next"), Nodegroups: []string{"n"}}, nil
		}
		if aws.ToString(in.NextToken) != "next" {
			t.Fatal("missing page token")
		}
		return &eks.ListNodegroupsOutput{}, nil
	}
	_, e := collectClusters(ctx, f, &ec2FakeForEKS{}, Scope{})
	if e != nil || calls != 2 {
		t.Fatalf("calls=%d error=%v", calls, e)
	}
}
func TestTask7ListFargateProfilesPages(t *testing.T) {
	ctx := context.Background()
	calls := 0
	f := eksFake{}
	f.ListFargateProfilesFn = func(in *eks.ListFargateProfilesInput) (*eks.ListFargateProfilesOutput, error) {
		calls++
		if calls == 1 {
			return &eks.ListFargateProfilesOutput{NextToken: aws.String("next"), FargateProfileNames: []string{"f"}}, nil
		}
		if aws.ToString(in.NextToken) != "next" {
			t.Fatal("missing page token")
		}
		return &eks.ListFargateProfilesOutput{}, nil
	}
	_, e := collectClusters(ctx, f, &ec2FakeForEKS{}, Scope{})
	if e != nil || calls != 2 {
		t.Fatalf("calls=%d error=%v", calls, e)
	}
}

func TestTask7ClusterCounts(t *testing.T) {
	f := eksFake{}
	f.ListNodegroupsFn = func(*eks.ListNodegroupsInput) (*eks.ListNodegroupsOutput, error) {
		return &eks.ListNodegroupsOutput{Nodegroups: []string{"n"}}, nil
	}
	f.ListFargateProfilesFn = func(*eks.ListFargateProfilesInput) (*eks.ListFargateProfilesOutput, error) {
		return &eks.ListFargateProfilesOutput{FargateProfileNames: []string{"f"}}, nil
	}
	n := 0
	ec := &ec2FakeForEKS{fn: func(in *ec2.DescribeInstancesInput) (*ec2.DescribeInstancesOutput, error) {
		if len(in.Filters) != 2 || aws.ToString(in.Filters[0].Name) != "tag-key" || in.Filters[0].Values[0] != "kubernetes.io/cluster/c" {
			t.Fatal(in)
		}
		n++
		if n == 2 && aws.ToString(in.NextToken) != "next" {
			t.Fatal(in)
		}
		o := &ec2.DescribeInstancesOutput{Reservations: []ec2types.Reservation{{Instances: []ec2types.Instance{{State: &ec2types.InstanceState{Name: ec2types.InstanceStateNameRunning}}, {State: &ec2types.InstanceState{Name: ec2types.InstanceStateNamePending}}, {State: &ec2types.InstanceState{Name: ec2types.InstanceStateNameStopped}}, {}}}}}
		if n == 1 {
			o.NextToken = aws.String("next")
		}
		return o, nil
	}}
	r, e := collectClusters(context.Background(), f, ec, Scope{})
	if e != nil {
		t.Fatal(e)
	}
	c := r[0].Config
	if c["nodegroup_count"] != 1 || c["fargate_profile_count"] != 1 || c["tagged_instance_count"] != 4 {
		t.Fatal(c)
	}
}
