package mods

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	domaintypes "github.com/iw2rmb/ploy/internal/domain/types"
	modsapi "github.com/iw2rmb/ploy/internal/mods/api"
)

// TestInspectCommand_Run validates InspectCommand output including job graph display.
func TestInspectCommand_Run(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name           string
		run            modsapi.RunSummary
		wantSubstrings []string
		wantMissing    []string
	}{
		{
			name: "basic run with MR and gate",
			run: modsapi.RunSummary{
				RunID: domaintypes.RunID("mods-abc123"),
				State: modsapi.RunStateRunning,
				Metadata: map[string]string{
					"mr_url":       "https://gitlab.com/org/repo/-/merge_requests/42",
					"gate_summary": "failed pre-gate duration=567ms",
				},
				CreatedAt: time.Now(),
				UpdatedAt: time.Now(),
			},
			wantSubstrings: []string{
				"Run mods-abc123: running",
				"MR: https://gitlab.com/org/repo/-/merge_requests/42",
				"Gate: failed pre-gate duration=567ms",
			},
		},
		{
			name: "run with job graph",
			run: modsapi.RunSummary{
				RunID: domaintypes.RunID("mods-def456"),
				State: modsapi.RunStateSucceeded,
				Metadata: map[string]string{
					"gate_summary": "passed duration=1234ms",
				},
				Stages: map[string]modsapi.StageStatus{
					"11111111-1111-1111-1111-111111111111": {
						State:     modsapi.StageStateSucceeded,
						StepIndex: 1000,
					},
					"22222222-2222-2222-2222-222222222222": {
						State:     modsapi.StageStateSucceeded,
						StepIndex: 2000,
					},
					"33333333-3333-3333-3333-333333333333": {
						State:     modsapi.StageStateRunning,
						StepIndex: 3000,
					},
				},
				CreatedAt: time.Now(),
				UpdatedAt: time.Now(),
			},
			wantSubstrings: []string{
				"Run mods-def456: succeeded",
				"Gate: passed duration=1234ms",
				"Jobs:",
				"[1000] 11111111: succeeded",
				"[2000] 22222222: succeeded",
				"[3000] 33333333: running",
			},
		},
		{
			name: "run with healing jobs at midpoint indices",
			run: modsapi.RunSummary{
				RunID: domaintypes.RunID("mods-heal789"),
				State: modsapi.RunStateRunning,
				Metadata: map[string]string{
					"gate_summary": "failed pre-gate duration=200ms",
				},
				Stages: map[string]modsapi.StageStatus{
					"aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa": {
						State:     modsapi.StageStateFailed,
						StepIndex: 1000, // pre-gate failed
					},
					"bbbbbbbb-bbbb-bbbb-bbbb-bbbbbbbbbbbb": {
						State:     modsapi.StageStateSucceeded,
						StepIndex: 1500, // heal inserted at midpoint
					},
					"cccccccc-cccc-cccc-cccc-cccccccccccc": {
						State:     modsapi.StageStateRunning,
						StepIndex: 1750, // re-gate
					},
				},
				CreatedAt: time.Now(),
				UpdatedAt: time.Now(),
			},
			wantSubstrings: []string{
				"Run mods-heal789: running",
				"Jobs:",
				"[1000] aaaaaaaa: failed",
				"[1500] bbbbbbbb: succeeded",
				"[1750] cccccccc: running",
			},
		},
		{
			name: "run without MR or gate",
			run: modsapi.RunSummary{
				RunID:     domaintypes.RunID("mods-minimal"),
				State:     modsapi.RunStatePending,
				CreatedAt: time.Now(),
				UpdatedAt: time.Now(),
			},
			wantSubstrings: []string{
				"Run mods-minimal: pending",
			},
			wantMissing: []string{
				"MR:",
				"Gate:",
				"Jobs:",
			},
		},
		{
			name: "run with empty stages map",
			run: modsapi.RunSummary{
				RunID:     domaintypes.RunID("mods-empty"),
				State:     modsapi.RunStateRunning,
				Stages:    map[string]modsapi.StageStatus{},
				CreatedAt: time.Now(),
				UpdatedAt: time.Now(),
			},
			wantSubstrings: []string{
				"Run mods-empty: running",
			},
			wantMissing: []string{
				"Jobs:",
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			// Set up mock server returning the test run (RunSummary directly, no wrapper).
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				// Return RunSummary directly — the canonical response shape.
				_ = json.NewEncoder(w).Encode(tc.run)
			}))
			t.Cleanup(srv.Close)

			baseURL, err := url.Parse(srv.URL)
			if err != nil {
				t.Fatalf("parse server URL: %v", err)
			}

			var out bytes.Buffer
			cmd := InspectCommand{
				Client:  srv.Client(),
				BaseURL: baseURL,
				RunID:   tc.run.RunID,
				Output:  &out,
			}

			if err := cmd.Run(context.Background()); err != nil {
				t.Fatalf("Run() error: %v", err)
			}

			result := out.String()

			// Verify expected substrings are present.
			for _, want := range tc.wantSubstrings {
				if !strings.Contains(result, want) {
					t.Errorf("output missing %q\ngot:\n%s", want, result)
				}
			}

			// Verify unwanted substrings are absent.
			for _, notWant := range tc.wantMissing {
				if strings.Contains(result, notWant) {
					t.Errorf("output should not contain %q\ngot:\n%s", notWant, result)
				}
			}
		})
	}
}

