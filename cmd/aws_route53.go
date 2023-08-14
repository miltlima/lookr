package cmd

import (
	"fmt"
	"lookr/deps"
	"os"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/route53"
	"github.com/olekukonko/tablewriter"
	"github.com/spf13/cobra"
)

var Route53Cmd = &cobra.Command{
	Use:   "route53",
	Short: "Query Route 53 hosted zones in different regions",
	Run:   queryRoute53,
}

func init() {
	rootCmd.AddCommand(Route53Cmd)
}

func queryRoute53(cmd *cobra.Command, args []string) {
	table := tablewriter.NewWriter(os.Stdout)
	table.SetHeader([]string{"Hosted Zone Name", "Region", "Private", "Record Count"})

	AuthRegions := deps.AuthRegions()
	for _, region := range AuthRegions {
		sess, err := session.NewSession(&aws.Config{
			Region: aws.String(region),
		})

		if err != nil {
			fmt.Println("failed to create session,", err)
			return
		}

		route53Client := route53.New(sess)

		input := &route53.ListHostedZonesInput{}

		result, err := route53Client.ListHostedZones(input)
		if err != nil {
			fmt.Println("failed to list Route 53 hosted zones,", err)
			return
		}

		regionName := deps.GetRegionName(region)

		for _, hostedZone := range result.HostedZones {
			isPrivate := "No"
			if *hostedZone.Config.PrivateZone {
				isPrivate = "Yes"
			}

			getHostedZoneInput := &route53.GetHostedZoneInput{
				Id: hostedZone.Id,
			}

			getHostedZoneOutput, err := route53Client.GetHostedZone(getHostedZoneInput)
			if err != nil {
				fmt.Println("failed to get hosted zone,", err)
				continue
			}

			row := []string{
				*hostedZone.Name,
				regionName,
				isPrivate,
				fmt.Sprintf("%d", *getHostedZoneOutput.HostedZone.ResourceRecordSetCount),
			}
			table.Append(row)
		}
	}
	table.Render()
}
