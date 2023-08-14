package cmd

import (
	"fmt"
	"lookr/deps"
	"os"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/cloudfront"
	"github.com/olekukonko/tablewriter"
	"github.com/spf13/cobra"
)

var CloudFrontCmd = &cobra.Command{
	Use:   "cloudfront",
	Short: "Query Amazon CloudFront distributions in different regions",
	Run:   queryCloudFront,
}

func init() {
	rootCmd.AddCommand(CloudFrontCmd)
}

func queryCloudFront(cmd *cobra.Command, args []string) {
	table := tablewriter.NewWriter(os.Stdout)
	table.SetHeader([]string{"Distribution ID", "Region", "Domain Name", "Status", "Default Cache Behavior"})

	AuthRegions := deps.AuthRegions()
	for _, region := range AuthRegions {
		sess, err := session.NewSession(&aws.Config{
			Region: aws.String(region),
		})

		if err != nil {
			fmt.Println("failed to create session,", err)
			return
		}

		cfClient := cloudfront.New(sess)

		input := &cloudfront.ListDistributionsInput{}

		result, err := cfClient.ListDistributions(input)
		if err != nil {
			fmt.Println("failed to list Amazon CloudFront distributions,", err)
			return
		}

		regionName := deps.GetRegionName(region)

		for _, distribution := range result.DistributionList.Items {
			defaultCacheBehavior := "N/A"
			if distribution.DefaultCacheBehavior != nil {
				defaultCacheBehavior = *distribution.DefaultCacheBehavior.TargetOriginId
			}

			row := []string{
				*distribution.Id,
				regionName,
				*distribution.DomainName,
				*distribution.Status,
				defaultCacheBehavior,
				*distribution.ARN,
			}
			table.Append(row)
		}
	}
	table.Render()
}