// TestInspectCommand_Errors validates error handling for missing required fields.
func TestInspectCommand_Errors(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		cmd     InspectCommand
		wantErr string
	}{
		{
			name:    "missing client",
			cmd:     InspectCommand{RunID: "test"},
			wantErr: "http client required",
		},
		{
			name:    "missing base URL",
			cmd:     InspectCommand{Client: http.DefaultClient, RunID: "test"},
			wantErr: "base url required",
		},
		{
			name: "missing run id",
			cmd: InspectCommand{
				Client:  http.DefaultClient,
				BaseURL: &url.URL{Scheme: "http", Host: "localhost"},
			},
			wantErr: "run id required",
		},
		{
			name: "empty run id after trim",
			cmd: InspectCommand{
				Client:  http.DefaultClient,
				BaseURL: &url.URL{Scheme: "http", Host: "localhost"},
				RunID:   "   ",
			},
			wantErr: "run id required",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			err := tc.cmd.Run(context.Background())
			if err == nil {
				t.Fatal("expected error, got nil")
			}
			if !strings.Contains(err.Error(), tc.wantErr) {
				t.Errorf("error %q should contain %q", err.Error(), tc.wantErr)
			}
		})
	}
}

// TestInspectCommand_HTTPError validates handling of non-200 responses.
func TestInspectCommand_HTTPError(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte("run not found"))
	}))
	t.Cleanup(srv.Close)

	baseURL, _ := url.Parse(srv.URL)

	var out bytes.Buffer
	cmd := InspectCommand{
		Client:  srv.Client(),
		BaseURL: baseURL,
		RunID:   "mods-unknown",
		Output:  &out,
	}

	err := cmd.Run(context.Background())
	if err == nil {
		t.Fatal("expected error for 404 response")
	}
	if !strings.Contains(err.Error(), "run not found") {
		t.Errorf("error should contain server message: %v", err)
	}
}

// TestInspectCommand_JobGraphSorting ensures jobs are output in step_index order.
func TestInspectCommand_JobGraphSorting(t *testing.T) {
	t.Parallel()

	// Jobs with out-of-order step indices to verify sorting.
	run := modsapi.RunSummary{
		RunID: domaintypes.RunID("mods-sort"),
		State: modsapi.RunStateSucceeded,
		Stages: map[string]modsapi.StageStatus{
			"zzzzzzzz-zzzz-zzzz-zzzz-zzzzzzzzzzzz": {State: modsapi.StageStateSucceeded, StepIndex: 3000},
			"aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa": {State: modsapi.StageStateSucceeded, StepIndex: 1000},
			"mmmmmmmm-mmmm-mmmm-mmmm-mmmmmmmmmmmm": {State: modsapi.StageStateSucceeded, StepIndex: 2000},
		},
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		// Return RunSummary directly — the canonical response shape.
		_ = json.NewEncoder(w).Encode(run)
	}))
	t.Cleanup(srv.Close)

	baseURL, _ := url.Parse(srv.URL)

	var out bytes.Buffer
	cmd := InspectCommand{
		Client:  srv.Client(),
		BaseURL: baseURL,
		RunID:   "mods-sort",
		Output:  &out,
	}

	if err := cmd.Run(context.Background()); err != nil {
		t.Fatalf("Run() error: %v", err)
	}

	result := out.String()

	// Find positions of each job line in output.
	pos1000 := strings.Index(result, "[1000] aaaaaaaa")
	pos2000 := strings.Index(result, "[2000] mmmmmmmm")
	pos3000 := strings.Index(result, "[3000] zzzzzzzz")

	if pos1000 == -1 || pos2000 == -1 || pos3000 == -1 {
		t.Fatalf("missing job entries in output:\n%s", result)
	}

	// Verify order: 1000 < 2000 < 3000.
	if pos1000 >= pos2000 || pos2000 >= pos3000 {
		t.Errorf("jobs not sorted by step_index:\n%s", result)
	}
}
