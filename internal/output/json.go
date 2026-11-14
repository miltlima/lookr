// Package output renders a scan report as JSON, CSV or a text table.
package output

import (
	"encoding/json"
	"io"

	"lookr/internal/model"
)

// WriteJSON writes the report as indented JSON followed by a newline.
func WriteJSON(w io.Writer, r model.Report) error {
	enc := json.NewEncoder(w)
	enc.SetEscapeHTML(false)
	enc.SetIndent("", "  ")
	return enc.Encode(r)
}
