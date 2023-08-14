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

var RdsCmd = &cobra.Command{
	Use:   "rds",
	Short: "Query Rds in different regions",
	Run:   queryRDS,
}

func init() {
	rootCmd.AddCommand(RdsCmd)
}
func queryRDS(cmd *cobra.Command, args []string) {

	tableData := [][]string{
		{"DB Name", "Region", "AZ", "Status", "Instance Type", "Engine", "Version", "Port", "Storage Type", "Storage Size", "Multi-AZ", "Replica", "ARN"},
	}
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

		input := &rds.DescribeDBInstancesInput{}

		result, err := rdsClient.DescribeDBInstances(input)
		if err != nil {
			fmt.Println("failed to describe db instances,", err)
			return
		}

		for _, dbInstance := range result.DBInstances {
			hasReadReplica := "No"
			if len(dbInstance.ReadReplicaDBInstanceIdentifiers) > 0 {
				hasReadReplica = "Yes"
			}

			multiAZ := "No"
			if dbInstance.MultiAZ != nil && *dbInstance.MultiAZ {
				multiAZ = "Yes"
			}

			regionName := deps.GetRegionName(region)

			row := []string{
				*dbInstance.DBInstanceIdentifier,
				regionName,
				*dbInstance.AvailabilityZone,
				*dbInstance.DBInstanceStatus,
				*dbInstance.DBInstanceClass,
				*dbInstance.Engine,
				*dbInstance.EngineVersion,
				fmt.Sprintf("%d", *dbInstance.Endpoint.Port),
				*dbInstance.StorageType,
				fmt.Sprintf("%d", *dbInstance.AllocatedStorage),
				multiAZ,
				hasReadReplica,
				*dbInstance.DBInstanceArn,
			}
			tableData = append(tableData, row)
		}
	}
	table := tablewriter.NewWriter(os.Stdout)
	table.SetHeader(tableData[0])
	table.SetColumnAlignment([]int{
		tablewriter.ALIGN_CENTER,
		tablewriter.ALIGN_CENTER,
		tablewriter.ALIGN_CENTER,
		tablewriter.ALIGN_CENTER,
		tablewriter.ALIGN_CENTER,
		tablewriter.ALIGN_CENTER,
		tablewriter.ALIGN_CENTER,
		tablewriter.ALIGN_CENTER,
		tablewriter.ALIGN_CENTER,
		tablewriter.ALIGN_CENTER,
		tablewriter.ALIGN_CENTER,
		tablewriter.ALIGN_CENTER,
		tablewriter.ALIGN_CENTER,
		tablewriter.ALIGN_CENTER,
		tablewriter.ALIGN_CENTER,
	})
	table.AppendBulk(tableData[1:])
	table.Render()
}
