package cmd

import (
	"fmt"
	"lookr/deps"
	"os"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/elbv2"
	"github.com/olekukonko/tablewriter"
	"github.com/spf13/cobra"
)

var ElbCmd = &cobra.Command{
	Use:   "elb",
	Short: "Query ELB Load Balancers in different regions",
	Run:   queryELB,
}

func init() {
	rootCmd.AddCommand(ElbCmd)
}

func queryELB(cmd *cobra.Command, args []string) {
	table := tablewriter.NewWriter(os.Stdout)
	table.SetHeader([]string{"Load Balancer Name", "Region", "DNS Name", "Scheme", "Type", "State"})

	AuthRegions := deps.AuthRegions()
	for _, region := range AuthRegions {
		sess, err := session.NewSession(&aws.Config{
			Region: aws.String(region),
		})

		if err != nil {
			fmt.Println("failed to create session,", err)
			return
		}

		elbv2Client := elbv2.New(sess)

		input := &elbv2.DescribeLoadBalancersInput{}

		result, err := elbv2Client.DescribeLoadBalancers(input)
		if err != nil {
			fmt.Println("failed to describe ELB Load Balancers,", err)
			return
		}

		regionName := deps.GetRegionName(region)

		for _, lb := range result.LoadBalancers {
			row := []string{
				*lb.LoadBalancerName,
				regionName,
				*lb.AvailabilityZones[0].ZoneName,
				*lb.DNSName,
				*lb.Scheme,
				*lb.Type,
				*lb.State.Code,
				*lb.LoadBalancerArn,
			}
			table.Append(row)
		}
	}
	table.Render()
}
