package usage

import (
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/service/cloudwatch"
	ct "github.com/aws/aws-sdk-go-v2/service/cloudwatch/types"
	"lookr/internal/model"
)

func TestFinalALBTargetlessActionsProveUse(t *testing.T) {
	r := model.Resource{Type: "aws.elb.loadbalancer", Config: map[string]any{"lb_type": "application", "target_count": 0, "metric_dimension": "app/x/y"}}
	for _, tc := range []struct {
		m    Metrics
		want model.Verdict
	}{{Metrics{"activity": 0, "redirects": 5, "fixed_responses": 0}, model.VerdictInUse}, {Metrics{"activity": 0, "redirects": 0, "fixed_responses": 7}, model.VerdictInUse}, {nil, model.VerdictUnknown}, {Metrics{"activity": 0, "redirects": 0, "fixed_responses": 0}, model.VerdictIdle}} {
		Apply(&r, Result{Metrics: tc.m}, time.Now(), 30)
		if r.Usage.Verdict != tc.want {
			t.Errorf("%v got %+v want %s", tc.m, r.Usage, tc.want)
		}
	}
	qs := QueriesFor(r)
	seen := map[string]bool{}
	for _, q := range qs {
		seen[q.Metric] = true
	}
	if !seen["HTTP_Redirect_Count"] || !seen["HTTP_Fixed_Response_Count"] {
		t.Fatal(qs)
	}
}

func TestFinalCacheQueriesIncludeEachNode(t *testing.T) {
	r := model.Resource{Type: "aws.elasticache.cluster", ID: "cache", Config: map[string]any{"engine": "redis", "num_nodes": 2, "cache_node_ids": []string{"0001", "0002"}}}
	qs := QueriesFor(r)
	if len(qs) != 4 {
		t.Fatalf("expected two counters per node: %+v", qs)
	}
	seen := map[string]int{}
	for _, q := range qs {
		if q.Dimensions["CacheClusterId"] != "cache" {
			t.Fatal(q)
		}
		seen[q.Dimensions["CacheNodeId"]]++
	}
	if seen["0001"] != 2 || seen["0002"] != 2 {
		t.Fatal(qs)
	}
	r.Config["cache_node_ids"] = []string{"0001"}
	Apply(&r, Result{Metrics: Metrics{"cmds_get": 0, "cmds_set": 0}}, time.Now(), 30)
	if r.Usage.Verdict != model.VerdictUnknown {
		t.Fatal("missing node inventory became idle", r.Usage)
	}
}

func TestFinalCacheNodeAggregationPreservesMissingData(t *testing.T) {
	for _, missing := range []bool{false, true} {
		f := &metricFake{respond: func(_ int, in *cloudwatch.GetMetricDataInput) (*cloudwatch.GetMetricDataOutput, error) {
			out := &cloudwatch.GetMetricDataOutput{}
			for i, q := range in.MetricDataQueries {
				values := []float64{float64(i + 1)}
				if missing && i == 1 {
					values = nil
				}
				out.MetricDataResults = append(out.MetricDataResults, ct.MetricDataResult{Id: q.Id, Values: values, StatusCode: ct.StatusCodeComplete})
			}
			return out, nil
		}}
		got := fetchTest(f, []Request{{ResourceKey: "cache", Queries: []Query{{Key: "cmds_get", Stat: "Sum"}, {Key: "cmds_get", Stat: "Sum"}}}})["cache"]
		if missing {
			if _, ok := got.Metrics["cmds_get"]; ok {
				t.Fatal("missing node fabricated complete aggregate", got)
			}
		} else if got.Metrics["cmds_get"] != 3 {
			t.Fatal("node sum overwritten", got)
		}
	}
}
