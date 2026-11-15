package collect

import (
	"context"
	"errors"
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/cloudfront"
	"github.com/aws/aws-sdk-go-v2/service/cloudfront/types"
	"testing"
)

var _ = aws.String
var _ = types.Tags{}

type cloudfrontFake struct {
	err                   error
	ListDistributionsFn   func(*cloudfront.ListDistributionsInput) (*cloudfront.ListDistributionsOutput, error)
	ListTagsForResourceFn func(*cloudfront.ListTagsForResourceInput) (*cloudfront.ListTagsForResourceOutput, error)
}

func (f cloudfrontFake) ListDistributions(_ context.Context, in *cloudfront.ListDistributionsInput, _ ...func(*cloudfront.Options)) (*cloudfront.ListDistributionsOutput, error) {
	if f.ListDistributionsFn != nil {
		return f.ListDistributionsFn(in)
	}
	return &cloudfront.ListDistributionsOutput{}, f.err
}
func (f cloudfrontFake) ListTagsForResource(_ context.Context, in *cloudfront.ListTagsForResourceInput, _ ...func(*cloudfront.Options)) (*cloudfront.ListTagsForResourceOutput, error) {
	if f.ListTagsForResourceFn != nil {
		return f.ListTagsForResourceFn(in)
	}
	return &cloudfront.ListTagsForResourceOutput{Tags: &types.Tags{}}, f.err
}
func TestTask7CloudfrontSparse(t *testing.T) {
	r, e := collectDistributions(context.Background(), cloudfrontFake{}, Scope{})
	if e != nil {
		t.Fatal(e)
	}
	_ = r
}
func TestTask7CloudfrontError(t *testing.T) {
	ctx := context.Background()
	want := errors.New("denied")
	f := cloudfrontFake{err: want}
	_, e := collectDistributions(ctx, f, Scope{})
	if !errors.Is(e, want) {
		t.Fatalf("got %v", e)
	}
}
func TestTask7DistributionPages(t *testing.T) {
	calls := 0
	f := cloudfrontFake{}
	f.ListDistributionsFn = func(in *cloudfront.ListDistributionsInput) (*cloudfront.ListDistributionsOutput, error) {
		calls++
		if calls == 1 {
			return &cloudfront.ListDistributionsOutput{DistributionList: &types.DistributionList{IsTruncated: aws.Bool(true), NextMarker: aws.String("next"), Items: []types.DistributionSummary{{Id: aws.String("d"), DefaultCacheBehavior: &types.DefaultCacheBehavior{ViewerProtocolPolicy: types.ViewerProtocolPolicyHttpsOnly}, ViewerCertificate: &types.ViewerCertificate{MinimumProtocolVersion: types.MinimumProtocolVersionTLSv122021}}}}}, nil
		}
		if aws.ToString(in.Marker) != "next" {
			t.Fatal(in)
		}
		return &cloudfront.ListDistributionsOutput{}, nil
	}
	r, e := collectDistributions(context.Background(), f, Scope{})
	if e != nil || len(r) != 1 || calls != 2 {
		t.Fatalf("%v %v", r, e)
	}
	if r[0].Region != "global" || r[0].Config["viewer_protocol_policy"] != "https-only" || r[0].Config["min_tls_version"] != "TLSv1.2_2021" {
		t.Fatal(r)
	}
}
