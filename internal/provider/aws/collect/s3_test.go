package collect

import (
	"context"
	"errors"
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/aws-sdk-go-v2/service/s3/types"
	"github.com/aws/smithy-go"
	"testing"
)

type bucketFake struct {
	calls   int
	regions []string
	deny    bool
	missing bool
	inputs  []*s3.ListBucketsInput
	block   types.PublicAccessBlockConfiguration
}

func (f *bucketFake) ListBuckets(_ context.Context, in *s3.ListBucketsInput, _ ...func(*s3.Options)) (*s3.ListBucketsOutput, error) {
	copy := *in
	f.inputs = append(f.inputs, &copy)
	f.calls++
	o := &s3.ListBucketsOutput{Buckets: []types.Bucket{{Name: aws.String("bucket")}}}
	if f.calls == 1 {
		o.ContinuationToken = aws.String("next")
	}
	return o, nil
}

func TestBucketPaginationRequest(t *testing.T) {
	f := &bucketFake{}
	_, err := collectBuckets(context.Background(), f, func(string) bucketRegionAPI { return f }, Scope{})
	if err != nil {
		t.Fatal(err)
	}
	if len(f.inputs) != 2 || aws.ToInt32(f.inputs[0].MaxBuckets) <= 0 || aws.ToString(f.inputs[0].ContinuationToken) != "" || aws.ToString(f.inputs[1].ContinuationToken) != "next" {
		t.Fatalf("paginated request required: %+v", f.inputs)
	}
}
func (f *bucketFake) GetBucketLocation(context.Context, *s3.GetBucketLocationInput, ...func(*s3.Options)) (*s3.GetBucketLocationOutput, error) {
	if f.deny {
		return nil, errors.New("AccessDenied")
	}
	return &s3.GetBucketLocationOutput{}, nil
}
func (f *bucketFake) GetPublicAccessBlock(context.Context, *s3.GetPublicAccessBlockInput, ...func(*s3.Options)) (*s3.GetPublicAccessBlockOutput, error) {
	if f.missing {
		return nil, &smithy.GenericAPIError{Code: "NoSuchPublicAccessBlockConfiguration"}
	}
	return &s3.GetPublicAccessBlockOutput{PublicAccessBlockConfiguration: &f.block}, nil
}
func TestCollectBuckets(t *testing.T) {
	for _, all := range []bool{false, true} {
		f := &bucketFake{block: types.PublicAccessBlockConfiguration{BlockPublicAcls: aws.Bool(true), BlockPublicPolicy: aws.Bool(true), IgnorePublicAcls: aws.Bool(true), RestrictPublicBuckets: aws.Bool(all)}}
		factory := func(region string) bucketRegionAPI { f.regions = append(f.regions, region); return f }
		r, e := collectBuckets(context.Background(), f, factory, Scope{})
		if e != nil || len(r) != 2 || r[0].Region != "global" || r[0].Str("bucket_region") != "us-east-1" || r[0].Bool("public_access_blocked") != all {
			t.Fatalf("%v %v", r, e)
		}
		if len(f.regions) != 2 || f.regions[0] != "us-east-1" {
			t.Fatal(f.regions)
		}
	}
	f := &bucketFake{deny: true}
	r, e := collectBuckets(context.Background(), f, func(string) bucketRegionAPI { return f }, Scope{})
	if e == nil || len(r) != 2 || r[0].Str("bucket_region") != "" {
		t.Fatalf("%v %v", r, e)
	}
	if _, ok := r[0].Config["public_access_blocked"]; ok {
		t.Fatal("unknown security represented as false")
	}
	f = &bucketFake{missing: true}
	r, e = collectBuckets(context.Background(), f, func(string) bucketRegionAPI { return f }, Scope{})
	if e != nil || r[0].Bool("public_access_blocked") {
		t.Fatalf("%v %v", r, e)
	}
}

type taggedBucketFake struct{ bucketFake }

func (*taggedBucketFake) GetBucketTagging(context.Context, *s3.GetBucketTaggingInput, ...func(*s3.Options)) (*s3.GetBucketTaggingOutput, error) {
	return &s3.GetBucketTaggingOutput{TagSet: []types.Tag{{Key: aws.String("Name"), Value: aws.String("named")}}}, nil
}
func TestBucketTags(t *testing.T) {
	f := &taggedBucketFake{}
	r, e := collectBuckets(context.Background(), f, func(string) bucketRegionAPI { return f }, Scope{})
	if e != nil || r[0].Name != "named" {
		t.Fatalf("%v %v", r, e)
	}
}

type mixedBucketFake struct{ bucketFake }

func (*mixedBucketFake) ListBuckets(context.Context, *s3.ListBucketsInput, ...func(*s3.Options)) (*s3.ListBucketsOutput, error) {
	return &s3.ListBucketsOutput{Buckets: []types.Bucket{{Name: aws.String("denied")}, {Name: aws.String("allowed")}}}, nil
}
func (*mixedBucketFake) GetBucketLocation(_ context.Context, in *s3.GetBucketLocationInput, _ ...func(*s3.Options)) (*s3.GetBucketLocationOutput, error) {
	if aws.ToString(in.Bucket) == "denied" {
		return nil, errors.New("AccessDenied")
	}
	return &s3.GetBucketLocationOutput{LocationConstraint: types.BucketLocationConstraintEuWest1}, nil
}
func TestBucketPartialAccessDenied(t *testing.T) {
	f := &mixedBucketFake{}
	r, e := collectBuckets(context.Background(), f, func(region string) bucketRegionAPI {
		if region != "eu-west-1" {
			t.Fatal(region)
		}
		return f
	}, Scope{})
	if e == nil || len(r) != 2 || r[0].Str("bucket_region") != "" || r[1].Str("bucket_region") != "eu-west-1" {
		t.Fatalf("%v %v", r, e)
	}
}

type blockedBucketFake struct{ bucketFake }

func (*blockedBucketFake) GetPublicAccessBlock(context.Context, *s3.GetPublicAccessBlockInput, ...func(*s3.Options)) (*s3.GetPublicAccessBlockOutput, error) {
	return nil, errors.New("AccessDenied")
}
func TestBucketSecurityUnknownOnError(t *testing.T) {
	f := &blockedBucketFake{}
	r, e := collectBuckets(context.Background(), f, func(string) bucketRegionAPI { return f }, Scope{})
	if e == nil || len(r) != 2 {
		t.Fatalf("%v %v", r, e)
	}
	if _, ok := r[0].Config["public_access_blocked"]; ok {
		t.Fatal("unknown security must be absent")
	}
}
