package output

import (
	"fmt"
	"io"
	"sort"
	"strconv"
	"strings"
	"text/tabwriter"

	"lookr/internal/model"
)

const maxCell = 40

// WriteTable writes a summary, the resource table and the findings list.
func WriteTable(out io.Writer, r model.Report) error {
	w := &errWriter{w: out}
	s := r.Summary
	pct := 0
	if s.MonthlyUSD > 0 {
		pct = int(s.WasteMonthlyUSD/s.MonthlyUSD*100 + 0.5)
	}
	fmt.Fprintf(w, "Account %s · %s · %s · %s/mo\n", r.AccountID,
		plural(len(r.Regions), "region"), plural(s.Resources, "resource"), money(s.MonthlyUSD))
	fmt.Fprintf(w, "Idle or orphan: %s · %s/mo (%d%%)\n",
		plural(s.WasteResources, "resource"), money(s.WasteMonthlyUSD), pct)
	fmt.Fprintf(w, "Findings: %d high · %d medium · %d low\n",
		s.FindingsBySeverity[model.SeverityHigh], s.FindingsBySeverity[model.SeverityMedium], s.FindingsBySeverity[model.SeverityLow])
	if len(r.Warnings) > 0 {
		fmt.Fprintf(w, "Warnings: %d (see stderr)\n", len(r.Warnings))
	}
	fmt.Fprintln(w)

	if len(r.Resources) == 0 {
		fmt.Fprintln(w, "No resources found.")
		return w.err
	}

	tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
	fmt.Fprintln(tw, "TYPE\tRESOURCE\tREGION\tVERDICT\tREASON\tCOST/MO\tISSUES")
	for _, res := range r.Resources {
		issues := "-"
		if n := len(res.Findings); n > 0 {
			issues = strconv.Itoa(n)
		}
		fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%s\t%s\t%s\n",
			res.Short, truncate(resourceLabel(res)), res.Region, res.Usage.Verdict,
			truncate(res.Usage.Reason), costLabel(res.Cost), issues)
	}
	tw.Flush()
	writeFindings(w, r.Resources)
	return w.err
}

// errWriter remembers the first write error and drops later writes.
type errWriter struct {
	w   io.Writer
	err error
}

func (e *errWriter) Write(p []byte) (int, error) {
	if e.err != nil {
		return 0, e.err
	}
	n, err := e.w.Write(p)
	e.err = err
	return n, err
}

type findingRow struct {
	res model.Resource
	f   model.Finding
}

func writeFindings(w io.Writer, resources []model.Resource) {
	var rows []findingRow
	for _, res := range resources {
		for _, f := range res.Findings {
			rows = append(rows, findingRow{res, f})
		}
	}
	if len(rows) == 0 {
		return
	}
	sort.SliceStable(rows, func(i, j int) bool {
		return severityRank(rows[i].f.Severity) < severityRank(rows[j].f.Severity)
	})

	var wShort, wID, wRegion int
	for _, row := range rows {
		wShort = max(wShort, len([]rune(row.res.Short)))
		wID = max(wID, len([]rune(row.res.ID)))
		wRegion = max(wRegion, len([]rune(row.res.Region)))
	}
	fmt.Fprint(w, "\nFINDINGS\n")
	for _, row := range rows {
		fmt.Fprintf(w, "%-8s%-*s  %-*s  %-*s  %s\n", strings.ToUpper(string(row.f.Severity)),
			wShort, row.res.Short, wID, row.res.ID, wRegion, row.res.Region, row.f.Message)
		if row.f.Recommendation != "" {
			fmt.Fprintf(w, "        -> %s\n", row.f.Recommendation)
		}
	}
}

func severityRank(s model.Severity) int {
	switch s {
	case model.SeverityHigh:
		return 0
	case model.SeverityMedium:
		return 1
	}
	return 2
}

func resourceLabel(r model.Resource) string {
	if r.Name != "" && r.Name != r.ID {
		return r.ID + " (" + r.Name + ")"
	}
	return r.ID
}

func costLabel(c model.Cost) string {
	if c.MonthlyUSD == nil {
		return "-"
	}
	switch c.Source {
	case model.CostEstimate:
		return money(*c.MonthlyUSD) + " (est.)"
	case model.CostActual:
		return money(*c.MonthlyUSD) + " (actual)"
	}
	return "-"
}

// truncate cuts s to maxCell characters, ending in an ellipsis when cut.
func truncate(s string) string {
	s = strings.NewReplacer("\n", " ", "\r", " ", "\t", " ").Replace(s)
	rs := []rune(s)
	if len(rs) <= maxCell {
		return s
	}
	return string(rs[:maxCell-1]) + "…"
}

func plural(n int, word string) string {
	if n == 1 {
		return fmt.Sprintf("1 %s", word)
	}
	return fmt.Sprintf("%d %ss", n, word)
}

// money formats v as dollars with two decimals and thousands separators.
func money(v float64) string {
	s := strconv.FormatFloat(v, 'f', 2, 64)
	neg := strings.HasPrefix(s, "-")
	s = strings.TrimPrefix(s, "-")
	dot := strings.IndexByte(s, '.')
	intPart, frac := s[:dot], s[dot:]
	var b strings.Builder
	for i, c := range intPart {
		if i > 0 && (len(intPart)-i)%3 == 0 {
			b.WriteByte(',')
		}
		b.WriteRune(c)
	}
	out := "$" + b.String() + frac
	if neg {
		out = "-" + out
	}
	return out
}
