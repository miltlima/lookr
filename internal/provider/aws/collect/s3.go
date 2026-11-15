package collect

import (
	"context"
	"errors"
	"fmt"
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/smithy-go"
	"lookr/internal/model"
)

type bucketAPI interface {
	ListBuckets(context.Context, *s3.ListBucketsInput, ...func(*s3.Options)) (*s3.ListBucketsOutput, error)
	GetBucketLocation(context.Context, *s3.GetBucketLocationInput, ...func(*s3.Options)) (*s3.GetBucketLocationOutput, error)
}
type bucketRegionAPI interface {
	GetPublicAccessBlock(context.Context, *s3.GetPublicAccessBlockInput, ...func(*s3.Options)) (*s3.GetPublicAccessBlockOutput, error)
}

// Unknown bucket facts remain absent rather than becoming false security claims.
func collectBuckets(ctx context.Context, c bucketAPI, regional func(string) bucketRegionAPI, s Scope) ([]model.Resource, error) {
	s.Region = "global"
	var result []model.Resource
	var warnings []error
	in := &s3.ListBucketsInput{}
	for {
		o, e := c.ListBuckets(ctx, in)
		if e != nil {
			return result, errors.Join(append(warnings, e)...)
		}
		for _, b := range o.Buckets {
			id := aws.ToString(b.Name)
			config := map[string]any{"bucket_region": ""}
			r := resource(s, "aws.s3.bucket", "s3", id, "", nil, config)
			r.ARN = "arn:aws:s3:::" + id
			r.CreatedAt = b.CreationDate
			loc, e := c.GetBucketLocation(ctx, &s3.GetBucketLocationInput{Bucket: b.Name})
			if e != nil {
				warnings = append(warnings, fmt.Errorf("bucket %s location: %w", id, e))
				result = append(result, r)
				continue
			}
			region := string(loc.LocationConstraint)
			if region == "" {
				region = "us-east-1"
			}
			if region == "EU" {
				region = "eu-west-1"
			}
			config["bucket_region"] = region
			rc := regional(region)
			if tc, ok := rc.(bucketTagAPI); ok {
				tagged, err := tc.GetBucketTagging(ctx, &s3.GetBucketTaggingInput{Bucket: b.Name})
				if err != nil {
					var api smithy.APIError
					if !errors.As(err, &api) || api.ErrorCode() != "NoSuchTagSet" {
						warnings = append(warnings, fmt.Errorf("bucket %s tags: %w", id, err))
					}
				} else {
					r.Tags = map[string]string{}
					for _, t := range tagged.TagSet {
						r.Tags[aws.ToString(t.Key)] = aws.ToString(t.Value)
					}
					if r.Tags["Name"] != "" {
						r.Name = r.Tags["Name"]
					}
				}
			}
			block, e := rc.GetPublicAccessBlock(ctx, &s3.GetPublicAccessBlockInput{Bucket: b.Name})
			if e != nil {
				var api smithy.APIError
				if errors.As(e, &api) && api.ErrorCode() == "NoSuchPublicAccessBlockConfiguration" {
					config["public_access_blocked"] = false
				} else {
					warnings = append(warnings, fmt.Errorf("bucket %s public access block: %w", id, e))
				}
			} else if block.PublicAccessBlockConfiguration != nil {
				v := block.PublicAccessBlockConfiguration
				config["public_access_blocked"] = aws.ToBool(v.BlockPublicAcls) && aws.ToBool(v.BlockPublicPolicy) && aws.ToBool(v.IgnorePublicAcls) && aws.ToBool(v.RestrictPublicBuckets)
			} else {
				warnings = append(warnings, fmt.Errorf("bucket %s public access block: missing configuration", id))
			}
			result = append(result, r)
		}
		if aws.ToString(o.ContinuationToken) == "" {
			break
		}
		in.ContinuationToken = o.ContinuationToken
	}
	return result, errors.Join(warnings...)
}

type bucketTagAPI interface {
	GetBucketTagging(context.Context, *s3.GetBucketTaggingInput, ...func(*s3.Options)) (*s3.GetBucketTaggingOutput, error)
}
