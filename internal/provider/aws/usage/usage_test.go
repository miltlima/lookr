package usage

import (
	"errors"
	"lookr/internal/model"
	"reflect"
	"testing"
	"time"
)

func TestApply(t *testing.T) {
	now := time.Date(2026, 11, 15, 0, 0, 0, 0, time.UTC)
	young := now.Add(-6 * 24 * time.Hour)
	old := now.Add(-7 * 24 * time.Hour)
	tests := []struct {
		name, typ string
		created   *time.Time
		err       error
		want      model.Verdict
		reason    string
	}{
		{"no rule", "aws.iam.role", &young, errors.New("denied"), model.VerdictNotEvaluated, "inventory only"},
		{"too new", "aws.ebs.volume", &young, nil, model.VerdictUnknown, "too new to evaluate"},
		{"seven days", "aws.ebs.volume", &old, nil, model.VerdictOrphan, "not attached to any instance"},
		{"metrics error", "aws.lambda.function", nil, errors.New("denied"), model.VerdictUnknown, "metrics unavailable: denied"},
		{"error ignored without queries", "aws.ebs.volume", nil, errors.New("denied"), model.VerdictOrphan, "not attached to any instance"},
		{"nil creation", "aws.ebs.volume", nil, nil, model.VerdictOrphan, "not attached to any instance"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := model.Resource{Type: tt.typ, CreatedAt: tt.created, Config: map[string]any{"state": "available"}}
			Apply(&r, Result{Err: tt.err}, now, 30)
			if r.Usage.Verdict != tt.want || r.Usage.Reason != tt.reason {
				t.Fatalf("got %+v", r.Usage)
			}
			if r.Usage.WindowDays != 30 || r.Usage.Metrics == nil {
				t.Fatal("missing window or metrics")
			}
		})
	}
	r := model.Resource{}
	m := Metrics{"sample": 2}
	Apply(&r, Result{Metrics: m}, now, 12)
	if !reflect.DeepEqual(r.Usage.Metrics, map[string]float64(m)) {
		t.Fatal(r.Usage)
	}
}
