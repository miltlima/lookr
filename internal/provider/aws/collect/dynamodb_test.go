package collect

import (
	"context"
	"errors"
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
	"testing"
)

type tableFake struct {
	calls int
	fail  bool
}

func (f *tableFake) ListTables(_ context.Context, in *dynamodb.ListTablesInput, _ ...func(*dynamodb.Options)) (*dynamodb.ListTablesOutput, error) {
	f.calls++
	o := &dynamodb.ListTablesOutput{TableNames: []string{"table"}}
	if f.calls == 1 {
		o.LastEvaluatedTableName = aws.String("table")
	}
	return o, nil
}
func (f *tableFake) DescribeTable(context.Context, *dynamodb.DescribeTableInput, ...func(*dynamodb.Options)) (*dynamodb.DescribeTableOutput, error) {
	if f.fail {
		return nil, errors.New("denied")
	}
	return &dynamodb.DescribeTableOutput{Table: &types.TableDescription{}}, nil
}
func TestCollectTables(t *testing.T) {
	f := &tableFake{}
	r, e := collectTables(context.Background(), f, Scope{})
	if e != nil || len(r) != 2 || r[0].Str("billing_mode") != "PROVISIONED" {
		t.Fatalf("%v %v", r, e)
	}
	f = &tableFake{fail: true}
	if _, e = collectTables(context.Background(), f, Scope{}); e == nil {
		t.Fatal("missing describe error")
	}
}

type taggedTableFake struct {
	tableFake
	tagCalls int
}

func (f *taggedTableFake) ListTagsOfResource(_ context.Context, in *dynamodb.ListTagsOfResourceInput, _ ...func(*dynamodb.Options)) (*dynamodb.ListTagsOfResourceOutput, error) {
	f.tagCalls++
	o := &dynamodb.ListTagsOfResourceOutput{Tags: []types.Tag{{Key: aws.String("Name"), Value: aws.String("named")}}}
	if in.NextToken == nil {
		o.NextToken = aws.String("next")
	}
	return o, nil
}
func TestTableTags(t *testing.T) {
	f := &taggedTableFake{}
	r, e := collectTables(context.Background(), f, Scope{})
	if e != nil || r[0].Name != "named" || f.tagCalls != 4 {
		t.Fatalf("%v %v calls %d", r, e, f.tagCalls)
	}
}

type emptyTableFake struct{ tableFake }

func (*emptyTableFake) DescribeTable(context.Context, *dynamodb.DescribeTableInput, ...func(*dynamodb.Options)) (*dynamodb.DescribeTableOutput, error) {
	return &dynamodb.DescribeTableOutput{}, nil
}
func TestMissingTableDescription(t *testing.T) {
	if _, e := collectTables(context.Background(), &emptyTableFake{}, Scope{}); e == nil {
		t.Fatal("missing table must produce error")
	}
}
