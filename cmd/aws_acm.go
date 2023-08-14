package cmd

import (
	"fmt"
	"lookr/deps"
	"os"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/acm"
	"github.com/olekukonko/tablewriter"
	"github.com/spf13/cobra"
)

var AcmCmd = &cobra.Command{
	Use:   "acm",
	Short: "Query AWS Certificate Manager certificates in different regions",
	Run:   queryACM,
}

func init() {
	rootCmd.AddCommand(AcmCmd)
}

func queryACM(cmd *cobra.Command, args []string) {
	table := tablewriter.NewWriter(os.Stdout)
	table.SetHeader([]string{"Certificate ARN", "Region", "Domain Name", "Status", "Type", "Validation Method"})

	AuthRegions := deps.AuthRegions()
	for _, region := range AuthRegions {
		sess, err := session.NewSession(&aws.Config{
			Region: aws.String(region),
		})

		if err != nil {
			fmt.Println("failed to create session,", err)
			return
		}

		acmClient := acm.New(sess)

		input := &acm.ListCertificatesInput{}

		result, err := acmClient.ListCertificates(input)
		if err != nil {
			fmt.Println("failed to list AWS ACM certificates,", err)
			return
		}

		regionName := deps.GetRegionName(region)

		for _, certificate := range result.CertificateSummaryList {
			describeInput := &acm.DescribeCertificateInput{
				CertificateArn: certificate.CertificateArn,
			}

			certificateDetails, err := acmClient.DescribeCertificate(describeInput)
			if err != nil {
				fmt.Println("failed to describe ACM certificate,", err)
				return
			}

			row := []string{
				*certificate.CertificateArn,
				regionName,
				*certificate.DomainName,
				*certificateDetails.Certificate.Status,
				*certificateDetails.Certificate.Type,
				*certificateDetails.Certificate.DomainValidationOptions[0].ValidationMethod,
				*certificate.CertificateArn,
			}
			table.Append(row)
		}
	}
	table.Render()
}
