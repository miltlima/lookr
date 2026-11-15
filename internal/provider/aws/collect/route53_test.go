package collect

import (
	"context"
	"errors"
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/route53"
	"github.com/aws/aws-sdk-go-v2/service/route53/types"
	"testing"
)

var _ = aws.String
var _ = types.HostedZone{}

type route53Fake struct {
	err                   error
	ListHostedZonesFn     func(*route53.ListHostedZonesInput) (*route53.ListHostedZonesOutput, error)
	ListTagsForResourceFn func(*route53.ListTagsForResourceInput) (*route53.ListTagsForResourceOutput, error)
}

func (f route53Fake) ListHostedZones(_ context.Context, in *route53.ListHostedZonesInput, _ ...func(*route53.Options)) (*route53.ListHostedZonesOutput, error) {
	if f.ListHostedZonesFn != nil {
		return f.ListHostedZonesFn(in)
	}
	return &route53.ListHostedZonesOutput{HostedZones: []types.HostedZone{{Id: aws.String("/hostedzone/z")}}}, f.err
}
func (f route53Fake) ListTagsForResource(_ context.Context, in *route53.ListTagsForResourceInput, _ ...func(*route53.Options)) (*route53.ListTagsForResourceOutput, error) {
	if f.ListTagsForResourceFn != nil {
		return f.ListTagsForResourceFn(in)
	}
	return &route53.ListTagsForResourceOutput{ResourceTagSet: &types.ResourceTagSet{}}, f.err
}
func TestTask7Route53Sparse(t *testing.T) {
	r, e := collectHostedZones(context.Background(), route53Fake{}, Scope{})
	if e != nil {
		t.Fatal(e)
	}
	_ = r
}
func TestTask7Route53Error(t *testing.T) {
	ctx := context.Background()
	want := errors.New("denied")
	f := route53Fake{err: want}
	_, e := collectHostedZones(ctx, f, Scope{})
	if !errors.Is(e, want) {
		t.Fatalf("got %v", e)
	}
}
func TestTask7ListHostedZonesPages(t *testing.T) {
	ctx := context.Background()
	calls := 0
	f := route53Fake{}
	f.ListHostedZonesFn = func(in *route53.ListHostedZonesInput) (*route53.ListHostedZonesOutput, error) {
		calls++
		if calls == 1 {
			return &route53.ListHostedZonesOutput{IsTruncated: true, NextMarker: aws.String("next"), HostedZones: []types.HostedZone{{Id: aws.String("/hostedzone/z")}}}, nil
		}
		if aws.ToString(in.Marker) != "next" {
			t.Fatal("missing page token")
		}
		return &route53.ListHostedZonesOutput{}, nil
	}
	_, e := collectHostedZones(ctx, f, Scope{})
	if e != nil || calls != 2 {
		t.Fatalf("calls=%d error=%v", calls, e)
	}
}
func TestTask7ZoneConfig(t *testing.T) {
	r, e := collectHostedZones(context.Background(), route53Fake{}, Scope{Region: "us-west-2"})
	if e != nil || len(r) != 1 || r[0].ID != "z" || r[0].ARN != "arn:aws:route53:::hostedzone/z" || r[0].Region != "global" || r[0].Config["private"] != false {
		t.Fatalf("%v %v", r, e)
	}
}
