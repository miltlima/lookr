package collect

import (
	"context"
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/lambda"
	"lookr/internal/model"
)

type functionAPI interface {
	ListFunctions(context.Context, *lambda.ListFunctionsInput, ...func(*lambda.Options)) (*lambda.ListFunctionsOutput, error)
	ListTags(context.Context, *lambda.ListTagsInput, ...func(*lambda.Options)) (*lambda.ListTagsOutput, error)
}

func collectFunctions(ctx context.Context, c functionAPI, s Scope) ([]model.Resource, error) {
	var result []model.Resource
	in := &lambda.ListFunctionsInput{}
	for {
		o, e := c.ListFunctions(ctx, in)
		if e != nil {
			return result, e
		}
		for _, v := range o.Functions {
			a := "x86_64"
			if len(v.Architectures) > 0 {
				a = string(v.Architectures[0])
			}
			t, e := c.ListTags(ctx, &lambda.ListTagsInput{Resource: v.FunctionArn})
			if e != nil {
				return result, e
			}
			r := resource(s, "aws.lambda.function", "lambda", aws.ToString(v.FunctionName), "", t.Tags, map[string]any{"runtime": string(v.Runtime), "handler": aws.ToString(v.Handler), "memory_mb": int(aws.ToInt32(v.MemorySize)), "timeout_s": int(aws.ToInt32(v.Timeout)), "architecture": a, "code_size_bytes": int(v.CodeSize), "last_modified": aws.ToString(v.LastModified)})
			r.ARN = aws.ToString(v.FunctionArn)
			result = append(result, r)
		}
		if aws.ToString(o.NextMarker) == "" {
			break
		}
		in.Marker = o.NextMarker
	}
	return result, nil
}
