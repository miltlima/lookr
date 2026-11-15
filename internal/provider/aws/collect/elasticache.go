package collect

import (
	"context"
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/elasticache"
	"lookr/internal/model"
)

type cacheAPI interface {
	DescribeCacheClusters(context.Context, *elasticache.DescribeCacheClustersInput, ...func(*elasticache.Options)) (*elasticache.DescribeCacheClustersOutput, error)
}

func collectCacheClusters(ctx context.Context, c cacheAPI, s Scope) ([]model.Resource, error) {
	var result []model.Resource
	in := &elasticache.DescribeCacheClustersInput{ShowCacheNodeInfo: aws.Bool(true)}
	for {
		o, e := c.DescribeCacheClusters(ctx, in)
		if e != nil {
			return result, e
		}
		for _, d := range o.CacheClusters {
			r := resource(s, "aws.elasticache.cluster", "elasticache", aws.ToString(d.CacheClusterId), "", nil, map[string]any{"engine": aws.ToString(d.Engine), "engine_version": aws.ToString(d.EngineVersion), "node_type": aws.ToString(d.CacheNodeType), "num_nodes": int(aws.ToInt32(d.NumCacheNodes)), "status": aws.ToString(d.CacheClusterStatus), "replication_group_id": aws.ToString(d.ReplicationGroupId)})
			nodes := []string{}
			for _, n := range d.CacheNodes {
				if id := aws.ToString(n.CacheNodeId); id != "" {
					nodes = append(nodes, id)
				}
			}
			r.Config["cache_node_ids"] = nodes
			r.ARN = aws.ToString(d.ARN)
			r.CreatedAt = d.CacheClusterCreateTime
			if tc, ok := c.(cacheTagAPI); ok {
				tagged, err := tc.ListTagsForResource(ctx, &elasticache.ListTagsForResourceInput{ResourceName: aws.String(r.ARN)})
				if err != nil {
					result = append(result, r)
					return result, err
				}
				tags := map[string]string{}
				for _, t := range tagged.TagList {
					tags[aws.ToString(t.Key)] = aws.ToString(t.Value)
				}
				r.Tags = tags
				if tags["Name"] != "" {
					r.Name = tags["Name"]
				}
			}
			result = append(result, r)
		}
		if aws.ToString(o.Marker) == "" {
			break
		}
		in.Marker = o.Marker
	}
	return result, nil
}

type cacheTagAPI interface {
	ListTagsForResource(context.Context, *elasticache.ListTagsForResourceInput, ...func(*elasticache.Options)) (*elasticache.ListTagsForResourceOutput, error)
}
