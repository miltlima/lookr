package usage

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/cloudwatch"
	"github.com/aws/aws-sdk-go-v2/service/cloudwatch/types"
)

type cloudWatchAPI interface {
	GetMetricData(context.Context, *cloudwatch.GetMetricDataInput, ...func(*cloudwatch.Options)) (*cloudwatch.GetMetricDataOutput, error)
}

func NewCloudWatch(cfg aws.Config) cloudWatchAPI { return cloudwatch.NewFromConfig(cfg) }

type Request struct {
	ResourceKey string
	Queries     []Query
}

type metricQuery struct {
	resource string
	query    Query
	id       string
}

// Fetch retrieves daily metrics in CloudWatch's maximum batches of 500 queries.
// Any retrieval failure remains attached to its resource, even across batches.
func Fetch(ctx context.Context, api cloudWatchAPI, reqs []Request, start, end time.Time) map[string]Result {
	results := make(map[string]Result, len(reqs))
	var queries []metricQuery
	for _, req := range reqs {
		if _, ok := results[req.ResourceKey]; !ok {
			results[req.ResourceKey] = Result{Metrics: Metrics{}}
		}
		for _, q := range req.Queries {
			queries = append(queries, metricQuery{req.ResourceKey, q, fmt.Sprintf("q%d", len(queries))})
		}
	}
	fail := func(resource string, err error) {
		r := results[resource]
		r.Err = errors.Join(r.Err, err)
		results[resource] = r
	}
	for offset := 0; offset < len(queries); offset += 500 {
		batch := queries[offset:min(offset+500, len(queries))]
		input := &cloudwatch.GetMetricDataInput{StartTime: &start, EndTime: &end}
		byID := make(map[string]metricQuery, len(batch))
		points := make(map[string][]float64, len(batch))
		seen := make(map[string]bool, len(batch))
		incomplete := make(map[string]bool, len(batch))
		invalid := make(map[string]bool, len(batch))
		for _, q := range batch {
			byID[q.id] = q
			names := make([]string, 0, len(q.query.Dimensions))
			for name := range q.query.Dimensions {
				names = append(names, name)
			}
			sort.Strings(names)
			dimensions := make([]types.Dimension, 0, len(names))
			for _, name := range names {
				dimensions = append(dimensions, types.Dimension{Name: aws.String(name), Value: aws.String(q.query.Dimensions[name])})
			}
			input.MetricDataQueries = append(input.MetricDataQueries, types.MetricDataQuery{Id: aws.String(q.id), ReturnData: aws.Bool(true), MetricStat: &types.MetricStat{Period: aws.Int32(86400), Stat: aws.String(q.query.Stat), Metric: &types.Metric{Namespace: aws.String(q.query.Namespace), MetricName: aws.String(q.query.Metric), Dimensions: dimensions}}})
		}
		failed := false
		tokens := map[string]bool{}
		for {
			out, err := api.GetMetricData(ctx, input)
			if err == nil && out == nil {
				err = errors.New("CloudWatch returned no response")
			}
			if err != nil {
				for _, q := range batch {
					fail(q.resource, err)
				}
				failed = true
				break
			}
			for _, data := range out.MetricDataResults {
				id := aws.ToString(data.Id)
				q, ok := byID[id]
				if !ok {
					continue
				}
				seen[id] = true
				if data.StatusCode != types.StatusCodeComplete && data.StatusCode != types.StatusCodePartialData {
					invalid[id] = true
					fail(q.resource, fmt.Errorf("CloudWatch query %s: status %s", q.query.Key, data.StatusCode))
					continue
				}
				incomplete[id] = data.StatusCode == types.StatusCodePartialData
				points[id] = append(points[id], data.Values...)
			}
			token := aws.ToString(out.NextToken)
			if token == "" {
				break
			}
			if tokens[token] {
				for _, q := range batch {
					fail(q.resource, errors.New("CloudWatch repeated pagination token"))
				}
				failed = true
				break
			}
			tokens[token] = true
			// A fresh input preserves the request recorded by callers and test fakes.
			next := *input
			next.NextToken = aws.String(token)
			input = &next
		}
		if failed {
			continue
		}
		for _, q := range batch {
			if !seen[q.id] {
				fail(q.resource, fmt.Errorf("CloudWatch omitted query %s", q.query.Key))
				continue
			}
			if incomplete[q.id] {
				fail(q.resource, fmt.Errorf("CloudWatch query %s: incomplete data", q.query.Key))
				continue
			}
			if invalid[q.id] {
				continue
			}
			values := points[q.id]
			if len(values) == 0 && !q.query.ZeroIfMissing {
				continue
			}
			var value float64
			for i, v := range values {
				switch q.query.Stat {
				case "Maximum":
					if i == 0 || v > value {
						value = v
					}
				default:
					value += v
				}
			}
			if q.query.Stat == "Average" && len(values) > 0 {
				value /= float64(len(values))
			}
			results[q.resource].Metrics[q.query.Key] = value
		}
	}
	return results
}
