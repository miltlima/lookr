// Package provider defines the common scan interface.
package provider

import (
	"context"
	"io"
	"lookr/internal/model"
	"time"
)

type ScanOptions struct {
	Days        int
	Regions     []string
	Types       []string
	CostMode    string
	Concurrency int
	Profile     string
	Now         time.Time
	Progress    io.Writer
}
type Provider interface {
	Name() string
	Scan(context.Context, ScanOptions) (model.Report, error)
}
