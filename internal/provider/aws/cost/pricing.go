package cost

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"sort"
	"strconv"
	"sync"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/pricing"
	"github.com/aws/aws-sdk-go-v2/service/pricing/types"
)

var ErrNoPrice = errors.New("no price found")

type PriceQuery struct {
	ServiceCode string
	Region      string
	Filters     map[string]string
}
type PriceLookup interface {
	Price(context.Context, PriceQuery) (float64, error)
}
type pricingAPI interface {
	GetProducts(context.Context, *pricing.GetProductsInput, ...func(*pricing.Options)) (*pricing.GetProductsOutput, error)
}
type priceLookup struct {
	api   pricingAPI
	mu    sync.Mutex
	cache map[string]float64
}

func NewPricing(cfg aws.Config) pricingAPI {
	cfg.Region = "us-east-1"
	return pricing.NewFromConfig(cfg)
}
func NewPricingLookup(api pricingAPI) PriceLookup {
	return &priceLookup{api: api, cache: map[string]float64{}}
}

// Price queries the product region independently of the API endpoint. All
// filters are ANDed. It selects the lowest positive on-demand dimension and
// caches successful lookups only. Product format and pagination reference:
// https://docs.aws.amazon.com/aws-cost-management/latest/APIReference/API_pricing_GetProducts.html
func (p *priceLookup) Price(ctx context.Context, q PriceQuery) (float64, error) {
	keyBytes, err := json.Marshal(q)
	if err != nil {
		return 0, err
	}
	key := string(keyBytes)
	// Serializing misses also coalesces identical concurrent calls, avoiding
	// duplicate paid or throttled upstream work during a scan.
	p.mu.Lock()
	defer p.mu.Unlock()
	if v, ok := p.cache[key]; ok {
		return v, nil
	}
	if err := ctx.Err(); err != nil {
		return 0, err
	}
	if p.api == nil {
		return 0, fmt.Errorf("pricing client unavailable")
	}
	input := &pricing.GetProductsInput{ServiceCode: aws.String(q.ServiceCode), FormatVersion: aws.String("aws_v1"), MaxResults: aws.Int32(100)}
	filters := map[string]string{}
	for k, v := range q.Filters {
		filters[k] = v
	}
	if q.Region != "" {
		filters["regionCode"] = q.Region
	}
	keys := make([]string, 0, len(filters))
	for k := range filters {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		input.Filters = append(input.Filters, types.Filter{Field: aws.String(k), Value: aws.String(filters[k]), Type: types.FilterTypeTermMatch})
	}
	best := math.Inf(1)
	tokens := map[string]bool{}
	for {
		out, err := p.api.GetProducts(ctx, input)
		if err != nil {
			return 0, err
		}
		if out == nil {
			return 0, fmt.Errorf("Pricing API returned no response")
		}
		for _, raw := range out.PriceList {
			var product struct {
				Terms struct {
					OnDemand map[string]struct {
						PriceDimensions map[string]struct{ PricePerUnit map[string]string }
					}
				}
			}
			if err := json.Unmarshal([]byte(raw), &product); err != nil {
				return 0, fmt.Errorf("invalid price document: %w", err)
			}
			for _, term := range product.Terms.OnDemand {
				for _, dimension := range term.PriceDimensions {
					value, ok := dimension.PricePerUnit["USD"]
					if !ok {
						continue
					}
					price, err := strconv.ParseFloat(value, 64)
					if err != nil || math.IsNaN(price) || math.IsInf(price, 0) {
						return 0, fmt.Errorf("invalid USD list price %q", value)
					}
					if price > 0 && price < best {
						best = price
					}
				}
			}
		}
		token := aws.ToString(out.NextToken)
		if token == "" {
			break
		}
		if tokens[token] {
			return 0, fmt.Errorf("Pricing API repeated pagination token")
		}
		tokens[token] = true
		next := *input
		next.NextToken = aws.String(token)
		input = &next
	}
	if math.IsInf(best, 1) {
		return 0, ErrNoPrice
	}
	p.cache[key] = best
	return best, nil
}
