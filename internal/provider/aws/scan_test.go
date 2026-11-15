package aws

import (
	"bytes"
	"context"
	"errors"
	"io"
	"reflect"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	sdk "github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/ec2"
	ec2types "github.com/aws/aws-sdk-go-v2/service/ec2/types"
	"github.com/aws/aws-sdk-go-v2/service/sts"

	"lookr/internal/model"
	"lookr/internal/provider"
	"lookr/internal/provider/aws/collect"
	"lookr/internal/provider/aws/cost"
	"lookr/internal/provider/aws/usage"
)

func options() provider.ScanOptions {
	return provider.ScanOptions{Days: 30, CostMode: "estimate", Concurrency: 2, Now: time.Date(2026, 11, 15, 12, 0, 0, 0, time.UTC), Progress: io.Discard}
}
func dependencies() Deps {
	return Deps{AccountID: func(context.Context) (string, error) { return "123", nil }, Regions: func(context.Context) ([]string, error) { return []string{"west", "east"}, nil }, Collectors: []collect.Collector{{Type: "aws.ec2.eip", Short: "eip"}, {Type: "aws.route53.hostedzone", Short: "route53", Global: true}}, Collect: func(_ context.Context, c collect.Collector, s collect.Scope) ([]model.Resource, error) {
		return []model.Resource{{Type: c.Type, Short: c.Short, ID: "same", Region: s.Region, Config: map[string]any{}}}, nil
	}}
}
func TestRunHappyPath(t *testing.T) {
	r, e := Run(context.Background(), dependencies(), options())
	if e != nil || len(r.Resources) != 3 || !reflect.DeepEqual(r.Regions, []string{"east", "west"}) {
		t.Fatalf("report=%+v error=%v", r, e)
	}
	for _, v := range r.Resources {
		if v.Usage.Verdict == "" || v.Cost.Source == "" || v.Findings == nil {
			t.Fatalf("incomplete resource %+v", v)
		}
	}
}
func TestRunCollectorFailureBecomesWarning(t *testing.T) {
	d := dependencies()
	original := d.Collect
	d.Collect = func(ctx context.Context, c collect.Collector, s collect.Scope) ([]model.Resource, error) {
		r, e := original(ctx, c, s)
		if s.Region == "west" {
			e = errors.New("denied")
		}
		return r, e
	}
	var b bytes.Buffer
	o := options()
	o.Progress = &b
	r, e := Run(context.Background(), d, o)
	if e != nil || len(r.Resources) != 3 || len(r.Warnings) != 1 || r.Warnings[0].Region != "west" || !strings.Contains(b.String(), "denied") {
		t.Fatalf("%+v %v %s", r, e, b.String())
	}
}
func TestRunAccountError(t *testing.T) {
	d := dependencies()
	d.AccountID = func(context.Context) (string, error) { return "", errors.New("account") }
	d.Collect = func(context.Context, collect.Collector, collect.Scope) ([]model.Resource, error) {
		t.Fatal("collect called")
		return nil, nil
	}
	if _, e := Run(context.Background(), d, options()); e == nil {
		t.Fatal("missing error")
	}
}
func TestRunRegionsOverrideAndTypeFilter(t *testing.T) {
	d := dependencies()
	d.Regions = func(context.Context) ([]string, error) { t.Fatal("regions called"); return nil, nil }
	o := options()
	o.Regions = []string{"selected"}
	o.Types = []string{"eip"}
	r, e := Run(context.Background(), d, o)
	if e != nil || len(r.Resources) != 1 || r.Resources[0].Region != "selected" {
		t.Fatalf("%+v %v", r, e)
	}
}
func TestRunMetricsRegion(t *testing.T) {
	d := dependencies()
	d.Collectors = []collect.Collector{{Type: "aws.s3.bucket", Short: "s3", Global: true}, {Type: "aws.lambda.function", Short: "lambda"}}
	d.Collect = func(_ context.Context, c collect.Collector, s collect.Scope) ([]model.Resource, error) {
		return []model.Resource{{Type: c.Type, Region: s.Region, ID: "x", Config: map[string]any{"bucket_region": "eu-west-3"}}}, nil
	}
	o := options()
	seen := map[string]bool{}
	d.Metrics = func(_ context.Context, region string, req []usage.Request, start, end time.Time) map[string]usage.Result {
		seen[region] = true
		if start != o.Now.AddDate(0, 0, -30) || end != o.Now {
			t.Error("window")
		}
		result := map[string]usage.Result{}
		for _, v := range req {
			result[v.ResourceKey] = usage.Result{Err: errors.New("metrics")}
		}
		return result
	}
	r, e := Run(context.Background(), d, o)
	if e != nil || !seen["eu-west-3"] || !seen["east"] || !seen["west"] || len(r.Warnings) != 0 {
		t.Fatalf("%+v %v %v", r, e, seen)
	}
	for _, v := range r.Resources {
		if v.Usage.Verdict != model.VerdictUnknown {
			t.Fatal(v.Usage)
		}
	}
}
func TestRunCostModes(t *testing.T) {
	for _, mode := range []string{"auto", "estimate", "none", "fallback"} {
		t.Run(mode, func(t *testing.T) {
			d := dependencies()
			d.Collectors = []collect.Collector{{Type: "aws.ec2.instance", Short: "ec2"}, {Type: "aws.lambda.function", Short: "lambda"}}
			var priceCalls int
			d.Prices = priceFunc(func(context.Context, cost.PriceQuery) (float64, error) { priceCalls++; return 1, nil })
			var calls int
			d.Actual = func(context.Context, time.Time) (cost.Actual, int, error) {
				calls++
				if mode == "fallback" {
					return nil, 2, errors.New("unavailable")
				}
				return cost.Actual{"same": 9}, 2, nil
			}
			o := options()
			o.CostMode = mode
			if mode == "fallback" {
				o.CostMode = "auto"
			}
			var b bytes.Buffer
			o.Progress = &b
			r, e := Run(context.Background(), d, o)
			if e != nil {
				t.Fatal(e)
			}
			for _, v := range r.Resources {
				if mode == "none" && (v.Cost.Source != model.CostNone || v.Cost.Note != "cost not requested") {
					t.Fatal(v.Cost)
				}
				if mode == "auto" && ((v.Type == "aws.ec2.instance") != (v.Cost.Source == model.CostActual)) {
					t.Fatal(v)
				}
			}
			if mode == "none" && priceCalls != 0 {
				t.Fatalf("none called Prices %d times", priceCalls)
			}
			if mode == "fallback" || mode == "estimate" {
				for _, v := range r.Resources {
					if v.Type == "aws.ec2.instance" && v.Cost.Source != model.CostEstimate {
						t.Fatal(v.Cost)
					}
				}
			}
			if (mode == "auto" || mode == "fallback") != (calls == 1) {
				t.Fatal(calls)
			}
			if mode == "fallback" && (len(r.Warnings) != 1 || r.Warnings[0].Type != "cost" || r.Warnings[0].Region != "global") {
				t.Fatal(r.Warnings)
			}
			if calls > 0 && !strings.Contains(b.String(), "2") {
				t.Fatal(b.String())
			}
		})
	}
}
func TestRunConcurrencyBound(t *testing.T) {
	d := dependencies()
	d.Collectors = d.Collectors[:1]
	var active, max atomic.Int32
	d.Collect = func(ctx context.Context, c collect.Collector, s collect.Scope) ([]model.Resource, error) {
		n := active.Add(1)
		for old := max.Load(); n > old && !max.CompareAndSwap(old, n); old = max.Load() {
		}
		time.Sleep(5 * time.Millisecond)
		active.Add(-1)
		return nil, nil
	}
	o := options()
	o.Regions = []string{"a", "b", "c", "d", "e", "f"}
	if _, e := Run(context.Background(), d, o); e != nil || max.Load() > 2 {
		t.Fatalf("max=%d error=%v", max.Load(), e)
	}
}
func TestRunCancel(t *testing.T) {
	d := dependencies()
	started := make(chan struct{}, 1)
	d.Collect = func(ctx context.Context, c collect.Collector, s collect.Scope) ([]model.Resource, error) {
		select {
		case started <- struct{}{}:
		default:
		}
		<-ctx.Done()
		return nil, ctx.Err()
	}
	ctx, cancel := context.WithCancel(context.Background())
	go func() { <-started; cancel() }()
	start := time.Now()
	if _, e := Run(ctx, d, options()); !errors.Is(e, context.Canceled) || time.Since(start) > time.Second {
		t.Fatalf("error=%v", e)
	}
}
func TestRunDeterministic(t *testing.T) {
	d := dependencies()
	a, e := Run(context.Background(), d, options())
	if e != nil {
		t.Fatal(e)
	}
	for range 10 {
		b, e := Run(context.Background(), d, options())
		if e != nil || !reflect.DeepEqual(a, b) {
			t.Fatalf("reports differ: %+v %+v %v", a, b, e)
		}
	}
}

