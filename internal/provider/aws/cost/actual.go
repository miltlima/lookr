package cost

import (
	"context"
	"fmt"
	"math"
	"strconv"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/costexplorer"
	"github.com/aws/aws-sdk-go-v2/service/costexplorer/types"
	"lookr/internal/model"
)

const ActualWindowDays = 14

type Actual map[string]float64

type costExplorerAPI interface {
	GetCostAndUsageWithResources(context.Context, *costexplorer.GetCostAndUsageWithResourcesInput, ...func(*costexplorer.Options)) (*costexplorer.GetCostAndUsageWithResourcesOutput, error)
}

func NewCostExplorer(cfg aws.Config) costExplorerAPI {
	cfg.Region = "us-east-1"
	return costexplorer.NewFromConfig(cfg)
}

// FetchActual uses the required EC2 Compute service filter and RESOURCE_ID
// grouping. This opt-in endpoint is not queried for other services: they use
// estimates. Its window is restricted to the last 14 days, ending today UTC.
// Verified 2026-11-15:
// https://docs.aws.amazon.com/aws-cost-management/latest/APIReference/API_GetCostAndUsageWithResources.html
func FetchActual(ctx context.Context, api costExplorerAPI, now time.Time) (Actual, int, error) {
	end := now.UTC()
	input := &costexplorer.GetCostAndUsageWithResourcesInput{
		TimePeriod:  &types.DateInterval{Start: aws.String(end.AddDate(0, 0, -ActualWindowDays).Format(time.DateOnly)), End: aws.String(end.Format(time.DateOnly))},
		Granularity: types.GranularityDaily,
		Metrics:     []string{"UnblendedCost"},
		GroupBy:     []types.GroupDefinition{{Type: types.GroupDefinitionTypeDimension, Key: aws.String("RESOURCE_ID")}},
		Filter:      &types.Expression{Dimensions: &types.DimensionValues{Key: types.DimensionService, Values: []string{"Amazon Elastic Compute Cloud - Compute"}}},
	}
	a := Actual{}
	calls := 0
	tokens := map[string]bool{}
	for {
		calls++
		out, err := api.GetCostAndUsageWithResources(ctx, input)
		if err != nil {
			return nil, calls, err
		}
		if out == nil {
			return nil, calls, fmt.Errorf("Cost Explorer returned no response")
		}
		for _, day := range out.ResultsByTime {
			for _, g := range day.Groups {
				if len(g.Keys) == 0 || g.Keys[0] == "" || g.Keys[0] == "NoResourceId" {
					continue
				}
				metric, ok := g.Metrics["UnblendedCost"]
				if !ok {
					return nil, calls, fmt.Errorf("Cost Explorer omitted UnblendedCost for %s", g.Keys[0])
				}
				if unit := aws.ToString(metric.Unit); unit != "USD" {
					return nil, calls, fmt.Errorf("unsupported Cost Explorer currency %q", unit)
				}
				amount, err := strconv.ParseFloat(aws.ToString(metric.Amount), 64)
				if err != nil || math.IsNaN(amount) || math.IsInf(amount, 0) {
					return nil, calls, fmt.Errorf("invalid Cost Explorer amount for %s", g.Keys[0])
				}
				a[g.Keys[0]] += amount
			}
		}
		token := aws.ToString(out.NextPageToken)
		if token == "" {
			break
		}
		if tokens[token] {
			return nil, calls, fmt.Errorf("Cost Explorer repeated pagination token")
		}
		tokens[token] = true
		next := *input
		next.NextPageToken = aws.String(token)
		input = &next
	}
	for id, total := range a {
		a[id] = total / ActualWindowDays * 30
	}
	return a, calls, nil
}

func (a Actual) Lookup(r model.Resource) (float64, bool) {
	if r.ARN != "" {
		if v, ok := a[r.ARN]; ok {
			return v, true
		}
	}
	if r.ID == "" {
		return 0, false
	}
	v, ok := a[r.ID]
	return v, ok
}

func ApplyActual(r *model.Resource, monthlyUSD float64) {
	v := math.Round(monthlyUSD*100) / 100
	r.Cost = model.Cost{MonthlyUSD: &v, Source: model.CostActual, Note: "projected from last 14 days"}
}
