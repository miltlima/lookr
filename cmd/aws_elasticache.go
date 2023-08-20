package cmd

import (
	"fmt"
	"lookr/deps"
	"os"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/elasticache"
	"github.com/olekukonko/tablewriter"
	"github.com/spf13/cobra"
)

var ElastiCacheCmd = &cobra.Command{
	Use:   "elasticache",
	Short: "Query Amazon ElastiCache clusters in different regions",
	Run:   queryElastiCache,
}

func init() {
	rootCmd.AddCommand(ElastiCacheCmd)
}

func queryElastiCache(cmd *cobra.Command, args []string) {
	table := tablewriter.NewWriter(os.Stdout)
	table.SetHeader([]string{"Cluster ID", "Region", "Engine", "Engine Version", "Status", "Node Type", "Nodes", "ARN"})

	AuthRegions := deps.AuthRegions()
	for _, region := range AuthRegions {
		sess, err := session.NewSession(&aws.Config{
			Region: aws.String(region),
		})

		if err != nil {
			fmt.Println("failed to create session,", err)
			return
		}

		elastiCacheClient := elasticache.New(sess)

		input := &elasticache.DescribeCacheClustersInput{}

		result, err := elastiCacheClient.DescribeCacheClusters(input)
		if err != nil {
			fmt.Println("failed to describe Amazon ElastiCache clusters,", err)
			return
		}

		regionName := deps.GetRegionName(region)

		for _, cluster := range result.CacheClusters {
			row := []string{
				*cluster.CacheClusterId,
				regionName,
				*cluster.Engine,
				*cluster.EngineVersion,
				*cluster.CacheClusterStatus,
				*cluster.CacheNodeType,
				fmt.Sprintf("%d", *cluster.NumCacheNodes),
				*cluster.ARN,
			}
			table.Append(row)
		}
	}
	table.Render()
}
