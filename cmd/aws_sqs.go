package cmd

import (
	"fmt"
	"lookr/deps"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/sqs"
	"github.com/olekukonko/tablewriter"
	"github.com/spf13/cobra"
)

var SqsCmd = &cobra.Command{
	Use:   "sqs",
	Short: "Query Amazon SQS queues in different regions",
	Run:   querySQS,
}

func init() {
	rootCmd.AddCommand(SqsCmd)
}

func querySQS(cmd *cobra.Command, args []string) {
	table := tablewriter.NewWriter(os.Stdout)
	table.SetHeader([]string{"Queue Name", "Region", "Visibility Timeout", "Approximate Messages", "Created Timestamp", "Arn"})

	AuthRegions := deps.AuthRegions()
	for _, region := range AuthRegions {
		sess, err := session.NewSession(&aws.Config{
			Region: aws.String(region),
		})

		if err != nil {
			fmt.Println("failed to create session,", err)
			return
		}

		sqsClient := sqs.New(sess)

		input := &sqs.ListQueuesInput{}

		result, err := sqsClient.ListQueues(input)
		if err != nil {
			fmt.Println("failed to list Amazon SQS queues,", err)
			return
		}

		regionName := deps.GetRegionName(region)

		for _, queueURL := range result.QueueUrls {
			getQueueAttributesInput := &sqs.GetQueueAttributesInput{
				QueueUrl: queueURL,
				AttributeNames: []*string{
					aws.String("VisibilityTimeout"),
					aws.String("ApproximateNumberOfMessages"),
					aws.String("CreatedTimestamp"),
					aws.String("Arn"),
				},
			}

			attributes, err := sqsClient.GetQueueAttributes(getQueueAttributesInput)
			if err != nil {
				fmt.Println("failed to get queue attributes,", err)
				return
			}

			row := []string{
				queueNameFromURL(*queueURL),
				regionName,
				*attributes.Attributes["VisibilityTimeout"],
				*attributes.Attributes["ApproximateNumberOfMessages"],
				timestampToTimeString(*attributes.Attributes["CreatedTimestamp"]),
				*attributes.Attributes["Arn"],
			}
			table.Append(row)
		}
	}
	table.Render()
}

func queueNameFromURL(url string) string {
	parts := splitLast(url, "/")
	if len(parts) > 0 {
		return parts[len(parts)-1]
	}
	return url
}

func splitLast(s, sep string) []string {
	parts := strings.Split(s, sep)
	if len(parts) == 0 {
		return nil
	}
	return parts
}

func timestampToTimeString(timestamp string) string {
	timestampInt64, err := strconv.ParseInt(timestamp, 10, 64)
	if err != nil {
		return timestamp
	}
	return time.Unix(timestampInt64, 0).String()
}
