package runs

import (
	"encoding/json"
	"fmt"
	"io"
)

// RenderRunStatusReportJSON renders the canonical run status report payload as JSON.
func RenderRunStatusReportJSON(w io.Writer, report RunStatusReport) error {
	if w == nil {
		return fmt.Errorf("run status report json: output writer required")
	}

	if err := json.NewEncoder(w).Encode(report); err != nil {
		return fmt.Errorf("run status report json: encode report: %w", err)
	}

	return nil
}