func TestProviderIdentity(t *testing.T) {
	if New().Name() != "aws" {
		t.Fatal("provider name")
	}
}

type fakeSTS struct {
	out *sts.GetCallerIdentityOutput
	err error
}

func (f fakeSTS) GetCallerIdentity(context.Context, *sts.GetCallerIdentityInput, ...func(*sts.Options)) (*sts.GetCallerIdentityOutput, error) {
	return f.out, f.err
}

type fakeRegions struct {
	out *ec2.DescribeRegionsOutput
	err error
}

func (f fakeRegions) DescribeRegions(context.Context, *ec2.DescribeRegionsInput, ...func(*ec2.Options)) (*ec2.DescribeRegionsOutput, error) {
	return f.out, f.err
}
func TestProviderAccount(t *testing.T) {
	id, e := readAccount(context.Background(), fakeSTS{out: &sts.GetCallerIdentityOutput{Account: sdk.String("123")}})
	if id != "123" || e != nil {
		t.Fatalf("%s %v", id, e)
	}
	for _, f := range []fakeSTS{{}, {err: errors.New("denied")}, {out: &sts.GetCallerIdentityOutput{}}} {
		if _, e := readAccount(context.Background(), f); e == nil {
			t.Fatal("missing account error")
		}
	}
}
func TestProviderRegions(t *testing.T) {
	r, e := readRegions(context.Background(), fakeRegions{out: &ec2.DescribeRegionsOutput{Regions: []ec2types.Region{{RegionName: sdk.String("east")}, {}}}})
	if e != nil || !reflect.DeepEqual(r, []string{"east"}) {
		t.Fatalf("%v %v", r, e)
	}
	for _, f := range []fakeRegions{{}, {err: errors.New("denied")}} {
		if _, e := readRegions(context.Background(), f); e == nil {
			t.Fatal("missing regions error")
		}
	}
}

type priceFunc func(context.Context, cost.PriceQuery) (float64, error)

func (f priceFunc) Price(ctx context.Context, q cost.PriceQuery) (float64, error) { return f(ctx, q) }
func TestRunGlobalMetricFallback(t *testing.T) {
	d := dependencies()
	d.Collectors = []collect.Collector{{Type: "aws.s3.bucket", Short: "s3", Global: true}}
	d.Metrics = func(_ context.Context, region string, req []usage.Request, _, _ time.Time) map[string]usage.Result {
		if region != "us-east-1" || len(req) != 1 {
			t.Fatalf("region %s requests %v", region, req)
		}
		return map[string]usage.Result{req[0].ResourceKey: {Metrics: usage.Metrics{"object_count": 0}}}
	}
	r, e := Run(context.Background(), d, options())
	if e != nil || len(r.Resources) != 1 || r.Resources[0].Usage.Verdict != model.VerdictIdle {
		t.Fatalf("%+v %v", r, e)
	}
}
func TestRunRegionsError(t *testing.T) {
	d := dependencies()
	d.Regions = func(context.Context) ([]string, error) { return nil, errors.New("regions denied") }
	if _, e := Run(context.Background(), d, options()); e == nil {
		t.Fatal("missing regions error")
	}
}
