package collect

import (
	"context"
	"fmt"
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/sqs"
	"github.com/aws/aws-sdk-go-v2/service/sqs/types"
	"testing"
)

type queueFake struct {
	calls int
	t     *testing.T
	attrs map[string]string
}

func (f *queueFake) ListQueues(_ context.Context, in *sqs.ListQueuesInput, _ ...func(*sqs.Options)) (*sqs.ListQueuesOutput, error) {
	f.calls++
	if in.MaxResults == nil {
		f.t.Fatal("pagination requires MaxResults")
	}
	o := &sqs.ListQueuesOutput{QueueUrls: []string{"https://sqs.r.amazonaws.com/123/q.fifo"}}
	if f.calls == 1 {
		o.NextToken = aws.String("next")
	}
	return o, nil
}
func (f *queueFake) GetQueueAttributes(_ context.Context, in *sqs.GetQueueAttributesInput, _ ...func(*sqs.Options)) (*sqs.GetQueueAttributesOutput, error) {
	found := false
	for _, a := range in.AttributeNames {
		if a == types.QueueAttributeNameQueueArn {
			found = true
		}
	}
	if !found {
		f.t.Fatal("missing QueueArn")
	}
	return &sqs.GetQueueAttributesOutput{Attributes: f.attrs}, nil
}
func TestCollectQueues(t *testing.T) {
	for _, attrs := range []map[string]string{nil, {"SqsManagedSseEnabled": "true", "RedrivePolicy": "{}"}, {"KmsMasterKeyId": "key"}} {
		f := &queueFake{t: t, attrs: attrs}
		r, e := collectQueues(context.Background(), f, Scope{})
		if e != nil || len(r) != 2 || r[0].ID != "q.fifo" || !r[0].Bool("fifo") {
			t.Fatalf("%v %v", r, e)
		}
		if r[0].Bool("encrypted") != (len(attrs) > 0) {
			t.Fatal("encryption")
		}
		if r[0].Bool("has_dlq") != (attrs["RedrivePolicy"] != "") {
			t.Fatal("DLQ")
		}
	}
}

type taggedQueueFake struct{ queueFake }

func (*taggedQueueFake) ListQueueTags(context.Context, *sqs.ListQueueTagsInput, ...func(*sqs.Options)) (*sqs.ListQueueTagsOutput, error) {
	return &sqs.ListQueueTagsOutput{Tags: map[string]string{"Name": "named"}}, nil
}
func TestQueueTags(t *testing.T) {
	r, e := collectQueues(context.Background(), &taggedQueueFake{queueFake: queueFake{t: t}}, Scope{})
	if e != nil || r[0].Name != "named" {
		t.Fatalf("%v %v", r, e)
	}
}

type failedQueueFake struct {
	queueFake
	list bool
}

func (f *failedQueueFake) ListQueues(ctx context.Context, in *sqs.ListQueuesInput, opt ...func(*sqs.Options)) (*sqs.ListQueuesOutput, error) {
	if f.list {
		return nil, fmt.Errorf("denied")
	}
	return f.queueFake.ListQueues(ctx, in, opt...)
}
func (*failedQueueFake) GetQueueAttributes(context.Context, *sqs.GetQueueAttributesInput, ...func(*sqs.Options)) (*sqs.GetQueueAttributesOutput, error) {
	return nil, fmt.Errorf("denied")
}
func TestQueueErrors(t *testing.T) {
	for _, list := range []bool{false, true} {
		if _, e := collectQueues(context.Background(), &failedQueueFake{queueFake: queueFake{t: t}, list: list}, Scope{}); e == nil {
			t.Fatal("missing error")
		}
	}
}
