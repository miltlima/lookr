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

var EbsCmd = &cobra.Command{
	Use:   "ebs",
	Short: "Query Amazon EBS volumes in different regions",
	Run:   queryEBS,
}

func init() {
	rootCmd.AddCommand(EbsCmd)
}

func queryEBS(cmd *cobra.Command, args []string) {
	table := tablewriter.NewWriter(os.Stdout)
	table.SetHeader([]string{"Volume ID", "Region", "az", "Size (GB)", "Type", "Status", "IOPS", "Encryption"})

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

		input := &ec2.DescribeVolumesInput{}

		result, err := ec2Client.DescribeVolumes(input)
		if err != nil {
			fmt.Println("failed to describe Amazon EBS volumes,", err)
			return
		}

		regionName := deps.GetRegionName(region)

		for _, volume := range result.Volumes {
			encryption := "No"
			if volume.Encrypted != nil && *volume.Encrypted {
				encryption = "Yes"
			}

			iops := ""
			if volume.Iops != nil {
				iops = fmt.Sprintf("%d", *volume.Iops)
			}

			row := []string{
				*volume.VolumeId,
				regionName,
				*volume.AvailabilityZone,
				fmt.Sprintf("%d", *volume.Size),
				*volume.VolumeType,
				*volume.State,
				iops,
				encryption,
			}
			table.Append(row)
		}
	}
	table.Render()
}
