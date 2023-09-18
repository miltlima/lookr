package cmd

import (
	"fmt"
	"lookr/deps"
	"os"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/dynamodb"
	"github.com/olekukonko/tablewriter"
	"github.com/spf13/cobra"
)

var DynamoDBCmd = &cobra.Command{
	Use:   "dynamodb",
	Short: "Query Amazon DynamoDB tables in different regions",
	Run:   queryDynamoDB,
}

func init() {
	rootCmd.AddCommand(DynamoDBCmd)
}

func queryDynamoDB(cmd *cobra.Command, args []string) {
	table := tablewriter.NewWriter(os.Stdout)
	table.SetHeader([]string{"Table Name", "Region", "Status", "Item Count", "Size (Bytes)", "Provisioned Throughput", "arn"})

	AuthRegions := deps.AuthRegions()
	for _, region := range AuthRegions {
		sess, err := session.NewSession(&aws.Config{
			Region: aws.String(region),
		})

		if err != nil {
			fmt.Println("failed to create session,", err)
			return
		}

		dynamoDBClient := dynamodb.New(sess)

		input := &dynamodb.ListTablesInput{}

		result, err := dynamoDBClient.ListTables(input)
		if err != nil {
			fmt.Println("failed to list Amazon DynamoDB tables,", err)
			return
		}

		regionName := deps.GetRegionName(region)

		for _, tableName := range result.TableNames {
			describeInput := &dynamodb.DescribeTableInput{
				TableName: tableName,
			}

			tableDetails, err := dynamoDBClient.DescribeTable(describeInput)
			if err != nil {
				fmt.Println("failed to describe DynamoDB table,", err)
				return
			}

			provisionedThroughput := ""
			if tableDetails.Table.ProvisionedThroughput != nil {
				provisionedThroughput = fmt.Sprintf("Read: %d, Write: %d",
					*tableDetails.Table.ProvisionedThroughput.ReadCapacityUnits,
					*tableDetails.Table.ProvisionedThroughput.WriteCapacityUnits)
			}

			row := []string{
				*tableName,
				regionName,
				*tableDetails.Table.TableStatus,
				fmt.Sprintf("%d", *tableDetails.Table.ItemCount),
				fmt.Sprintf("%d", *tableDetails.Table.TableSizeBytes),
				provisionedThroughput,
				*tableDetails.Table.TableArn,
			}
			table.Append(row)
		}
	}
	table.Render()
}
