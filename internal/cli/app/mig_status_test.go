package app

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	domaintypes "github.com/iw2rmb/ploy/internal/domain/types"
	"github.com/iw2rmb/ploy/internal/testutil/assertx"
	"github.com/iw2rmb/ploy/internal/testutil/clienv"
)

func TestMigStatusPrintsMigrationSummary(t *testing.T) {
	migID := domaintypes.MigID("mig001")
	specID := domaintypes.NewSpecID()
	repoID1 := domaintypes.NewMigRepoID()
	repoID2 := domaintypes.NewMigRepoID()
	runID1 := domaintypes.NewRunID()
	runID2 := domaintypes.NewRunID()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/v1/migs":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"migs": []map[string]any{
					{
						"id":         migID.String(),
						"name":       "java17-upgrade",
						"spec_id":    specID.String(),
						"archived":   false,
						"created_at": "2026-02-24T07:30:00Z",
					},
				},
			})
		case r.Method == http.MethodGet && r.URL.Path == "/v1/migs/"+migID.String()+"/repos":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"repos": []map[string]any{
					{
						"id":         repoID1.String(),
						"mig_id":     migID.String(),
						"repo_url":   "https://github.com/acme/service-a.git",
						"base_ref":   "main",
						"target_ref": "ploy/java17-a",
						"created_at": "2026-02-24T07:31:00Z",
					},
					{
						"id":         repoID2.String(),
						"mig_id":     migID.String(),
						"repo_url":   "https://github.com/acme/service-b.git",
						"base_ref":   "main",
						"target_ref": "ploy/java17-b",
						"created_at": "2026-02-24T07:32:00Z",
					},
				},
			})
		case r.Method == http.MethodGet && r.URL.Path == "/v1/runs":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"runs": []map[string]any{
					{
						"id":         runID1.String(),
						"status":     "Running",
						"mig_id":     migID.String(),
						"spec_id":    specID.String(),
						"created_at": "2026-02-24T08:00:00Z",
						"repo_counts": map[string]any{
							"total":          2,
							"queued":         0,
							"running":        1,
							"success":        1,
							"fail":           0,
							"cancelled":      0,
							"derived_status": "running",
						},
					},
					{
						"id":         runID2.String(),
						"status":     "Success",
						"mig_id":     migID.String(),
						"spec_id":    specID.String(),
						"created_at": "2026-02-24T08:10:00Z",
						"repo_counts": map[string]any{
							"total":          2,
							"queued":         0,
							"running":        0,
							"success":        2,
							"fail":           0,
							"cancelled":      0,
							"derived_status": "success",
						},
					},
				},
			})
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	clienv.UseServerDescriptor(t, server.URL)

	var buf bytes.Buffer
	err := executeCmd([]string{"mig", "status", migID.String()}, &buf)
	if err != nil {
		t.Fatalf("mig status error: %v", err)
	}

	out := buf.String()
	assertx.Contains(t, out, "Mig:   "+migID.String()+"  | java17-upgrade")
	assertx.Contains(t, out, "Spec:  "+specID.String()+" | Download")
	assertx.Contains(t, out, "Repos: 2")
	assertx.Contains(t, out, "Run")
	assertx.Contains(t, out, "Success")
	assertx.Contains(t, out, "Fail")
	assertx.Contains(t, out, "⣽  "+runID1.String())
	assertx.Contains(t, out, "✓  "+runID2.String())
}
