package collect

import (
	"context"
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/route53"
	"github.com/aws/aws-sdk-go-v2/service/route53/types"
	"lookr/internal/model"
	"strings"
)

type hostedZoneAPI interface {
	ListHostedZones(context.Context, *route53.ListHostedZonesInput, ...func(*route53.Options)) (*route53.ListHostedZonesOutput, error)
	ListTagsForResource(context.Context, *route53.ListTagsForResourceInput, ...func(*route53.Options)) (*route53.ListTagsForResourceOutput, error)
}

func collectHostedZones(ctx context.Context, c hostedZoneAPI, s Scope) ([]model.Resource, error) {
	s.Region = "global"
	var result []model.Resource
	in := &route53.ListHostedZonesInput{}
	for {
		o, e := c.ListHostedZones(ctx, in)
		if e != nil {
			return result, e
		}
		for _, v := range o.HostedZones {
			id := strings.TrimPrefix(aws.ToString(v.Id), "/hostedzone/")
			t, e := c.ListTagsForResource(ctx, &route53.ListTagsForResourceInput{ResourceId: aws.String(id), ResourceType: types.TagResourceTypeHostedzone})
			if e != nil {
				return result, e
			}
			tags := map[string]string{}
			if t.ResourceTagSet != nil {
				for _, x := range t.ResourceTagSet.Tags {
					tags[aws.ToString(x.Key)] = aws.ToString(x.Value)
				}
			}
			private := false
			if v.Config != nil {
				private = v.Config.PrivateZone
			}
			r := resource(s, "aws.route53.hostedzone", "route53", id, "", tags, map[string]any{"private": private, "record_count": int(aws.ToInt64(v.ResourceRecordSetCount))})
			r.ARN = "arn:aws:route53:::hostedzone/" + id
			if tags["Name"] == "" {
				r.Name = aws.ToString(v.Name)
			}
			result = append(result, r)
		}
		if !o.IsTruncated {
			break
		}
		in.Marker = o.NextMarker
	}
	return result, nil
}
