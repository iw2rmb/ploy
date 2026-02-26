package main

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	domaintypes "github.com/iw2rmb/ploy/internal/domain/types"
)

func TestMigStatusPrintsMigrationSummary(t *testing.T) {
	t.Helper()

	migID := domaintypes.NewMigID()
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

	useServerDescriptor(t, server.URL)

	var buf bytes.Buffer
	err := executeCmd([]string{"mig", "status", migID.String()}, &buf)
	if err != nil {
		t.Fatalf("mig status error: %v", err)
	}

	out := buf.String()
	assertContainsMig(t, out, "Mig:   "+migID.String()+"  | java17-upgrade")
	assertContainsMig(t, out, "Spec:  "+specID.String()+" | Download")
	assertContainsMig(t, out, "Repos: 2")
	assertContainsMig(t, out, "Run")
	assertContainsMig(t, out, "Success")
	assertContainsMig(t, out, "Fail")
	assertContainsMig(t, out, "⣽  "+runID1.String())
	assertContainsMig(t, out, "✓  "+runID2.String())
}

func TestMigStatusRequiresMigID(t *testing.T) {
	t.Helper()
	useServerDescriptor(t, "http://example.test")

	var buf bytes.Buffer
	err := executeCmd([]string{"mig", "status"}, &buf)
	if err == nil {
		t.Fatal("expected error when mig id is missing")
	}
	if !strings.Contains(err.Error(), "mig id required") {
		t.Fatalf("expected mig id required error, got %v", err)
	}
	if !strings.Contains(buf.String(), "Usage: ploy mig status <mig-id>") {
		t.Fatalf("expected usage output, got %q", buf.String())
	}
}

func assertContainsMig(t *testing.T, output string, want string) {
	t.Helper()
	if !strings.Contains(output, want) {
		t.Fatalf("expected output to contain %q, got %q", want, output)
	}
}
