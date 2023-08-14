package cmd

import (
	"fmt"
	"lookr/deps"
	"os"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/lambda"
	"github.com/olekukonko/tablewriter"
	"github.com/spf13/cobra"
)

var LambdaCmd = &cobra.Command{
	Use:   "lambda",
	Short: "Query AWS Lambda functions in different regions",
	Run:   queryLambda,
}

func init() {
	rootCmd.AddCommand(LambdaCmd)
}

func queryLambda(cmd *cobra.Command, args []string) {
	table := tablewriter.NewWriter(os.Stdout)
	table.SetHeader([]string{"Function Name", "Region", "Runtime", "Handler", "Memory (MB)", "Timeout (s)", "Arn"})

	AuthRegions := deps.AuthRegions()
	for _, region := range AuthRegions {
		sess, err := session.NewSession(&aws.Config{
			Region: aws.String(region),
		})

		if err != nil {
			fmt.Println("failed to create session,", err)
			return
		}

		lambdaClient := lambda.New(sess)

		input := &lambda.ListFunctionsInput{}

		result, err := lambdaClient.ListFunctions(input)
		if err != nil {
			fmt.Println("failed to list AWS Lambda functions,", err)
			return
		}

		regionName := deps.GetRegionName(region)

		for _, function := range result.Functions {
			row := []string{
				*function.FunctionName,
				regionName,
				*function.Runtime,
				*function.Handler,
				fmt.Sprintf("%d", *function.MemorySize),
				fmt.Sprintf("%d", *function.Timeout),
				*function.FunctionArn,
			}
			table.Append(row)
		}
	}
	table.Render()
}
