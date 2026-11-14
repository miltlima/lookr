package model

import (
	"math"
	"reflect"
	"testing"
	"time"
)

var fixedNow = time.Date(2026, 11, 14, 12, 0, 0, 0, time.UTC)

func ptr(f float64) *float64 { return &f }

func TestNewReportSummary(t *testing.T) {
	res := []Resource{
		{Type: "aws.ec2.instance", ID: "i-1", Usage: Usage{Verdict: VerdictInUse}, Cost: Cost{MonthlyUSD: ptr(100), Source: CostActual}},
		{Type: "aws.ebs.volume", ID: "vol-1", Usage: Usage{Verdict: VerdictOrphan}, Cost: Cost{MonthlyUSD: ptr(80), Source: CostEstimate},
			Findings: []Finding{{Check: "ebs.unencrypted", Severity: SeverityMedium}}},
		{Type: "aws.elb.loadbalancer", ID: "lb-1", Usage: Usage{Verdict: VerdictIdle}, Cost: Cost{MonthlyUSD: ptr(18.4), Source: CostEstimate}},
		{Type: "aws.iam.role", ID: "r-1", Usage: Usage{Verdict: VerdictNotEvaluated}, Cost: Cost{Source: CostNone}},
	}
	r := NewReport("aws", "123456789012", 30, []string{"us-east-1"}, res, nil, fixedNow)

	if r.SchemaVersion != "1" {
		t.Errorf("SchemaVersion = %q", r.SchemaVersion)
	}
	if !r.GeneratedAt.Equal(fixedNow) || r.Provider != "aws" || r.AccountID != "123456789012" || r.WindowDays != 30 {
		t.Errorf("header mismatch: %+v", r)
	}
	s := r.Summary
	if s.Resources != 4 {
		t.Errorf("Resources = %d", s.Resources)
	}
	if math.Abs(s.MonthlyUSD-198.4) > 1e-9 {
		t.Errorf("MonthlyUSD = %v", s.MonthlyUSD)
	}
	if s.WasteResources != 2 {
		t.Errorf("WasteResources = %d", s.WasteResources)
	}
	if math.Abs(s.WasteMonthlyUSD-98.4) > 1e-9 {
		t.Errorf("WasteMonthlyUSD = %v", s.WasteMonthlyUSD)
	}
	if len(s.ByVerdict) != 5 {
		t.Errorf("ByVerdict = %v", s.ByVerdict)
	}
	for _, v := range []Verdict{VerdictInUse, VerdictIdle, VerdictOrphan, VerdictUnknown, VerdictNotEvaluated} {
		if _, ok := s.ByVerdict[v]; !ok {
			t.Errorf("missing verdict key %q", v)
		}
	}
	if s.ByVerdict[VerdictUnknown] != 0 || s.ByVerdict[VerdictOrphan] != 1 {
		t.Errorf("ByVerdict = %v", s.ByVerdict)
	}
	if len(s.FindingsBySeverity) != 3 || s.FindingsBySeverity[SeverityMedium] != 1 {
		t.Errorf("FindingsBySeverity = %v", s.FindingsBySeverity)
	}
	if r.Warnings == nil || len(r.Warnings) != 0 {
		t.Errorf("Warnings = %#v", r.Warnings)
	}
	for _, x := range r.Resources {
		if x.Findings == nil || x.Tags == nil || x.Config == nil || x.Usage.Metrics == nil {
			t.Errorf("nil collection in %s", x.ID)
		}
	}
	// input untouched
	if res[0].Tags != nil || res[0].Config != nil || res[0].Usage.Metrics != nil || res[0].Findings != nil {
		t.Error("input slice was mutated")
	}
	if res[0].ID != "i-1" || res[3].ID != "r-1" {
		t.Error("input order changed")
	}
}

func TestNewReportOrder(t *testing.T) {
	mk := func(typ, id string, v Verdict, c *float64) Resource {
		return Resource{Type: typ, ID: id, Usage: Usage{Verdict: v}, Cost: Cost{MonthlyUSD: c}}
	}
	res := []Resource{
		mk("b", "1", VerdictNotEvaluated, nil),
		mk("b", "2", VerdictInUse, ptr(5)),
		mk("b", "3", VerdictUnknown, nil),
		mk("b", "4", VerdictIdle, nil),
		mk("b", "5", VerdictIdle, ptr(10)),
		mk("b", "6", VerdictIdle, ptr(20)),
		mk("b", "7", VerdictOrphan, ptr(1)),
		mk("b", "9", VerdictIdle, ptr(10)),
		mk("a", "8", VerdictIdle, ptr(10)),
		mk("b", "0", VerdictIdle, nil),
	}
	r := NewReport("aws", "1", 30, nil, res, nil, fixedNow)
	var got []string
	for _, x := range r.Resources {
		got = append(got, x.ID)
	}
	want := []string{"7", "6", "8", "5", "9", "0", "4", "3", "2", "1"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("order = %v, want %v", got, want)
	}
	if r.Regions == nil {
		t.Error("Regions nil")
	}
}
