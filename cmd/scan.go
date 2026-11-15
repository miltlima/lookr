package cmd

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"slices"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"lookr/internal/output"
	"lookr/internal/provider"
	awsprovider "lookr/internal/provider/aws"
	"lookr/internal/provider/aws/collect"
)

var newProvider = func() provider.Provider { return awsprovider.New() }

func newScanCommand() *cobra.Command {
	var days, concurrency int
	var skipMetrics bool
	var regions, types, format, file, costMode, profile string
	c := &cobra.Command{Use: "scan", Short: "Scan AWS resources for inventory, usage, cost and configuration findings", Args: cobra.NoArgs}
	c.Flags().IntVar(&days, "days", 30, "Usage observation window in days")
	c.Flags().StringVar(&regions, "regions", "", "Comma-separated regions (default: all enabled regions)")
	c.Flags().StringVar(&types, "type", "", "Comma-separated resource types (default: all)")
	c.Flags().StringVar(&format, "output", "table", "Output format: table, json, csv")
	c.Flags().StringVar(&file, "out-file", "", "Write report to this file instead of stdout")
	c.Flags().StringVar(&costMode, "cost", "auto", "Cost mode: auto, estimate, none")
	c.Flags().StringVar(&profile, "profile", "", "AWS shared configuration profile")
	c.Flags().IntVar(&concurrency, "concurrency", 8, "Maximum regions collected concurrently")
	c.Flags().BoolVar(&skipMetrics, "no-metrics", false, "Disable paid CloudWatch usage queries")
	c.RunE = func(c *cobra.Command, _ []string) (err error) {
		if days <= 0 {
			return fmt.Errorf("--days must be a positive integer (accepted: 1 or greater)")
		}
		if concurrency <= 0 {
			return fmt.Errorf("--concurrency must be a positive integer (accepted: 1 or greater)")
		}
		if !slices.Contains([]string{"table", "json", "csv"}, format) {
			return fmt.Errorf("--output must be one of: table, json, csv")
		}
		if !slices.Contains([]string{"auto", "estimate", "none"}, costMode) {
			return fmt.Errorf("--cost must be one of: auto, estimate, none")
		}
		selected := parseList(types)
		accepted := collect.Shorts()
		for _, typ := range selected {
			if !slices.Contains(accepted, typ) {
				return fmt.Errorf("--type %q is invalid; accepted: %s", typ, strings.Join(accepted, ", "))
			}
		}
		var destination io.Writer = c.OutOrStdout()
		if file != "" {
			f, openErr := os.Create(file)
			if openErr != nil {
				return fmt.Errorf("--out-file: %w", openErr)
			}
			defer func() {
				if closeErr := f.Close(); err == nil && closeErr != nil {
					err = closeErr
				}
			}()
			destination = f
		}
		var progress bytes.Buffer
		o := provider.ScanOptions{SkipMetrics: skipMetrics, Days: days, Regions: parseList(regions), Types: selected, CostMode: costMode, Concurrency: concurrency, Profile: profile, Now: time.Now().UTC(), Progress: io.MultiWriter(c.ErrOrStderr(), &progress)}
		report, err := newProvider().Scan(c.Context(), o)
		if err != nil {
			return err
		}
		for _, w := range report.Warnings {
			line := fmt.Sprintf("Warning [%s/%s]: %s\n", w.Region, w.Type, w.Error)
			if !strings.Contains(progress.String(), line) {
				if _, err := io.WriteString(c.ErrOrStderr(), line); err != nil {
					return err
				}
			}
		}
		switch format {
		case "json":
			return output.WriteJSON(destination, report)
		case "csv":
			return output.WriteCSV(destination, report)
		default:
			return output.WriteTable(destination, report)
		}
	}
	return c
}

func parseList(value string) []string {
	var result []string
	for _, part := range strings.Split(value, ",") {
		if part = strings.TrimSpace(part); part != "" && !slices.Contains(result, part) {
			result = append(result, part)
		}
	}
	return result
}
