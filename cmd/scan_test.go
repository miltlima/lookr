package cmd

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"lookr/internal/model"
	"lookr/internal/provider"
	"lookr/internal/provider/aws/collect"
)

type fakeProvider struct {
	options      provider.ScanOptions
	calls        int
	err          error
	warnings     []model.Warning
	liveWarnings bool
}

func (*fakeProvider) Name() string { return "aws" }
func (p *fakeProvider) Scan(_ context.Context, o provider.ScanOptions) (model.Report, error) {
	p.calls++
	p.options = o
	if p.liveWarnings {
		for _, w := range p.warnings {
			_, _ = o.Progress.Write([]byte("Warning [" + w.Region + "/" + w.Type + "]: " + w.Error + "\n"))
		}
	}
	return model.NewReport("aws", "123", o.Days, []string{"us-east-1"}, nil, p.warnings, time.Date(2026, 11, 15, 0, 0, 0, 0, time.UTC)), p.err
}
func runScan(t *testing.T, p *fakeProvider, args ...string) (string, string, error) {
	t.Helper()
	old := newProvider
	newProvider = func() provider.Provider { return p }
	t.Cleanup(func() { newProvider = old })
	var out, stderr bytes.Buffer
	c := newRootCommand()
	c.SetOut(&out)
	c.SetErr(&stderr)
	c.SetArgs(append([]string{"scan"}, args...))
	err := c.ExecuteContext(context.Background())
	return out.String(), stderr.String(), err
}
func TestScanDefaults(t *testing.T) {
	p := &fakeProvider{}
	out, _, err := runScan(t, p)
	if err != nil || p.calls != 1 || p.options.Days != 30 || p.options.Concurrency != 8 || p.options.CostMode != "auto" || len(p.options.Regions) != 0 || len(p.options.Types) != 0 || !strings.Contains(out, "Account ") {
		t.Fatalf("%s %+v %v", out, p, err)
	}
}
func TestScanParsesLists(t *testing.T) {
	p := &fakeProvider{}
	_, _, err := runScan(t, p, "--regions", " us-east-1, sa-east-1 ", "--type", " ec2, ebs ", "--profile", "test")
	if err != nil || !reflect.DeepEqual(p.options.Regions, []string{"us-east-1", "sa-east-1"}) || !reflect.DeepEqual(p.options.Types, []string{"ec2", "ebs"}) || p.options.Profile != "test" {
		t.Fatal(p.options, err)
	}
}
func TestScanOutputFormats(t *testing.T) {
	for _, format := range []string{"json", "csv"} {
		t.Run(format, func(t *testing.T) {
			out, stderr, err := runScan(t, &fakeProvider{}, "--output", format)
			if err != nil || stderr != "" {
				t.Fatal(err, stderr)
			}
			if format == "json" {
				var r model.Report
				if err := json.Unmarshal([]byte(out), &r); err != nil || r.SchemaVersion != "1" {
					t.Fatal(out, err)
				}
			} else if !strings.HasPrefix(out, "provider,account_id,") {
				t.Fatal(out)
			}
		})
	}
}
func TestScanOutFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "report.json")
	p := &fakeProvider{}
	out, _, err := runScan(t, p, "--output", "json", "--out-file", path)
	data, readErr := os.ReadFile(path)
	if err != nil || readErr != nil || out != "" || !json.Valid(data) {
		t.Fatal(out, err, readErr, string(data))
	}
}
func TestScanOutFileError(t *testing.T) {
	p := &fakeProvider{}
	_, _, err := runScan(t, p, "--out-file", filepath.Join(t.TempDir(), "missing", "r.json"))
	if err == nil || p.calls != 0 {
		t.Fatal(err, p.calls)
	}
}
func TestScanInvalidArgs(t *testing.T) {
	for _, args := range [][]string{{"--days", "0"}, {"--days", "-3"}, {"--output", "xml"}, {"--cost", "real"}, {"--concurrency", "0"}, {"--type", "foo"}, {"unexpected"}} {
		t.Run(strings.Join(args, " "), func(t *testing.T) {
			p := &fakeProvider{}
			out, _, err := runScan(t, p, args...)
			if err == nil || p.calls != 0 || out != "" {
				t.Fatal(out, err, p.calls)
			}
			if len(args) > 1 && !strings.Contains(err.Error(), args[0]) {
				t.Fatal(err)
			}
			if args[0] == "--type" {
				for _, s := range collect.Shorts() {
					if !strings.Contains(err.Error(), s) {
						t.Fatal(err)
					}
				}
			}
		})
	}
}
func TestScanProviderError(t *testing.T) {
	want := errors.New("invalid credentials")
	out, _, err := runScan(t, &fakeProvider{err: want})
	if !errors.Is(err, want) || out != "" {
		t.Fatal(out, err)
	}
}
func TestScanWarningsToStderr(t *testing.T) {
	for _, live := range []bool{false, true} {
		p := &fakeProvider{warnings: []model.Warning{{Region: "global", Type: "cost", Error: "denied"}}, liveWarnings: live}
		out, stderr, err := runScan(t, p, "--output", "json")
		if err != nil || !json.Valid([]byte(out)) || strings.Count(stderr, "Warning [global/cost]: denied") != 1 {
			t.Fatal(out, stderr, err)
		}
	}
}
