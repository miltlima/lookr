package collect

import (
	"context"
	"errors"
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/acm"
	"github.com/aws/aws-sdk-go-v2/service/acm/types"
	"testing"
)

var _ = aws.String
var _ = types.CertificateDetail{}

type acmFake struct {
	err                      error
	ListCertificatesFn       func(*acm.ListCertificatesInput) (*acm.ListCertificatesOutput, error)
	DescribeCertificateFn    func(*acm.DescribeCertificateInput) (*acm.DescribeCertificateOutput, error)
	ListTagsForCertificateFn func(*acm.ListTagsForCertificateInput) (*acm.ListTagsForCertificateOutput, error)
}

func (f acmFake) ListCertificates(_ context.Context, in *acm.ListCertificatesInput, _ ...func(*acm.Options)) (*acm.ListCertificatesOutput, error) {
	if f.ListCertificatesFn != nil {
		return f.ListCertificatesFn(in)
	}
	return &acm.ListCertificatesOutput{CertificateSummaryList: []types.CertificateSummary{{CertificateArn: aws.String("arn:c")}}}, f.err
}
func (f acmFake) DescribeCertificate(_ context.Context, in *acm.DescribeCertificateInput, _ ...func(*acm.Options)) (*acm.DescribeCertificateOutput, error) {
	if f.DescribeCertificateFn != nil {
		return f.DescribeCertificateFn(in)
	}
	return &acm.DescribeCertificateOutput{Certificate: &types.CertificateDetail{}}, f.err
}
func (f acmFake) ListTagsForCertificate(_ context.Context, in *acm.ListTagsForCertificateInput, _ ...func(*acm.Options)) (*acm.ListTagsForCertificateOutput, error) {
	if f.ListTagsForCertificateFn != nil {
		return f.ListTagsForCertificateFn(in)
	}
	return &acm.ListTagsForCertificateOutput{}, f.err
}
func TestTask7AcmSparse(t *testing.T) {
	r, e := collectCertificates(context.Background(), acmFake{}, Scope{})
	if e != nil {
		t.Fatal(e)
	}
	_ = r
}
func TestTask7AcmError(t *testing.T) {
	ctx := context.Background()
	want := errors.New("denied")
	f := acmFake{err: want}
	_, e := collectCertificates(ctx, f, Scope{})
	if !errors.Is(e, want) {
		t.Fatalf("got %v", e)
	}
}
func TestTask7ListCertificatesPages(t *testing.T) {
	ctx := context.Background()
	calls := 0
	f := acmFake{}
	f.ListCertificatesFn = func(in *acm.ListCertificatesInput) (*acm.ListCertificatesOutput, error) {
		calls++
		if calls == 1 {
			return &acm.ListCertificatesOutput{NextToken: aws.String("next"), CertificateSummaryList: []types.CertificateSummary{{CertificateArn: aws.String("arn:c")}}}, nil
		}
		if aws.ToString(in.NextToken) != "next" {
			t.Fatal("missing page token")
		}
		return &acm.ListCertificatesOutput{}, nil
	}
	_, e := collectCertificates(ctx, f, Scope{})
	if e != nil || calls != 2 {
		t.Fatalf("calls=%d error=%v", calls, e)
	}
}
func TestTask7CertificateConfig(t *testing.T) {
	f := acmFake{}
	f.DescribeCertificateFn = func(*acm.DescribeCertificateInput) (*acm.DescribeCertificateOutput, error) {
		return &acm.DescribeCertificateOutput{Certificate: &types.CertificateDetail{CertificateArn: aws.String("arn:c"), DomainName: aws.String("example.com"), InUseBy: []string{"a", "b"}}}, nil
	}
	r, e := collectCertificates(context.Background(), f, Scope{})
	if e != nil || r[0].ARN != "arn:c" || r[0].Name != "example.com" || r[0].Config["in_use_count"] != 2 || r[0].Config["not_after"] != "" {
		t.Fatalf("%v %v", r, e)
	}
	f.DescribeCertificateFn = func(*acm.DescribeCertificateInput) (*acm.DescribeCertificateOutput, error) {
		return &acm.DescribeCertificateOutput{}, nil
	}
	if _, e = collectCertificates(context.Background(), f, Scope{}); e == nil {
		t.Fatal("expected missing description error")
	}
}
