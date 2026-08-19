package types

import (
	"encoding/json"
	"testing"
)

func TestRunSummaryJSONRejectsEmptyIDs(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		json string
	}{
		{"empty mig ID", `{
			"id": "2NQPoBfVkc8dFmGAQqJnUwMu9jR",
			"status": "Queued",
			"mig_id": "",
			"spec_id": "spec-y2Z",
			"created_at": "2024-01-01T00:00:00Z"
		}`},
		{"empty spec ID", `{
			"id": "2NQPoBfVkc8dFmGAQqJnUwMu9jR",
			"status": "Queued",
			"mig_id": "mig-x1",
			"spec_id": "",
			"created_at": "2024-01-01T00:00:00Z"
		}`},
		{"whitespace mig ID", `{
			"id": "2NQPoBfVkc8dFmGAQqJnUwMu9jR",
			"status": "Queued",
			"mig_id": "   ",
			"spec_id": "spec-y2Z",
			"created_at": "2024-01-01T00:00:00Z"
		}`},
		{"whitespace spec ID", `{
			"id": "2NQPoBfVkc8dFmGAQqJnUwMu9jR",
			"status": "Queued",
			"mig_id": "mig-x1",
			"spec_id": "   ",
			"created_at": "2024-01-01T00:00:00Z"
		}`},
		{"empty run ID", `{
			"id": "",
			"status": "Queued",
			"mig_id": "mig-x1",
			"spec_id": "spec-y2",
			"created_at": "2024-01-01T00:00:00Z"
		}`},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			var summary RunSummary
			if err := json.Unmarshal([]byte(tt.json), &summary); err == nil {
				t.Fatal("json.Unmarshal() error = nil, want ID validation error")
			}
		})
	}
}
