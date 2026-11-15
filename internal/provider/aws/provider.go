package aws

import (
	"context"
	"fmt"
	"time"

	sdk "github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/ec2"
	"github.com/aws/aws-sdk-go-v2/service/sts"
	"lookr/internal/model"
	"lookr/internal/provider"
	"lookr/internal/provider/aws/collect"
	"lookr/internal/provider/aws/cost"
	"lookr/internal/provider/aws/usage"
)

type awsProvider struct{}

func New() provider.Provider     { return awsProvider{} }
func (awsProvider) Name() string { return "aws" }
func (awsProvider) Scan(ctx context.Context, o provider.ScanOptions) (model.Report, error) {
	var opts []func(*config.LoadOptions) error
	if o.Profile != "" {
		opts = append(opts, config.WithSharedConfigProfile(o.Profile))
	}
	cfg, err := config.LoadDefaultConfig(ctx, opts...)
	if err != nil {
		return model.Report{}, fmt.Errorf("load AWS configuration: %w", err)
	}
	if cfg.Region == "" {
		cfg.Region = "us-east-1"
	}
	regional := func(region string) sdk.Config {
		c := cfg
		if region == "global" {
			region = "us-east-1"
		}
		c.Region = region
		return c
	}
	d := Deps{
		AccountID: func(ctx context.Context) (string, error) {
			return readAccount(ctx, sts.NewFromConfig(cfg))
		},
		Regions: func(ctx context.Context) ([]string, error) {
			return readRegions(ctx, ec2.NewFromConfig(cfg))
		},
		Collectors: collect.All(),
		Collect: func(ctx context.Context, c collect.Collector, s collect.Scope) ([]model.Resource, error) {
			return c.Collect(ctx, regional(s.Region), s)
		},
		Metrics: func(ctx context.Context, region string, reqs []usage.Request, start, end time.Time) map[string]usage.Result {
			return usage.Fetch(ctx, usage.NewCloudWatch(regional(region)), reqs, start, end)
		},
		Actual: func(ctx context.Context, now time.Time) (cost.Actual, int, error) {
			return cost.FetchActual(ctx, cost.NewCostExplorer(cfg), now)
		},
		Prices: cost.NewPricingLookup(cost.NewPricing(cfg)),
	}
	return Run(ctx, d, o)
}

type stsAPI interface {
	GetCallerIdentity(context.Context, *sts.GetCallerIdentityInput, ...func(*sts.Options)) (*sts.GetCallerIdentityOutput, error)
}
type regionsAPI interface {
	DescribeRegions(context.Context, *ec2.DescribeRegionsInput, ...func(*ec2.Options)) (*ec2.DescribeRegionsOutput, error)
}

func readAccount(ctx context.Context, api stsAPI) (string, error) {
	out, e := api.GetCallerIdentity(ctx, &sts.GetCallerIdentityInput{})
	if e != nil {
		return "", e
	}
	if out == nil || sdk.ToString(out.Account) == "" {
		return "", fmt.Errorf("STS returned no account ID")
	}
	return sdk.ToString(out.Account), nil
}
func readRegions(ctx context.Context, api regionsAPI) ([]string, error) {
	out, e := api.DescribeRegions(ctx, &ec2.DescribeRegionsInput{})
	if e != nil {
		return nil, e
	}
	if out == nil {
		return nil, fmt.Errorf("EC2 returned no regions response")
	}
	var regions []string
	for _, r := range out.Regions {
		if name := sdk.ToString(r.RegionName); name != "" {
			regions = append(regions, name)
		}
	}
	return regions, nil
}
