package runs

import (
	"encoding/json"
	"fmt"
	"io"
)

// RenderRunReportJSON renders the canonical run report payload as JSON.
func RenderRunReportJSON(w io.Writer, report RunReport) error {
	if w == nil {
		return fmt.Errorf("run report json: output writer required")
	}

	if err := json.NewEncoder(w).Encode(report); err != nil {
		return fmt.Errorf("run report json: encode report: %w", err)
	}

	return nil
}
