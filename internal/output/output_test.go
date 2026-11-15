package output

import (
	"bytes"
	"encoding/csv"
	"errors"
	"flag"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"lookr/internal/model"
)

var update = flag.Bool("update", false, "rewrite golden files")

func f64(v float64) *float64 { return &v }

func sampleReport() model.Report {
	created := time.Date(2025, 3, 9, 14, 30, 0, 0, time.UTC)
	resources := []model.Resource{
		{
			Provider: "aws", AccountID: "123456789012", Region: "us-east-1",
			Type: "aws.ebs.volume", Short: "ebs", ID: "vol-0a1b2c3d",
			ARN:       "arn:aws:ec2:us-east-1:123456789012:volume/vol-0a1b2c3d",
			Tags:      map[string]string{"env": "dev"},
			CreatedAt: &created,
			Config:    map[string]any{"size_gb": 800, "volume_type": "gp3", "state": "available"},
			Usage:     model.Usage{Verdict: model.VerdictOrphan, Reason: "not attached to any instance", WindowDays: 30},
			Cost:      model.Cost{MonthlyUSD: f64(80), Source: model.CostEstimate, Note: "storage only"},
			Findings: []model.Finding{{
				Check: "ebs.unencrypted", Severity: model.SeverityMedium,
				Message: "volume is not encrypted", Recommendation: "Snapshot and recreate the volume with encryption enabled",
			}},
		},
		{
			Provider: "aws", AccountID: "123456789012", Region: "us-east-1",
			Type: "aws.ec2.instance", Short: "ec2", ID: "i-04f2", Name: "api-2",
			Config: map[string]any{"instance_type": "t3.large", "state": "running", "security_groups": []string{"sg-1"}},
			Usage: model.Usage{Verdict: model.VerdictIdle, Reason: "avg CPU 0.8% over 30d", WindowDays: 30,
				Metrics: map[string]float64{"cpu_avg": 0.8}},
			Cost: model.Cost{MonthlyUSD: f64(61.32), Source: model.CostActual},
		},
		{
			Provider: "aws", AccountID: "123456789012", Region: "us-east-1",
			Type: "aws.rds.instance", Short: "rds", ID: "db-homolog", Name: "db-homolog",
			Config: map[string]any{"engine": "postgres", "publicly_accessible": true},
			Usage:  model.Usage{Verdict: model.VerdictInUse, Reason: "max 14 connections", WindowDays: 30},
			Cost:   model.Cost{MonthlyUSD: f64(198.40), Source: model.CostEstimate},
			Findings: []model.Finding{{
				Check: "rds.public", Severity: model.SeverityHigh,
				Message:        "database is publicly accessible",
				Recommendation: "Disable public accessibility or restrict the security group to known addresses",
			}},
		},
		{
			Provider: "aws", AccountID: "123456789012", Region: "global",
			Type: "aws.iam.role", Short: "iam", ID: "deploy-role", Name: "deploy-role",
			Usage: model.Usage{Verdict: model.VerdictNotEvaluated, Reason: "inventory only"},
			Cost:  model.Cost{Source: model.CostNone},
		},
		{
			Provider: "aws", AccountID: "123456789012", Region: "us-east-1",
			Type: "aws.sqs.queue", Short: "sqs", ID: "q-1", Name: `a,"b"`,
			Tags:   map[string]string{"note": "line1\nline2"},
			Config: map[string]any{"fifo": false},
			Usage:  model.Usage{Verdict: model.VerdictUnknown, Reason: "no metrics", WindowDays: 30},
			Cost:   model.Cost{MonthlyUSD: f64(0), Source: model.CostEstimate},
		},
	}
	warnings := []model.Warning{{Region: "eu-west-1", Type: "aws.eks.cluster", Error: "AccessDenied"}}
	now := time.Date(2026, 11, 14, 12, 0, 0, 0, time.UTC)
	return model.NewReport("aws", "123456789012", 30, []string{"us-east-1"}, resources, warnings, now)
}

func golden(t *testing.T, name string, write func(*bytes.Buffer) error) {
	t.Helper()
	var buf bytes.Buffer
	if err := write(&buf); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join("testdata", name)
	if *update {
		if err := os.WriteFile(path, buf.Bytes(), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	want, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if buf.String() != string(want) {
		t.Errorf("%s differs from golden file.\n--- got ---\n%s\n--- want ---\n%s", name, buf.String(), want)
	}
}

func TestWriteJSONGolden(t *testing.T) {
	golden(t, "report.json", func(b *bytes.Buffer) error { return WriteJSON(b, sampleReport()) })
}

func TestWriteCSVGolden(t *testing.T) {
	golden(t, "report.csv", func(b *bytes.Buffer) error { return WriteCSV(b, sampleReport()) })
}

func TestWriteTableGolden(t *testing.T) {
	golden(t, "report.txt", func(b *bytes.Buffer) error { return WriteTable(b, sampleReport()) })
}

func TestWriteCSVRoundTrip(t *testing.T) {
	r := sampleReport()
	var buf bytes.Buffer
	if err := WriteCSV(&buf, r); err != nil {
		t.Fatal(err)
	}
	records, err := csv.NewReader(&buf).ReadAll()
	if err != nil {
		t.Fatal(err)
	}
	if len(records) != len(r.Resources)+1 {
		t.Fatalf("records = %d, want %d", len(records), len(r.Resources)+1)
	}
	for i, rec := range records {
		if len(rec) != 17 {
			t.Errorf("record %d has %d fields, want 17", i, len(rec))
		}
	}
	found := false
	for _, rec := range records[1:] {
		if rec[6] == `a,"b"` {
			found = true
		}
	}
	if !found {
		t.Error(`name a,"b" did not round-trip`)
	}
}

func TestWriteTableEmpty(t *testing.T) {
	r := model.NewReport("aws", "123456789012", 30, nil, nil, nil, time.Now())
	var buf bytes.Buffer
	if err := WriteTable(&buf, r); err != nil {
		t.Fatal(err)
	}
	out := buf.String()
	if !strings.Contains(out, "Account 123456789012") || !strings.Contains(out, "No resources found.") {
		t.Errorf("unexpected output:\n%s", out)
	}
	if !strings.Contains(out, "(0%)") {
		t.Errorf("expected 0%% waste share:\n%s", out)
	}
}

// failAfter accepts n bytes in total and then fails every write.
type failAfter struct{ n int }

func (f *failAfter) Write(p []byte) (int, error) {
	if len(p) > f.n {
		n := f.n
		f.n = 0
		return n, errors.New("write failed")
	}
	f.n -= len(p)
	return len(p), nil
}

func TestWriteTablePropagatesWriteErrors(t *testing.T) {
	r := sampleReport()
	var full bytes.Buffer
	if err := WriteTable(&full, r); err != nil {
		t.Fatal(err)
	}
	// Every cut point, from inside the header to inside the FINDINGS block,
	// must surface the error.
	for n := 0; n < full.Len(); n++ {
		if err := WriteTable(&failAfter{n: n}, r); err == nil {
			t.Fatalf("no error when writer fails after %d of %d bytes", n, full.Len())
		}
	}
}
