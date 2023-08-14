package cmd

import (
	"fmt"
	"lookr/deps"
	"os"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/eks"
	"github.com/olekukonko/tablewriter"
	"github.com/spf13/cobra"
)

var EksCmd = &cobra.Command{
	Use:   "eks",
	Short: "Query EKS clusters in different regions",
	Run:   queryEKS,
}

func init() {
	rootCmd.AddCommand(EksCmd)
}

func queryEKS(cmd *cobra.Command, args []string) {
	table := tablewriter.NewWriter(os.Stdout)
	table.SetHeader([]string{"Cluster Name", "Region", "Status", "Endpoint", "Kubernetes Version", "Arn"})

	AuthRegions := deps.AuthRegions()
	for _, region := range AuthRegions {
		sess, err := session.NewSession(&aws.Config{
			Region: aws.String(region),
		})

		if err != nil {
			fmt.Println("failed to create session,", err)
			return
		}

		eksClient := eks.New(sess)

		input := &eks.ListClustersInput{}

		result, err := eksClient.ListClusters(input)
		if err != nil {
			fmt.Println("failed to list EKS clusters,", err)
			return
		}

		regionName := deps.GetRegionName(region)

		for _, clusterName := range result.Clusters {
			describeInput := &eks.DescribeClusterInput{
				Name: aws.String(*clusterName),
			}

			clusterDetails, err := eksClient.DescribeCluster(describeInput)
			if err != nil {
				fmt.Println("failed to describe EKS cluster,", err)
				return
			}

			cluster := clusterDetails.Cluster
			row := []string{
				*cluster.Name,
				regionName,
				*cluster.Status,
				*cluster.Endpoint,
				*cluster.Version,
				*cluster.Arn,
			}
			table.Append(row)
		}
	}
	table.Render()
}
