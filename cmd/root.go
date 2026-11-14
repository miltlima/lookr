package cmd

import (
	"github.com/spf13/cobra"
)

var rootCmd = &cobra.Command{Use: "lookr", SilenceUsage: true}

func Execute() error {
	return rootCmd.Execute()
}
