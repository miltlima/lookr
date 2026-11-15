package usage

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/cloudwatch"
	"github.com/aws/aws-sdk-go-v2/service/cloudwatch/types"
)

type metricFake struct {
	inputs  []*cloudwatch.GetMetricDataInput
	respond func(int, *cloudwatch.GetMetricDataInput) (*cloudwatch.GetMetricDataOutput, error)
}

func (f *metricFake) GetMetricData(_ context.Context, in *cloudwatch.GetMetricDataInput, _ ...func(*cloudwatch.Options)) (*cloudwatch.GetMetricDataOutput, error) {
	f.inputs = append(f.inputs, in)
	return f.respond(len(f.inputs)-1, in)
}
func metricOutput(in *cloudwatch.GetMetricDataInput, values ...float64) *cloudwatch.GetMetricDataOutput {
	out := &cloudwatch.GetMetricDataOutput{}
	for _, q := range in.MetricDataQueries {
		out.MetricDataResults = append(out.MetricDataResults, types.MetricDataResult{Id: q.Id, Values: values, StatusCode: types.StatusCodeComplete})
	}
	return out
}
func fetchTest(f *metricFake, reqs []Request) map[string]Result {
	return Fetch(context.Background(), f, reqs, time.Unix(1, 0), time.Unix(2, 0))
}
func TestFetchAggregates(t *testing.T) {
	for _, tc := range []struct {
		stat   string
		values []float64
		want   float64
	}{{"Sum", []float64{1, 2, 3}, 6}, {"Maximum", []float64{4, 9, 2}, 9}, {"Average", []float64{10, 20}, 15}} {
		t.Run(tc.stat, func(t *testing.T) {
			f := &metricFake{respond: func(_ int, in *cloudwatch.GetMetricDataInput) (*cloudwatch.GetMetricDataOutput, error) {
				return metricOutput(in, tc.values...), nil
			}}
			got := fetchTest(f, []Request{{"r", []Query{{Key: "m", Stat: tc.stat}}}})["r"]
			if got.Err != nil || got.Metrics["m"] != tc.want {
				t.Fatalf("got %+v", got)
			}
		})
	}
}
func TestFetchMissingDatapoints(t *testing.T) {
	f := &metricFake{respond: func(_ int, in *cloudwatch.GetMetricDataInput) (*cloudwatch.GetMetricDataOutput, error) {
		return metricOutput(in), nil
	}}
	got := fetchTest(f, []Request{{"r", []Query{{Key: "zero", Stat: "Sum", ZeroIfMissing: true}, {Key: "absent", Stat: "Average"}}}})["r"]
	if v, ok := got.Metrics["zero"]; !ok || v != 0 || got.Err != nil {
		t.Fatalf("got %+v", got)
	}
	if _, ok := got.Metrics["absent"]; ok {
		t.Fatal("missing metric present")
	}
}
func TestFetchBatches(t *testing.T) {
	reqs := make([]Request, 1201)
	for i := range reqs {
		reqs[i] = Request{fmt.Sprint(i), []Query{{Key: "m", Stat: "Sum"}}}
	}
	f := &metricFake{respond: func(_ int, in *cloudwatch.GetMetricDataInput) (*cloudwatch.GetMetricDataOutput, error) {
		if len(in.MetricDataQueries) > 500 {
			t.Fatal("oversized batch")
		}
		return metricOutput(in, 1), nil
	}}
	got := fetchTest(f, reqs)
	if len(f.inputs) != 3 || len(got) != 1201 {
		t.Fatalf("calls %d results %d", len(f.inputs), len(got))
	}
	for _, r := range got {
		if r.Err != nil || r.Metrics["m"] != 1 {
			t.Fatalf("got %+v", r)
		}
	}
}
func TestFetchPaginates(t *testing.T) {
	f := &metricFake{respond: func(n int, in *cloudwatch.GetMetricDataInput) (*cloudwatch.GetMetricDataOutput, error) {
		out := metricOutput(in, float64(n+1))
		if n == 0 {
			out.NextToken = aws.String("next")
		} else if aws.ToString(in.NextToken) != "next" {
			t.Fatal("token missing")
		}
		return out, nil
	}}
	got := fetchTest(f, []Request{{"r", []Query{{Key: "m", Stat: "Sum"}}}})
	if len(f.inputs) != 2 || got["r"].Metrics["m"] != 3 {
		t.Fatalf("got %+v", got)
	}
}
func TestFetchBatchError(t *testing.T) {
	qs := make([]Query, 501)
	for i := range qs {
		qs[i] = Query{Key: fmt.Sprint(i), Stat: "Sum"}
	}
	f := &metricFake{respond: func(n int, in *cloudwatch.GetMetricDataInput) (*cloudwatch.GetMetricDataOutput, error) {
		if n == 1 {
			return nil, errors.New("denied")
		}
		return metricOutput(in, 2), nil
	}}
	got := fetchTest(f, []Request{{"good", qs[:1]}, {"split", qs}})
	if got["good"].Err != nil || got["good"].Metrics["0"] != 2 || got["split"].Err == nil {
		t.Fatalf("got %+v", got)
	}
}
func TestFetchRequestShape(t *testing.T) {
	f := &metricFake{respond: func(_ int, in *cloudwatch.GetMetricDataInput) (*cloudwatch.GetMetricDataOutput, error) {
		if !in.StartTime.Equal(time.Unix(1, 0)) || !in.EndTime.Equal(time.Unix(2, 0)) {
			t.Fatal("window")
		}
		seen := map[string]bool{}
		for _, q := range in.MetricDataQueries {
			id := aws.ToString(q.Id)
			if !regexp.MustCompile(`^[a-z][a-zA-Z0-9_]*$`).MatchString(id) || seen[id] || !aws.ToBool(q.ReturnData) || aws.ToInt32(q.MetricStat.Period) != 86400 {
				t.Fatalf("bad query %+v", q)
			}
			seen[id] = true
			if aws.ToString(q.MetricStat.Metric.Namespace) != "ns" || aws.ToString(q.MetricStat.Metric.MetricName) != "metric" || len(q.MetricStat.Metric.Dimensions) != 1 {
				t.Fatal("metric shape")
			}
		}
		return metricOutput(in), nil
	}}
	fetchTest(f, []Request{{"r", []Query{{Key: "a", Namespace: "ns", Metric: "metric", Stat: "Sum", Dimensions: map[string]string{"Name": "value"}}, {Key: "b", Namespace: "ns", Metric: "metric", Stat: "Sum", Dimensions: map[string]string{"Name": "value"}}}}})
}
func TestFetchEmpty(t *testing.T) {
	f := &metricFake{}
	if got := fetchTest(f, nil); len(got) != 0 || len(f.inputs) != 0 {
		t.Fatal("expected empty")
	}
}
func TestFetchFailedStatus(t *testing.T) {
	for _, status := range []types.StatusCode{types.StatusCodePartialData, types.StatusCodeInternalError, types.StatusCodeForbidden} {
		t.Run(string(status), func(t *testing.T) {
			f := &metricFake{respond: func(_ int, in *cloudwatch.GetMetricDataInput) (*cloudwatch.GetMetricDataOutput, error) {
				out := metricOutput(in)
				out.MetricDataResults[0].StatusCode = status
				return out, nil
			}}
			got := fetchTest(f, []Request{{"r", []Query{{Key: "m", Stat: "Sum", ZeroIfMissing: true}}}})["r"]
			if got.Err == nil {
				t.Fatalf("failed status became zero: %+v", got)
			}
		})
	}
}

func TestFetchPartialPageCompletes(t *testing.T) {
	f := &metricFake{respond: func(n int, in *cloudwatch.GetMetricDataInput) (*cloudwatch.GetMetricDataOutput, error) {
		out := metricOutput(in, float64(n+1))
		if n == 0 {
			out.NextToken = aws.String("next")
			out.MetricDataResults[0].StatusCode = types.StatusCodePartialData
		}
		return out, nil
	}}
	got := fetchTest(f, []Request{{"r", []Query{{Key: "m", Stat: "Sum", ZeroIfMissing: true}}}})["r"]
	if got.Err != nil || got.Metrics["m"] != 3 {
		t.Fatalf("got %+v", got)
	}
}
