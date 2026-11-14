package model

import (
	"math"
	"sort"
	"time"
)

type Warning struct {
	Region string `json:"region"`
	Type   string `json:"type"`
	Error  string `json:"error"`
}

type Summary struct {
	Resources          int              `json:"resources"`
	MonthlyUSD         float64          `json:"monthly_usd"`
	WasteResources     int              `json:"waste_resources"`
	WasteMonthlyUSD    float64          `json:"waste_monthly_usd"`
	ByVerdict          map[Verdict]int  `json:"by_verdict"`
	FindingsBySeverity map[Severity]int `json:"findings_by_severity"`
}

type Report struct {
	SchemaVersion string     `json:"schema_version"`
	GeneratedAt   time.Time  `json:"generated_at"`
	Provider      string     `json:"provider"`
	AccountID     string     `json:"account_id"`
	WindowDays    int        `json:"window_days"`
	Regions       []string   `json:"regions"`
	Summary       Summary    `json:"summary"`
	Resources     []Resource `json:"resources"`
	Warnings      []Warning  `json:"warnings"`
}

var verdictRank = map[Verdict]int{
	VerdictOrphan:       0,
	VerdictIdle:         1,
	VerdictUnknown:      2,
	VerdictInUse:        3,
	VerdictNotEvaluated: 4,
}

func round2(f float64) float64 { return math.Round(f*100) / 100 }

// NewReport builds a report without modifying the slices it receives: it
// copies the resources, replaces nil maps and slices with empty ones, sorts
// them (orphan, idle, unknown, in_use, not_evaluated; then cost descending
// with unpriced last; then Type and ID) and computes the summary.
func NewReport(provider, accountID string, windowDays int, regions []string, resources []Resource, warnings []Warning, now time.Time) Report {
	res := make([]Resource, len(resources))
	copy(res, resources)
	for i := range res {
		r := &res[i]
		if r.Tags == nil {
			r.Tags = map[string]string{}
		}
		if r.Config == nil {
			r.Config = map[string]any{}
		}
		if r.Usage.Metrics == nil {
			r.Usage.Metrics = map[string]float64{}
		}
		if r.Findings == nil {
			r.Findings = []Finding{}
		}
	}

	sort.SliceStable(res, func(i, j int) bool {
		a, b := res[i], res[j]
		if ra, rb := verdictRank[a.Usage.Verdict], verdictRank[b.Usage.Verdict]; ra != rb {
			return ra < rb
		}
		ca, cb := a.Cost.MonthlyUSD, b.Cost.MonthlyUSD
		switch {
		case ca != nil && cb == nil:
			return true
		case ca == nil && cb != nil:
			return false
		case ca != nil && cb != nil && *ca != *cb:
			return *ca > *cb
		}
		if a.Type != b.Type {
			return a.Type < b.Type
		}
		return a.ID < b.ID
	})

	sum := Summary{
		Resources: len(res),
		ByVerdict: map[Verdict]int{
			VerdictInUse: 0, VerdictIdle: 0, VerdictOrphan: 0, VerdictUnknown: 0, VerdictNotEvaluated: 0,
		},
		FindingsBySeverity: map[Severity]int{SeverityHigh: 0, SeverityMedium: 0, SeverityLow: 0},
	}
	var total, waste float64
	for _, r := range res {
		sum.ByVerdict[r.Usage.Verdict]++
		for _, f := range r.Findings {
			sum.FindingsBySeverity[f.Severity]++
		}
		var c float64
		if r.Cost.MonthlyUSD != nil {
			c = *r.Cost.MonthlyUSD
		}
		total += c
		if r.Usage.Verdict == VerdictOrphan || r.Usage.Verdict == VerdictIdle {
			sum.WasteResources++
			waste += c
		}
	}
	sum.MonthlyUSD = round2(total)
	sum.WasteMonthlyUSD = round2(waste)

	if regions == nil {
		regions = []string{}
	}
	if warnings == nil {
		warnings = []Warning{}
	}
	return Report{
		SchemaVersion: "1",
		GeneratedAt:   now,
		Provider:      provider,
		AccountID:     accountID,
		WindowDays:    windowDays,
		Regions:       regions,
		Summary:       sum,
		Resources:     res,
		Warnings:      warnings,
	}
}
