package output

import (
	"encoding/csv"
	"encoding/json"
	"io"
	"strconv"
	"time"

	"lookr/internal/model"
)

var csvHeader = []string{
	"provider", "account_id", "region", "type", "id", "arn", "name", "created_at",
	"verdict", "reason", "monthly_usd", "cost_source", "cost_note",
	"tags", "config", "metrics", "findings",
}

// WriteCSV writes one row per resource. The last four columns hold compact JSON.
func WriteCSV(w io.Writer, r model.Report) error {
	cw := csv.NewWriter(w)
	if err := cw.Write(csvHeader); err != nil {
		return err
	}
	for _, res := range r.Resources {
		var created, cost string
		if res.CreatedAt != nil {
			created = res.CreatedAt.Format(time.RFC3339)
		}
		if res.Cost.MonthlyUSD != nil {
			cost = strconv.FormatFloat(*res.Cost.MonthlyUSD, 'f', 2, 64)
		}
		tags, err := compactJSON(res.Tags)
		if err != nil {
			return err
		}
		config, err := compactJSON(res.Config)
		if err != nil {
			return err
		}
		metrics, err := compactJSON(res.Usage.Metrics)
		if err != nil {
			return err
		}
		findings, err := compactJSON(res.Findings)
		if err != nil {
			return err
		}
		row := []string{
			res.Provider, res.AccountID, res.Region, res.Type, res.ID, res.ARN, res.Name, created,
			string(res.Usage.Verdict), res.Usage.Reason, cost, string(res.Cost.Source), res.Cost.Note,
			tags, config, metrics, findings,
		}
		if err := cw.Write(row); err != nil {
			return err
		}
	}
	cw.Flush()
	return cw.Error()
}

func compactJSON(v any) (string, error) {
	b, err := json.Marshal(v)
	return string(b), err
}
