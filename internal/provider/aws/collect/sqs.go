package collect

import (
	"context"
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/sqs"
	"github.com/aws/aws-sdk-go-v2/service/sqs/types"
	"lookr/internal/model"
	"strconv"
	"strings"
)

type queueAPI interface {
	ListQueues(context.Context, *sqs.ListQueuesInput, ...func(*sqs.Options)) (*sqs.ListQueuesOutput, error)
	GetQueueAttributes(context.Context, *sqs.GetQueueAttributesInput, ...func(*sqs.Options)) (*sqs.GetQueueAttributesOutput, error)
}

func collectQueues(ctx context.Context, c queueAPI, s Scope) ([]model.Resource, error) {
	var result []model.Resource
	in := &sqs.ListQueuesInput{MaxResults: aws.Int32(1000)}
	for {
		o, e := c.ListQueues(ctx, in)
		if e != nil {
			return result, e
		}
		for _, url := range o.QueueUrls {
			d, e := c.GetQueueAttributes(ctx, &sqs.GetQueueAttributesInput{QueueUrl: aws.String(url), AttributeNames: []types.QueueAttributeName{types.QueueAttributeNameQueueArn, types.QueueAttributeNameVisibilityTimeout, types.QueueAttributeNameApproximateNumberOfMessages, types.QueueAttributeNameMessageRetentionPeriod, types.QueueAttributeNameSqsManagedSseEnabled, types.QueueAttributeNameKmsMasterKeyId, types.QueueAttributeNameRedrivePolicy}})
			if e != nil {
				return result, e
			}
			a := d.Attributes
			number := func(key string) int { v, _ := strconv.Atoi(a[key]); return v }
			id := url[strings.LastIndex(url, "/")+1:]
			r := resource(s, "aws.sqs.queue", "sqs", id, "", nil, map[string]any{"url": url, "fifo": strings.HasSuffix(id, ".fifo"), "visibility_timeout_s": number("VisibilityTimeout"), "approximate_messages": number("ApproximateNumberOfMessages"), "retention_s": number("MessageRetentionPeriod"), "encrypted": a["SqsManagedSseEnabled"] == "true" || a["KmsMasterKeyId"] != "", "has_dlq": a["RedrivePolicy"] != ""})
			r.ARN = a["QueueArn"]
			if tc, ok := c.(queueTagAPI); ok {
				tagged, err := tc.ListQueueTags(ctx, &sqs.ListQueueTagsInput{QueueUrl: aws.String(url)})
				if err != nil {
					result = append(result, r)
					return result, err
				}
				tags := tagged.Tags
				r.Tags = tags
				if tags["Name"] != "" {
					r.Name = tags["Name"]
				}
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

type queueTagAPI interface {
	ListQueueTags(context.Context, *sqs.ListQueueTagsInput, ...func(*sqs.Options)) (*sqs.ListQueueTagsOutput, error)
}
