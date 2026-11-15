package cmd

import (
	"context"
	"github.com/spf13/cobra"
	"os"
	"os/signal"
)

func newRootCommand() *cobra.Command {
	root := &cobra.Command{Use: "lookr", Short: "Read-only AWS inventory, usage and cost analysis", SilenceUsage: true, SilenceErrors: true}
	root.AddCommand(newScanCommand())
	return root
}

func Execute() error {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()
	return newRootCommand().ExecuteContext(ctx)
}
