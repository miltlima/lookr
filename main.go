package main

import (
	"fmt"
	"lookr/cmd"
	"os"

	"github.com/spf13/cobra"
)

func main() {
	var rootCmd = &cobra.Command{Use: "lookr"}
	rootCmd.AddCommand(
		cmd.AcmCmd,
		cmd.AuroraCmd,
		cmd.CloudFrontCmd,
		cmd.DynamoDBCmd,
		cmd.EbsCmd,
		cmd.EC2Cmd,
		cmd.ElastiCacheCmd,
		cmd.ElbCmd,
		cmd.EksCmd,
		cmd.IAMCmd,
		cmd.LambdaCmd,
		cmd.RdsCmd,
		cmd.Route53Cmd,
		cmd.SqsCmd,
	)
	if err := rootCmd.Execute(); err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
}
