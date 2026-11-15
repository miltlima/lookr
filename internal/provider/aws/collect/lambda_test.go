package collect

import (
	"context"
	"errors"
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/lambda"
	"github.com/aws/aws-sdk-go-v2/service/lambda/types"
	"testing"
)

var _ = aws.String
var _ = types.FunctionConfiguration{}

type lambdaFake struct {
	err             error
	ListFunctionsFn func(*lambda.ListFunctionsInput) (*lambda.ListFunctionsOutput, error)
	ListTagsFn      func(*lambda.ListTagsInput) (*lambda.ListTagsOutput, error)
}

func (f lambdaFake) ListFunctions(_ context.Context, in *lambda.ListFunctionsInput, _ ...func(*lambda.Options)) (*lambda.ListFunctionsOutput, error) {
	if f.ListFunctionsFn != nil {
		return f.ListFunctionsFn(in)
	}
	return &lambda.ListFunctionsOutput{Functions: []types.FunctionConfiguration{{FunctionName: aws.String("f")}}}, f.err
}
func (f lambdaFake) ListTags(_ context.Context, in *lambda.ListTagsInput, _ ...func(*lambda.Options)) (*lambda.ListTagsOutput, error) {
	if f.ListTagsFn != nil {
		return f.ListTagsFn(in)
	}
	return &lambda.ListTagsOutput{}, f.err
}
func TestTask7LambdaSparse(t *testing.T) {
	r, e := collectFunctions(context.Background(), lambdaFake{}, Scope{})
	if e != nil {
		t.Fatal(e)
	}
	_ = r
}
func TestTask7LambdaError(t *testing.T) {
	ctx := context.Background()
	want := errors.New("denied")
	f := lambdaFake{err: want}
	_, e := collectFunctions(ctx, f, Scope{})
	if !errors.Is(e, want) {
		t.Fatalf("got %v", e)
	}
}
func TestTask7ListFunctionsPages(t *testing.T) {
	ctx := context.Background()
	calls := 0
	f := lambdaFake{}
	f.ListFunctionsFn = func(in *lambda.ListFunctionsInput) (*lambda.ListFunctionsOutput, error) {
		calls++
		if calls == 1 {
			return &lambda.ListFunctionsOutput{NextMarker: aws.String("next"), Functions: []types.FunctionConfiguration{{FunctionName: aws.String("f")}}}, nil
		}
		if aws.ToString(in.Marker) != "next" {
			t.Fatal("missing page token")
		}
		return &lambda.ListFunctionsOutput{}, nil
	}
	_, e := collectFunctions(ctx, f, Scope{})
	if e != nil || calls != 2 {
		t.Fatalf("calls=%d error=%v", calls, e)
	}
}
func TestTask7FunctionConfig(t *testing.T) {
	f := lambdaFake{}
	f.ListFunctionsFn = func(*lambda.ListFunctionsInput) (*lambda.ListFunctionsOutput, error) {
		return &lambda.ListFunctionsOutput{Functions: []types.FunctionConfiguration{{FunctionName: aws.String("f"), FunctionArn: aws.String("arn:f"), Architectures: []types.Architecture{types.ArchitectureArm64}, MemorySize: aws.Int32(512)}}}, nil
	}
	f.ListTagsFn = func(*lambda.ListTagsInput) (*lambda.ListTagsOutput, error) {
		return &lambda.ListTagsOutput{Tags: map[string]string{"Name": "tagged"}}, nil
	}
	r, e := collectFunctions(context.Background(), f, Scope{})
	if e != nil || len(r) != 1 || r[0].Name != "tagged" || r[0].ARN != "arn:f" || r[0].Config["architecture"] != "arm64" || r[0].Config["memory_mb"] != 512 {
		t.Fatalf("%v %v", r, e)
	}
	f.ListTagsFn = func(*lambda.ListTagsInput) (*lambda.ListTagsOutput, error) { return nil, errors.New("tags denied") }
	if _, e = collectFunctions(context.Background(), f, Scope{}); e == nil {
		t.Fatal("expected tag error")
	}
}
