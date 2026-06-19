package app

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	domaintypes "github.com/iw2rmb/ploy/internal/domain/types"
	"github.com/iw2rmb/ploy/internal/testutil/clienv"
)

// TestRunListCallsControlPlane validates ls command calls the API.
// Not parallel because useServerDescriptor uses t.Setenv.
func TestRunListCallsControlPlane(t *testing.T) {
	t.Setenv("USER", "test-user")
	var called bool

	runID1 := domaintypes.NewRunID().String()
	runID2 := domaintypes.NewRunID().String()
	migID1 := domaintypes.NewMigID().String()
	migID2 := domaintypes.NewMigID().String()
	specID1 := domaintypes.NewSpecID().String()
	specID2 := domaintypes.NewSpecID().String()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet && strings.HasPrefix(r.URL.Path, "/v1/runs") {
			called = true

			// Check pagination params.
			limit := r.URL.Query().Get("limit")
			offset := r.URL.Query().Get("offset")
			if limit != "10" {
				t.Errorf("expected limit=10, got %s", limit)
			}
			if offset != "5" {
				t.Errorf("expected offset=5, got %s", offset)
			}
			if got := r.URL.Query().Get("created_by"); got != "test-user" {
				t.Errorf("expected created_by=test-user, got %s", got)
			}
			if got := r.URL.Query().Get("all"); got != "" {
				t.Errorf("expected all to be omitted, got %s", got)
			}

			now := time.Now()
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"runs": []map[string]any{
					{
						"id":                 runID1,
						"status":             "Running",
						"mig_id":             migID1,
						"spec_id":            specID1,
						"repo_url":           "https://gitlab.example.com/team/service.git",
						"source_commit_sha":  "0123456789abcdef0123456789abcdef01234567",
						"spec_name":          "upgrade-java",
						"spec_source_domain": "gitlab.example.com",
						"spec_source_repo":   "team/specs",
						"created_at":         now,
					},
					{
						"id":         runID2,
						"status":     "Success",
						"mig_id":     migID2,
						"spec_id":    specID2,
						"created_at": now,
					},
				},
			})
			return
		}
		http.NotFound(w, r)
	}))
	defer server.Close()

	clienv.UseControlPlaneEnv(t, server.URL)

	var buf bytes.Buffer
	err := executeCmd([]string{"run", "ls", "--limit", "10", "--offset", "5"}, &buf)
	if err != nil {
		t.Fatalf("run ls error: %v", err)
	}
	if !called {
		t.Fatal("expected GET /v1/runs to be called")
	}

	output := buf.String()
	if !strings.Contains(output, runID1) {
		t.Errorf("output should contain %s: %s", runID1, output)
	}
	if !strings.Contains(output, "Running") {
		t.Errorf("output should contain Started: %s", output)
	}
	if strings.Contains(output, "MIG") || strings.Contains(output, "MOD") {
		t.Errorf("output should not contain removed MIG/MOD columns, got: %s", output)
	}
	if !strings.Contains(output, "ID") || !strings.Contains(output, "STATUS") || !strings.Contains(output, "SPEC") || !strings.Contains(output, "REPO") {
		t.Errorf("output should contain ID STATUS SPEC REPO columns, got: %s", output)
	}
	if strings.Contains(output, "DERIVED STATUS") {
		t.Errorf("output should not contain derived status column: %s", output)
	}
	if !strings.Contains(output, "REPO") || !strings.Contains(output, "team/service:01234567") {
		t.Errorf("output should contain repo label: %s", output)
	}
	if !strings.Contains(output, "gitlab.example.com/team/specs:upgrade-java") {
		t.Errorf("output should contain named spec label: %s", output)
	}
}

// TestRunListEmptyResult validates ls command handles empty results.
// Not parallel because useServerDescriptor uses t.Setenv.
func TestRunListEmptyResult(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet && strings.HasPrefix(r.URL.Path, "/v1/runs") {
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{"runs": []any{}})
			return
		}
		http.NotFound(w, r)
	}))
	defer server.Close()

	clienv.UseControlPlaneEnv(t, server.URL)

	var buf bytes.Buffer
	err := executeCmd([]string{"run", "ls"}, &buf)
	if err != nil {
		t.Fatalf("run ls error: %v", err)
	}

	output := buf.String()
	if !strings.Contains(output, "No runs found") {
		t.Errorf("output should contain 'No runs found': %s", output)
	}
}

