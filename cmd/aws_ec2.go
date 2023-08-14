package cmd

import (
	"fmt"
	"lookr/deps"
	"os"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/ec2"
	"github.com/olekukonko/tablewriter"
	"github.com/spf13/cobra"
)

var EC2Cmd = &cobra.Command{
	Use:   "ec2",
	Short: "Query EC2 instances in different regions",
	Run:   queryEC2,
}

func init() {
	rootCmd.AddCommand(EC2Cmd)
}

func queryEC2(cmd *cobra.Command, args []string) {
	table := tablewriter.NewWriter(os.Stdout)
	table.SetHeader([]string{"Instance ID", "Region", "Instance Type", "State", "Private IP", "Public IP"})

	AuthRegions := deps.AuthRegions()
	for _, region := range AuthRegions {
		sess, err := session.NewSession(&aws.Config{
			Region: aws.String(region),
		})

		if err != nil {
			fmt.Println("failed to create session,", err)
			return
		}

		ec2Client := ec2.New(sess)

		input := &ec2.DescribeInstancesInput{}

		result, err := ec2Client.DescribeInstances(input)
		if err != nil {
			fmt.Println("failed to describe EC2 instances,", err)
			return
		}

		regionName := deps.GetRegionName(region)

		for _, reservation := range result.Reservations {
			for _, instance := range reservation.Instances {
				row := []string{
					*instance.InstanceId,
					regionName,
					*instance.InstanceType,
					*instance.State.Name,
					*instance.PrivateIpAddress,
					*instance.PublicIpAddress,
					*instance.SecurityGroups[0].GroupName,
					*instance.ImageId,
					*instance.IamInstanceProfile.Arn,
				}
				table.Append(row)
			}
		}
	}
	table.Render()
}
