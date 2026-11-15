package collect

import (
	"context"
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/cloudfront"
	"lookr/internal/model"
)

type distributionAPI interface {
	ListDistributions(context.Context, *cloudfront.ListDistributionsInput, ...func(*cloudfront.Options)) (*cloudfront.ListDistributionsOutput, error)
	ListTagsForResource(context.Context, *cloudfront.ListTagsForResourceInput, ...func(*cloudfront.Options)) (*cloudfront.ListTagsForResourceOutput, error)
}

func collectDistributions(ctx context.Context, c distributionAPI, s Scope) ([]model.Resource, error) {
	s.Region = "global"
	var result []model.Resource
	in := &cloudfront.ListDistributionsInput{}
	for {
		o, e := c.ListDistributions(ctx, in)
		if e != nil {
			return result, e
		}
		if o.DistributionList == nil {
			break
		}
		for _, v := range o.DistributionList.Items {
			t, e := c.ListTagsForResource(ctx, &cloudfront.ListTagsForResourceInput{Resource: v.ARN})
			if e != nil {
				return result, e
			}
			tags := map[string]string{}
			if t.Tags != nil {
				for _, x := range t.Tags.Items {
					tags[aws.ToString(x.Key)] = aws.ToString(x.Value)
				}
			}
			aliases := []string{}
			if v.Aliases != nil {
				aliases = append(aliases, v.Aliases.Items...)
			}
			policy, tls := "", ""
			if v.DefaultCacheBehavior != nil {
				policy = string(v.DefaultCacheBehavior.ViewerProtocolPolicy)
			}
			if v.ViewerCertificate != nil {
				tls = string(v.ViewerCertificate.MinimumProtocolVersion)
			}
			r := resource(s, "aws.cloudfront.distribution", "cloudfront", aws.ToString(v.Id), "", tags, map[string]any{"domain_name": aws.ToString(v.DomainName), "status": aws.ToString(v.Status), "enabled": aws.ToBool(v.Enabled), "aliases": aliases, "price_class": string(v.PriceClass), "viewer_protocol_policy": policy, "min_tls_version": tls})
			r.ARN = aws.ToString(v.ARN)
			result = append(result, r)
		}
		if !aws.ToBool(o.DistributionList.IsTruncated) {
			break
		}
		in.Marker = o.DistributionList.NextMarker
	}
	return result, nil
}
