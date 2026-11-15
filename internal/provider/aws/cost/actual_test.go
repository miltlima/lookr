package cost

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/costexplorer"
	"github.com/aws/aws-sdk-go-v2/service/costexplorer/types"
	"lookr/internal/model"
)

type actualFake struct {
	outputs []*costexplorer.GetCostAndUsageWithResourcesOutput
	inputs  []*costexplorer.GetCostAndUsageWithResourcesInput
	err     error
}

func (f *actualFake) GetCostAndUsageWithResources(_ context.Context, in *costexplorer.GetCostAndUsageWithResourcesInput, _ ...func(*costexplorer.Options)) (*costexplorer.GetCostAndUsageWithResourcesOutput, error) {
	copy := *in
	f.inputs = append(f.inputs, &copy)
	if f.err != nil {
		return nil, f.err
	}
	out := f.outputs[0]
	f.outputs = f.outputs[1:]
	return out, nil
}
func costGroup(id, amount string) types.Group {
	return types.Group{Keys: []string{id}, Metrics: map[string]types.MetricValue{"UnblendedCost": {Amount: aws.String(amount), Unit: aws.String("USD")}}}
}

func TestFetchActualProjects(t *testing.T) {
	var days []types.ResultByTime
	for i := 0; i < 14; i++ {
		groups := []types.Group{costGroup("i-1", "1.00")}
		if i < 7 {
			groups = append(groups, costGroup("i-2", "2.00"))
		}
		days = append(days, types.ResultByTime{Groups: groups})
	}
	f := &actualFake{outputs: []*costexplorer.GetCostAndUsageWithResourcesOutput{{ResultsByTime: days}}}
	a, calls, err := FetchActual(context.Background(), f, time.Date(2026, 11, 15, 12, 0, 0, 0, time.UTC))
	if err != nil || calls != 1 || a["i-1"] != 30 || a["i-2"] != 30 {
		t.Fatalf("%v %d %v", a, calls, err)
	}
}
func TestFetchActualRequestAndPagination(t *testing.T) {
	f := &actualFake{outputs: []*costexplorer.GetCostAndUsageWithResourcesOutput{{NextPageToken: aws.String("next"), ResultsByTime: []types.ResultByTime{{Groups: []types.Group{costGroup("i-1", "7")}}}}, {ResultsByTime: []types.ResultByTime{{Groups: []types.Group{costGroup("i-1", "7"), costGroup("", "40"), costGroup("NoResourceId", "50")}}}}}}
	// UTC date differs from the local date, so the request must normalize first.
	now := time.Date(2026, 11, 15, 23, 30, 0, 0, time.FixedZone("local", -3*3600))
	a, calls, err := FetchActual(context.Background(), f, now)
	if err != nil || calls != 2 || a["i-1"] != 30 || len(a) != 1 {
		t.Fatalf("%v %d %v", a, calls, err)
	}
	in := f.inputs[0]
	if aws.ToString(in.TimePeriod.Start) != "2026-11-02" || aws.ToString(in.TimePeriod.End) != "2026-11-16" || in.Granularity != types.GranularityDaily || len(in.Metrics) != 1 || in.Metrics[0] != "UnblendedCost" || len(in.GroupBy) != 1 || aws.ToString(in.GroupBy[0].Key) != "RESOURCE_ID" || in.GroupBy[0].Type != types.GroupDefinitionTypeDimension {
		t.Fatalf("%+v", in)
	}
	if in.Filter == nil || in.Filter.Dimensions == nil || in.Filter.Dimensions.Key != types.DimensionService || len(in.Filter.Dimensions.Values) != 1 || in.Filter.Dimensions.Values[0] != "Amazon Elastic Compute Cloud - Compute" {
		t.Fatalf("missing required service filter: %+v", in.Filter)
	}
	if aws.ToString(f.inputs[1].NextPageToken) != "next" || in.NextPageToken != nil {
		t.Fatal(f.inputs)
	}
}
func TestFetchActualError(t *testing.T) {
	want := errors.New("AccessDenied")
	f := &actualFake{err: want}
	a, n, err := FetchActual(context.Background(), f, time.Now())
	if a != nil || n != 1 || !errors.Is(err, want) {
		t.Fatalf("%v %d %v", a, n, err)
	}
}
func TestLookupAndApplyActual(t *testing.T) {
	a := Actual{"arn:test": 12.345, "i-1": 50}
	r := model.Resource{ARN: "arn:test", ID: "i-1"}
	v, ok := a.Lookup(r)
	if !ok || v != 12.345 {
		t.Fatal(v, ok)
	}
	ApplyActual(&r, v)
	if r.Cost.Source != model.CostActual || r.Cost.MonthlyUSD == nil || *r.Cost.MonthlyUSD != 12.35 || r.Cost.Note != "projected from last 14 days" {
		t.Fatal(r.Cost)
	}
	r.ARN = "missing"
	if v, ok = a.Lookup(r); !ok || v != 50 {
		t.Fatal(v, ok)
	}
	r.ID = "none"
	if _, ok = a.Lookup(r); ok {
		t.Fatal("unexpected match")
	}
}
func TestFetchActualInvalidAmount(t *testing.T) {
	for _, amount := range []string{"invalid", "NaN", "+Inf"} {
		f := &actualFake{outputs: []*costexplorer.GetCostAndUsageWithResourcesOutput{{ResultsByTime: []types.ResultByTime{{Groups: []types.Group{costGroup("i-1", amount)}}}}}}
		if a, _, err := FetchActual(context.Background(), f, time.Now()); err == nil || a != nil {
			t.Fatalf("%s: %v %v", amount, a, err)
		}
	}
}
