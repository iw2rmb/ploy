package mig

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	domainapi "github.com/iw2rmb/ploy/internal/domain/api"
	domaintypes "github.com/iw2rmb/ploy/internal/domain/types"
	"github.com/iw2rmb/ploy/internal/testutil/clienv"
)

func TestRunProjectFollowTerminalOutput(t *testing.T) {
	for _, status := range []domaintypes.WaveStatus{domaintypes.WaveStatusFinished, domaintypes.WaveStatusCancelled} {
		t.Run(string(status), func(t *testing.T) {
			migID, waveID, specID := domaintypes.NewMigID(), domaintypes.NewWaveID(), domaintypes.NewSpecID()
			server := migFollowServer(t, migID, waveID, specID, func(w http.ResponseWriter, r *http.Request) {
				_ = json.NewEncoder(w).Encode(domainapi.WaveSummary{
					ID: waveID, MigID: migID, SpecID: specID, Status: status,
					CreatedAt: "Mon, 26 Oct 2026 08:00:00 GMT", Counts: &domaintypes.RunCounts{Total: 1, Fail: 1},
				})
			})
			defer server.Close()
			clienv.UseControlPlaneEnv(t, server.URL)
			var output bytes.Buffer
			err := RunProject(context.Background(), RunOptions{MigRef: "test-mig", Follow: true, Output: &output})
			if status == domaintypes.WaveStatusFinished && err != nil {
				t.Fatal(err)
			}
			if status == domaintypes.WaveStatusCancelled && (err == nil || err.Error() != "mig wave ended in cancelled") {
				t.Fatalf("error = %v", err)
			}
			want := waveID.String() + "\nwave " + waveID.String() + " " + string(status) + "\n"
			if output.String() != want {
				t.Fatalf("output = %q, want %q", output.String(), want)
			}
		})
	}
}

func TestRunProjectFollowCapAndParentCancellation(t *testing.T) {
	for _, tc := range []struct {
		name           string
		blockRequest   bool
		cancelOnCap    bool
		cancelStatus   int
		parentCancel   bool
		parentDeadline bool
		wantError      string
	}{
		{name: "cap during wait"},
		{name: "cap during request", blockRequest: true},
		{name: "cancel on cap during wait", cancelOnCap: true, cancelStatus: http.StatusOK},
		{name: "cancel on cap during request", blockRequest: true, cancelOnCap: true, cancelStatus: http.StatusOK},
		{name: "failed cancellation", cancelOnCap: true, cancelStatus: http.StatusInternalServerError, wantError: "cancel rejected"},
		{name: "parent cancellation", blockRequest: true, cancelOnCap: true, parentCancel: true, wantError: "context canceled"},
		{name: "parent deadline", blockRequest: true, cancelOnCap: true, parentDeadline: true, wantError: "context deadline exceeded"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			capDuration := 50 * time.Millisecond
			if tc.parentDeadline {
				var deadlineCancel context.CancelFunc
				ctx, deadlineCancel = context.WithTimeout(ctx, 50*time.Millisecond)
				defer deadlineCancel()
				capDuration = time.Minute
			}
			migID, waveID, specID := domaintypes.NewMigID(), domaintypes.NewWaveID(), domaintypes.NewSpecID()
			var cancelCalls atomic.Int32
			server := migFollowServer(t, migID, waveID, specID, func(w http.ResponseWriter, r *http.Request) {
				if r.Method == http.MethodPost {
					cancelCalls.Add(1)
					if r.Context().Err() != nil {
						t.Error("cancel request context already expired")
					}
					if tc.cancelStatus != http.StatusOK {
						w.WriteHeader(http.StatusInternalServerError)
						_, _ = w.Write([]byte(`{"error":"cancel rejected"}`))
						return
					}
					_ = json.NewEncoder(w).Encode(domainapi.WaveSummary{ID: waveID, MigID: migID, SpecID: specID, Status: domaintypes.WaveStatusCancelled})
					return
				}
				if tc.parentCancel {
					cancel()
				}
				if tc.blockRequest {
					<-r.Context().Done()
					return
				}
				_ = json.NewEncoder(w).Encode(domainapi.WaveSummary{ID: waveID, MigID: migID, SpecID: specID, Status: domaintypes.WaveStatusStarted})
			})
			defer server.Close()
			clienv.UseControlPlaneEnv(t, server.URL)
			var output bytes.Buffer
			err := RunProject(ctx, RunOptions{MigRef: "test-mig", Follow: true, Cap: capDuration, CancelOnCap: tc.cancelOnCap, Output: &output})
			if tc.wantError == "" {
				if err != nil {
					t.Fatal(err)
				}
			} else if err == nil || !strings.Contains(err.Error(), tc.wantError) {
				t.Fatalf("error = %v, want %q", err, tc.wantError)
			}
			if tc.parentCancel && !errors.Is(err, context.Canceled) {
				t.Fatalf("error = %v, want parent cancellation", err)
			}
			if tc.parentDeadline && !errors.Is(err, context.DeadlineExceeded) {
				t.Fatalf("error = %v, want parent deadline", err)
			}
			wantCancels := int32(0)
			if tc.cancelOnCap && !tc.parentCancel && !tc.parentDeadline {
				wantCancels = 1
			}
			if cancelCalls.Load() != wantCancels {
				t.Fatalf("cancel requests = %d, want %d", cancelCalls.Load(), wantCancels)
			}
		})
	}
}

func migFollowServer(t *testing.T, migID domaintypes.MigID, waveID domaintypes.WaveID, specID domaintypes.SpecID, waveHandler http.HandlerFunc) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/v1/migs":
			_ = json.NewEncoder(w).Encode(domainapi.MigListResponse{Migs: []domainapi.MigSummary{{ID: migID, Name: "test-mig"}}})
		case "/v1/migs/" + migID.String() + "/waves":
			w.WriteHeader(http.StatusCreated)
			_ = json.NewEncoder(w).Encode(map[string]any{"wave_id": waveID, "mig_id": migID, "spec_id": specID, "run_count": 1})
		case "/v1/waves/" + waveID.String(), "/v1/waves/" + waveID.String() + "/cancel":
			waveHandler(w, r)
		default:
			t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
			http.NotFound(w, r)
		}
	}))
}
