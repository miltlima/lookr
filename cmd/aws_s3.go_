package cmd

import (
	"fmt"
	"lookr/deps"
	"os"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/s3"
	"github.com/olekukonko/tablewriter"
	"github.com/spf13/cobra"
)

var S3Cmd = &cobra.Command{
	Use:   "s3",
	Short: "Query S3 buckets in different regions",
	Run:   queryS3,
}

func init() {
	rootCmd.AddCommand(S3Cmd)
}

func queryS3(cmd *cobra.Command, args []string) {
	table := tablewriter.NewWriter(os.Stdout)
	table.SetHeader([]string{"Bucket Name", "Region", "Creation Date", "Size (Bytes)"})

	AuthRegions := deps.AuthRegions()
	for _, region := range AuthRegions {
		sess, err := session.NewSession(&aws.Config{
			Region: aws.String(region),
		})

		if err != nil {
			fmt.Println("failed to create session,", err)
			return
		}

		s3Client := s3.New(sess)

		input := &s3.ListBucketsInput{}

		result, err := s3Client.ListBuckets(input)
		if err != nil {
			fmt.Println("failed to list S3 buckets,", err)
			return
		}

		regionName := deps.GetRegionName(region)

		for _, bucket := range result.Buckets {
			getBucketSizeInput := &s3.ListObjectsInput{
				Bucket: aws.String(*bucket.Name),
			}

			objects, err := s3Client.ListObjects(getBucketSizeInput)
			if err != nil {
				fmt.Println("failed to list objects in S3 bucket,", err)
				return
			}

			var totalSize int64
			for _, obj := range objects.Contents {
				totalSize += *obj.Size
			}

			row := []string{
				*bucket.Name,
				regionName,
				bucket.CreationDate.String(),
				fmt.Sprintf("%d", totalSize),
			}
			table.Append(row)
		}
	}
	table.Render()
}
