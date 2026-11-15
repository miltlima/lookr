// Package model defines the data every scan produces: resources with their
// configuration, usage verdict, cost and findings.
package model

import "time"

type Verdict string

const (
	VerdictInUse        Verdict = "in_use"
	VerdictIdle         Verdict = "idle"
	VerdictOrphan       Verdict = "orphan"
	VerdictUnknown      Verdict = "unknown"
	VerdictNotEvaluated Verdict = "not_evaluated"
)

type CostSource string

const (
	CostActual   CostSource = "actual"
	CostEstimate CostSource = "estimate"
	CostNone     CostSource = "none"
)

type Severity string

const (
	SeverityHigh   Severity = "high"
	SeverityMedium Severity = "medium"
	SeverityLow    Severity = "low"
)

type Usage struct {
	Verdict    Verdict            `json:"verdict"`
	Reason     string             `json:"reason"`
	WindowDays int                `json:"window_days"`
	Metrics    map[string]float64 `json:"metrics"`
}

type Cost struct {
	MonthlyUSD *float64   `json:"monthly_usd"`
	Source     CostSource `json:"source"`
	Note       string     `json:"note"`
}

type Finding struct {
	Check          string   `json:"check"`
	Severity       Severity `json:"severity"`
	Message        string   `json:"message"`
	Recommendation string   `json:"recommendation"`
}

type Resource struct {
	Provider  string            `json:"provider"`
	AccountID string            `json:"account_id"`
	Region    string            `json:"region"`
	Type      string            `json:"type"`
	Short     string            `json:"-"`
	ID        string            `json:"id"`
	ARN       string            `json:"arn"`
	Name      string            `json:"name"`
	Tags      map[string]string `json:"tags"`
	CreatedAt *time.Time        `json:"created_at"`
	Config    map[string]any    `json:"config"`
	Usage     Usage             `json:"usage"`
	Cost      Cost              `json:"cost"`
	Findings  []Finding         `json:"findings"`
}

// Key uniquely identifies a resource within a scan.
func (r Resource) Key() string { return r.Type + "|" + r.Region + "|" + r.ID }

// Str returns Config[key] as a string, or "" if absent or of another type.
func (r Resource) Str(key string) string {
	s, _ := r.Config[key].(string)
	return s
}

// Int returns Config[key] as an int, accepting int, int32, int64 and float64;
// 0 if absent or of another type.
func (r Resource) Int(key string) int {
	switch v := r.Config[key].(type) {
	case int:
		return v
	case int32:
		return int(v)
	case int64:
		return int(v)
	case float64:
		return int(v)
	}
	return 0
}

// Float returns Config[key] as a float64, accepting numeric types; 0 otherwise.
func (r Resource) Float(key string) float64 {
	switch v := r.Config[key].(type) {
	case float64:
		return v
	case int:
		return float64(v)
	case int32:
		return float64(v)
	case int64:
		return float64(v)
	}
	return 0
}

// Bool returns Config[key] as a bool; false if absent or of another type.
func (r Resource) Bool(key string) bool {
	b, _ := r.Config[key].(bool)
	return b
}

// Strs returns Config[key] as a []string; nil if absent or of another type.
func (r Resource) Strs(key string) []string {
	s, _ := r.Config[key].([]string)
	return s
}
