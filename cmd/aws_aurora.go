package cmd

import (
	"fmt"
	"lookr/deps"
	"os"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/rds"
	"github.com/olekukonko/tablewriter"
	"github.com/spf13/cobra"
)

var AuroraCmd = &cobra.Command{
	Use:   "aurora",
	Short: "Query Amazon Aurora clusters in different regions",
	Run:   queryAurora,
}

func init() {
	rootCmd.AddCommand(AuroraCmd)
}

func queryAurora(cmd *cobra.Command, args []string) {
	table := tablewriter.NewWriter(os.Stdout)
	table.SetHeader([]string{"Cluster Name", "Region", "Status", "Engine", "Engine Version", "DB Instances", "Replicas", "arn"})

	AuthRegions := deps.AuthRegions()
	for _, region := range AuthRegions {
		sess, err := session.NewSession(&aws.Config{
			Region: aws.String(region),
		})

		if err != nil {
			fmt.Println("failed to create session,", err)
			return
		}

		rdsClient := rds.New(sess)

		input := &rds.DescribeDBClustersInput{}

		result, err := rdsClient.DescribeDBClusters(input)
		if err != nil {
			fmt.Println("failed to describe Amazon Aurora clusters,", err)
			return
		}

		regionName := deps.GetRegionName(region)

		for _, cluster := range result.DBClusters {
			dbInstances := ""
			for _, instance := range cluster.DBClusterMembers {
				dbInstances += *instance.DBInstanceIdentifier + ", "
			}

			row := []string{
				*cluster.DBClusterIdentifier,
				regionName,
				*cluster.Status,
				*cluster.Engine,
				*cluster.EngineVersion,
				dbInstances,
				*cluster.ReadReplicaIdentifiers[0],
				*cluster.DBClusterArn,
			}
			table.Append(row)
		}
	}
	table.Render()
}
