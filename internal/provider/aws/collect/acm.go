package collect

import (
	"context"
	"fmt"
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/acm"
	"github.com/aws/aws-sdk-go-v2/service/acm/types"
	"lookr/internal/model"
	"time"
)

type certificateAPI interface {
	ListCertificates(context.Context, *acm.ListCertificatesInput, ...func(*acm.Options)) (*acm.ListCertificatesOutput, error)
	DescribeCertificate(context.Context, *acm.DescribeCertificateInput, ...func(*acm.Options)) (*acm.DescribeCertificateOutput, error)
	ListTagsForCertificate(context.Context, *acm.ListTagsForCertificateInput, ...func(*acm.Options)) (*acm.ListTagsForCertificateOutput, error)
}

func collectCertificates(ctx context.Context, c certificateAPI, s Scope) ([]model.Resource, error) {
	var result []model.Resource
	// Defaults omit non-RSA and ACME certificates; inventory every supported
	// key algorithm and key-pair origin explicitly.
	in := &acm.ListCertificatesInput{Includes: &types.Filters{KeyTypes: types.KeyAlgorithm("").Values()}, CertificateKeyPairOrigins: types.CertificateKeyPairOrigin("").Values()}
	for {
		o, e := c.ListCertificates(ctx, in)
		if e != nil {
			return result, e
		}
		for _, v := range o.CertificateSummaryList {
			d, e := c.DescribeCertificate(ctx, &acm.DescribeCertificateInput{CertificateArn: v.CertificateArn})
			if e != nil {
				return result, e
			}
			if d.Certificate == nil {
				return result, fmt.Errorf("certificate %s: missing description", aws.ToString(v.CertificateArn))
			}
			v := d.Certificate
			t, e := c.ListTagsForCertificate(ctx, &acm.ListTagsForCertificateInput{CertificateArn: v.CertificateArn})
			if e != nil {
				return result, e
			}
			tags := map[string]string{}
			for _, x := range t.Tags {
				tags[aws.ToString(x.Key)] = aws.ToString(x.Value)
			}
			end := ""
			if v.NotAfter != nil {
				end = v.NotAfter.UTC().Format(time.RFC3339)
			}
			id := aws.ToString(v.CertificateArn)
			r := resource(s, "aws.acm.certificate", "acm", id, "", tags, map[string]any{"domain_name": aws.ToString(v.DomainName), "status": string(v.Status), "cert_type": string(v.Type), "in_use_count": len(v.InUseBy), "not_after": end})
			r.ARN = id
			r.CreatedAt = v.CreatedAt
			if tags["Name"] == "" {
				r.Name = aws.ToString(v.DomainName)
			}
			result = append(result, r)
		}
		if aws.ToString(o.NextToken) == "" {
			break
		}
		in.NextToken = o.NextToken
	}
	return result, nil
}
