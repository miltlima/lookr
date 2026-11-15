package collect

import (
	"context"
	"errors"
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/elasticache"
	"github.com/aws/aws-sdk-go-v2/service/elasticache/types"
	"testing"
)

type elasticacheFake struct {
	calls int
	fail  bool
}

func (f *elasticacheFake) DescribeCacheClusters(ctx context.Context, in *elasticache.DescribeCacheClustersInput, opt ...func(*elasticache.Options)) (*elasticache.DescribeCacheClustersOutput, error) {
	f.calls++
	if f.fail {
		return nil, errors.New("denied")
	}
	o := &elasticache.DescribeCacheClustersOutput{CacheClusters: []types.CacheCluster{{}}}
	if f.calls == 1 {
		o.Marker = aws.String("next")
	}
	return o, nil
}
func TestCollectCacheClusters(t *testing.T) {
	f := &elasticacheFake{}
	r, e := collectCacheClusters(context.Background(), f, Scope{Region: "r"})
	if e != nil || len(r) != 2 || f.calls != 2 {
		t.Fatalf("%v %v", r, e)
	}
	f = &elasticacheFake{fail: true}
	if _, e = collectCacheClusters(context.Background(), f, Scope{}); e == nil {
		t.Fatal("missing error")
	}
}

type taggedCacheFake struct{ elasticacheFake }

func (*taggedCacheFake) ListTagsForResource(context.Context, *elasticache.ListTagsForResourceInput, ...func(*elasticache.Options)) (*elasticache.ListTagsForResourceOutput, error) {
	return &elasticache.ListTagsForResourceOutput{TagList: []types.Tag{{Key: aws.String("Name"), Value: aws.String("named")}}}, nil
}
func TestCacheTags(t *testing.T) {
	r, e := collectCacheClusters(context.Background(), &taggedCacheFake{}, Scope{})
	if e != nil || r[0].Name != "named" {
		t.Fatalf("%v %v", r, e)
	}
}
