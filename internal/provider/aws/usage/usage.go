// Package usage describes CloudWatch queries and evaluates resource activity.
package usage

import (
	"lookr/internal/model"
	"time"
)

type Query struct {
	Key           string
	Namespace     string
	Metric        string
	Stat          string
	Dimensions    map[string]string
	ZeroIfMissing bool
	Region        string
}
type Metrics map[string]float64
type Result struct {
	Metrics Metrics
	Err     error
}
type Rule struct {
	Queries  func(model.Resource) []Query
	Evaluate func(model.Resource, Metrics, time.Time, int) (model.Verdict, string)
}

func QueriesFor(r model.Resource) []Query {
	rule, ok := Rules()[r.Type]
	if !ok || rule.Queries == nil {
		return nil
	}
	return rule.Queries(r)
}
func Apply(r *model.Resource, res Result, now time.Time, days int) {
	r.Usage = model.Usage{WindowDays: days, Metrics: res.Metrics}
	if r.Usage.Metrics == nil {
		r.Usage.Metrics = map[string]float64{}
	}
	rule, ok := Rules()[r.Type]
	switch {
	case !ok:
		r.Usage.Verdict = model.VerdictNotEvaluated
		r.Usage.Reason = "inventory only"
	case r.CreatedAt != nil && now.Sub(*r.CreatedAt) < MinAgeDays*24*time.Hour:
		r.Usage.Verdict = model.VerdictUnknown
		r.Usage.Reason = "too new to evaluate"
	case len(QueriesFor(*r)) > 0 && res.Err != nil:
		r.Usage.Verdict = model.VerdictUnknown
		r.Usage.Reason = "metrics unavailable: " + res.Err.Error()
	default:
		r.Usage.Verdict, r.Usage.Reason = rule.Evaluate(*r, res.Metrics, now, days)
	}
}
