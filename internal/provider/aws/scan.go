package aws

import (
	"context"
	"errors"
	"fmt"
	"io"
	"sort"
	"time"

	"lookr/internal/model"
	"lookr/internal/provider"
	"lookr/internal/provider/aws/checks"
	"lookr/internal/provider/aws/collect"
	"lookr/internal/provider/aws/cost"
	"lookr/internal/provider/aws/usage"
)

type Deps struct {
	AccountID  func(context.Context) (string, error)
	Regions    func(context.Context) ([]string, error)
	Collectors []collect.Collector
	Collect    func(context.Context, collect.Collector, collect.Scope) ([]model.Resource, error)
	Metrics    func(context.Context, string, []usage.Request, time.Time, time.Time) map[string]usage.Result
	Actual     func(context.Context, time.Time) (cost.Actual, int, error)
	Prices     cost.PriceLookup
}

// Run scans with injected dependencies. Only the coordinating goroutine writes
// progress and builds the report, including when collection is concurrent.
func Run(ctx context.Context, d Deps, o provider.ScanOptions) (model.Report, error) {
	if err := ctx.Err(); err != nil {
		return model.Report{}, err
	}
	if o.Progress == nil {
		o.Progress = io.Discard
	}
	if o.Now.IsZero() {
		o.Now = time.Now()
	}
	if o.Concurrency < 1 {
		o.Concurrency = 1
	}
	account, err := d.AccountID(ctx)
	if err != nil {
		return model.Report{}, fmt.Errorf("identify AWS account: %w", err)
	}
	regions := append([]string(nil), o.Regions...)
	if len(regions) == 0 {
		regions, err = d.Regions(ctx)
		if err != nil {
			return model.Report{}, fmt.Errorf("list AWS regions: %w", err)
		}
		regions = append([]string(nil), regions...)
	}
	sort.Strings(regions)
	regions = unique(regions)
	selected := map[string]bool{}
	for _, v := range o.Types {
		selected[v] = true
	}
	var regional, global []collect.Collector
	for _, c := range d.Collectors {
		if len(selected) > 0 && !selected[c.Short] {
			continue
		}
		if c.Global {
			global = append(global, c)
		} else {
			regional = append(regional, c)
		}
	}
	type task struct {
		region     string
		collectors []collect.Collector
	}
	var tasks []task
	if len(regional) > 0 {
		for _, region := range regions {
			tasks = append(tasks, task{region, regional})
		}
	}
	if len(global) > 0 {
		tasks = append(tasks, task{"global", global})
	}
	type result struct {
		resources []model.Resource
		warnings  []model.Warning
		region    string
	}
	results := make(chan result, len(tasks))
	jobs := make(chan task)
	workerCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	for range min(o.Concurrency, len(tasks)) {
		go func() {
			for job := range jobs {
				if workerCtx.Err() != nil {
					return
				}
				r := result{region: job.region}
				for _, c := range job.collectors {
					if workerCtx.Err() != nil {
						return
					}
					resources, e := d.Collect(workerCtx, c, collect.Scope{AccountID: account, Region: job.region})
					r.resources = append(r.resources, resources...)
					if e != nil {
						r.warnings = append(r.warnings, model.Warning{Region: job.region, Type: c.Type, Error: e.Error()})
					}
				}
				select {
				case results <- r:
				case <-workerCtx.Done():
					return
				}
			}
		}()
	}
	go func() {
		defer close(jobs)
		for _, job := range tasks {
			select {
			case jobs <- job:
			case <-workerCtx.Done():
				return
			}
		}
	}()
	var resources []model.Resource
	var warnings []model.Warning
	warn := func(w model.Warning) {
		warnings = append(warnings, w)
		fmt.Fprintf(o.Progress, "Warning [%s/%s]: %s\n", w.Region, w.Type, w.Error)
	}
	for range tasks {
		select {
		case <-ctx.Done():
			return model.Report{}, ctx.Err()
		case r := <-results:
			resources = append(resources, r.resources...)
			for _, w := range r.warnings {
				warn(w)
			}
			fmt.Fprintf(o.Progress, "Completed region %s\n", r.region)
		}
	}
	if err := ctx.Err(); err != nil {
		return model.Report{}, err
	}
	sort.SliceStable(resources, func(i, j int) bool { return resources[i].Key() < resources[j].Key() })
	requests := map[string][]usage.Request{}
	for _, r := range resources {
		byRegion := map[string][]usage.Query{}
		for _, q := range usage.QueriesFor(r) {
			region := q.Region
			if region == "" {
				region = r.Region
			}
			if region == "" || region == "global" {
				region = "us-east-1"
			}
			byRegion[region] = append(byRegion[region], q)
		}
		for region, queries := range byRegion {
			requests[region] = append(requests[region], usage.Request{ResourceKey: r.Key(), Queries: queries})
		}
	}
	metricRegions := make([]string, 0, len(requests))
	for region := range requests {
		metricRegions = append(metricRegions, region)
	}
	sort.Strings(metricRegions)
	metrics := map[string]usage.Result{}
	for _, region := range metricRegions {
		if err := ctx.Err(); err != nil {
			return model.Report{}, err
		}
		var fetched map[string]usage.Result
		if d.Metrics != nil {
			fetched = d.Metrics(ctx, region, requests[region], o.Now.AddDate(0, 0, -o.Days), o.Now)
		}
		for _, req := range requests[region] {
			v, ok := fetched[req.ResourceKey]
			if !ok {
				v.Err = errors.New("metrics response unavailable")
			}
			merged := metrics[req.ResourceKey]
			if merged.Metrics == nil {
				merged.Metrics = usage.Metrics{}
			}
			for k, n := range v.Metrics {
				merged.Metrics[k] = n
			}
			merged.Err = errors.Join(merged.Err, v.Err)
			metrics[req.ResourceKey] = merged
		}
	}
	if err := ctx.Err(); err != nil {
		return model.Report{}, err
	}
	var actual cost.Actual
	if o.CostMode == "auto" {
		var calls int
		var e error
		if d.Actual == nil {
			e = errors.New("Cost Explorer unavailable")
		} else {
			actual, calls, e = d.Actual(ctx, o.Now)
		}
		if err := ctx.Err(); err != nil {
			return model.Report{}, err
		}
		fmt.Fprintf(o.Progress, "Cost Explorer: %d API calls\n", calls)
		if e != nil {
			actual = nil
			warn(model.Warning{Region: "global", Type: "cost", Error: e.Error()})
		}
	}
	for i := range resources {
		if err := ctx.Err(); err != nil {
			return model.Report{}, err
		}
		r := &resources[i]
		usage.Apply(r, metrics[r.Key()], o.Now, o.Days)
		if o.CostMode == "none" {
			r.Cost = model.Cost{Source: model.CostNone, Note: "cost not requested"}
		} else if value, ok := actual.Lookup(*r); ok && r.Type == "aws.ec2.instance" {
			cost.ApplyActual(r, value)
		} else {
			cost.Estimate(ctx, r, r.Usage.Metrics, o.Days, d.Prices)
		}
		r.Findings = checks.Run(*r, o.Now)
	}
	if err := ctx.Err(); err != nil {
		return model.Report{}, err
	}
	sort.Slice(warnings, func(i, j int) bool {
		a, b := warnings[i], warnings[j]
		if a.Region != b.Region {
			return a.Region < b.Region
		}
		if a.Type != b.Type {
			return a.Type < b.Type
		}
		return a.Error < b.Error
	})
	return model.NewReport("aws", account, o.Days, regions, resources, warnings, o.Now), nil
}
func unique(values []string) []string {
	result := values[:0]
	for _, v := range values {
		if len(result) == 0 || result[len(result)-1] != v {
			result = append(result, v)
		}
	}
	return result
}
