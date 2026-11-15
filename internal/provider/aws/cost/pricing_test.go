package cost

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/pricing"
	"github.com/aws/aws-sdk-go-v2/service/pricing/types"
)

type pricingFake struct {
	inputs  []*pricing.GetProductsInput
	outputs []*pricing.GetProductsOutput
	err     error
}

func (f *pricingFake) GetProducts(_ context.Context, in *pricing.GetProductsInput, _ ...func(*pricing.Options)) (*pricing.GetProductsOutput, error) {
	x := *in
	f.inputs = append(f.inputs, &x)
	if f.err != nil {
		return nil, f.err
	}
	if len(f.outputs) == 0 {
		return &pricing.GetProductsOutput{}, nil
	}
	o := f.outputs[0]
	f.outputs = f.outputs[1:]
	return o, nil
}
func product(amount string) string {
	return fmt.Sprintf(`{"terms":{"OnDemand":{"sku.term":{"priceDimensions":{"sku.rate":{"pricePerUnit":{"USD":%q}}}}}}}`, amount)
}
func TestPricingParsesAndPaginates(t *testing.T) {
	f := &pricingFake{outputs: []*pricing.GetProductsOutput{{PriceList: []string{product("0"), product("0.20")}, NextToken: aws.String("page2")}, {PriceList: []string{product("0.08")}}}}
	p := NewPricingLookup(f)
	q := PriceQuery{ServiceCode: "AmazonEC2", Region: "us-east-1", Filters: map[string]string{"volumeApiName": "gp3", "productFamily": "Storage"}}
	v, e := p.Price(context.Background(), q)
	if e != nil || v != .08 {
		t.Fatal(v, e)
	}
	if len(f.inputs) != 2 || aws.ToString(f.inputs[1].NextToken) != "page2" {
		t.Fatal(f.inputs)
	}
	in := f.inputs[0]
	if aws.ToString(in.ServiceCode) != "AmazonEC2" || len(in.Filters) != 3 {
		t.Fatal(in)
	}
	got := map[string]string{}
	for _, filter := range in.Filters {
		if filter.Type != types.FilterTypeTermMatch {
			t.Fatal(filter)
		}
		got[aws.ToString(filter.Field)] = aws.ToString(filter.Value)
	}
	if got["regionCode"] != "us-east-1" || got["volumeApiName"] != "gp3" || got["productFamily"] != "Storage" {
		t.Fatal(got)
	}
	v, e = p.Price(context.Background(), q)
	if e != nil || v != .08 || len(f.inputs) != 2 {
		t.Fatal("uncached", v, e, len(f.inputs))
	}
}
func TestPricingMissingAndErrors(t *testing.T) {
	for _, items := range [][]string{nil, {product("0")}} {
		f := &pricingFake{outputs: []*pricing.GetProductsOutput{{PriceList: items}}}
		if _, e := NewPricingLookup(f).Price(context.Background(), PriceQuery{}); !errors.Is(e, ErrNoPrice) {
			t.Fatal(e)
		}
	}
	f := &pricingFake{err: errors.New("denied")}
	p := NewPricingLookup(f)
	q := PriceQuery{ServiceCode: "AmazonEC2"}
	if _, e := p.Price(context.Background(), q); e == nil {
		t.Fatal("missing error")
	}
	f.err = nil
	f.outputs = []*pricing.GetProductsOutput{{PriceList: []string{product(".08")}}}
	if v, e := p.Price(context.Background(), q); e != nil || v != .08 || len(f.inputs) != 2 {
		t.Fatal(v, e)
	}
}
func TestPricingConcurrentCache(t *testing.T) {
	f := &pricingFake{outputs: []*pricing.GetProductsOutput{{PriceList: []string{product(".08")}}}}
	p := NewPricingLookup(f)
	var wg sync.WaitGroup
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			v, e := p.Price(context.Background(), PriceQuery{ServiceCode: "AmazonEC2"})
			if e != nil || v != .08 {
				t.Errorf("%v %v", v, e)
			}
		}()
	}
	wg.Wait()
	if len(f.inputs) != 1 {
		t.Fatal(len(f.inputs))
	}
}