// TestRunListInvalidLimit validates ls command rejects invalid limit.
// This uses t.Parallel since it does not use t.Setenv.
func TestRunListInvalidLimit(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		args    []string
		wantErr string
	}{
		{
			name:    "limit below minimum",
			args:    []string{"run", "ls", "--limit", "0"},
			wantErr: "limit must be between 1 and 100",
		},
		{
			name:    "limit above maximum",
			args:    []string{"run", "ls", "--limit", "101"},
			wantErr: "limit must be between 1 and 100",
		},
		{
			name:    "negative offset",
			args:    []string{"run", "ls", "--offset", "-1"},
			wantErr: "offset must be non-negative",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			clienv.RunExpectError(t, executeCmd, tc.args, tc.wantErr)
		})
	}
}

func TestMigRunRepoSelectorResolvesToExplicitRepoURL(t *testing.T) {
	migID := domaintypes.NewMigID().String()
	waveID := domaintypes.NewWaveID().String()
	specID := domaintypes.NewSpecID().String()
	var capturedResolve map[string]string
	var capturedWave struct {
		RepoSelector struct {
			Mode  string   `json:"mode"`
			Repos []string `json:"repos,omitempty"`
		} `json:"repo_selector"`
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/v1/migs":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"migs": []map[string]any{{
					"id":         migID,
					"name":       "my-wave",
					"created_at": time.Now().UTC(),
					"archived":   false,
				}},
			})
		case r.Method == http.MethodPost && r.URL.Path == "/v1/repos/resolve":
			if err := json.NewDecoder(r.Body).Decode(&capturedResolve); err != nil {
				t.Fatalf("decode resolve request: %v", err)
			}
			_ = json.NewEncoder(w).Encode(map[string]any{
				"repo_url":   "https://gitlab.example.com/acme/service.git",
				"ref":        "feature/test",
				"ref_is_sha": false,
			})
		case r.Method == http.MethodPost && r.URL.Path == "/v1/migs/"+migID+"/waves":
			if err := json.NewDecoder(r.Body).Decode(&capturedWave); err != nil {
				t.Fatalf("decode wave request: %v", err)
			}
			w.WriteHeader(http.StatusCreated)
			_ = json.NewEncoder(w).Encode(map[string]any{
				"wave_id":   waveID,
				"mig_id":    migID,
				"spec_id":   specID,
				"run_count": 1,
			})
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	clienv.UseControlPlaneEnv(t, server.URL)

	var buf bytes.Buffer
	if err := executeCmd([]string{"mig", "run", "my-wave", "acme/service:feature/test"}, &buf); err != nil {
		t.Fatalf("mig run error: %v", err)
	}
	if capturedResolve["selector"] != "acme/service" || capturedResolve["ref"] != "feature/test" {
		t.Fatalf("unexpected resolve request: %#v", capturedResolve)
	}
	if capturedWave.RepoSelector.Mode != "explicit" {
		t.Fatalf("repo selector mode = %q", capturedWave.RepoSelector.Mode)
	}
	if len(capturedWave.RepoSelector.Repos) != 1 || capturedWave.RepoSelector.Repos[0] != "https://gitlab.example.com/acme/service.git" {
		t.Fatalf("unexpected explicit repos: %#v", capturedWave.RepoSelector.Repos)
	}
	if !strings.Contains(buf.String(), waveID) {
		t.Fatalf("expected wave id in output, got %q", buf.String())
	}
}

// TestMigRunStatusNotFound validates run status command handles 404.
// Not parallel because useServerDescriptor uses t.Setenv.
func TestMigRunStatusNotFound(t *testing.T) {
	runID := domaintypes.NewRunID().String()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet && r.URL.Path == "/v1/runs/"+runID {
			http.Error(w, "run not found", http.StatusNotFound)
			return
		}
		http.NotFound(w, r)
	}))
	defer server.Close()

	clienv.UseControlPlaneEnv(t, server.URL)

	var buf bytes.Buffer
	err := executeCmd([]string{"run", "status", runID}, &buf)
	if err == nil {
		t.Fatal("expected error for 404 response")
	}
	if !strings.Contains(err.Error(), "not found") && !strings.Contains(err.Error(), "404") {
		t.Errorf("error should mention not found or 404: %v", err)
	}
}
