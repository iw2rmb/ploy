package handlers

import (
	"bytes"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgtype"

	domaintypes "github.com/iw2rmb/ploy/internal/domain/types"
	"github.com/iw2rmb/ploy/internal/gitauth"
	"github.com/iw2rmb/ploy/internal/gitlabtoken"
	"github.com/iw2rmb/ploy/internal/server/gitlabtokens"
	"github.com/iw2rmb/ploy/internal/store"
)

const testRunSHASeed = "0123456789abcdef0123456789abcdef01234567"

func TestCancelRunHandlerV1TokenRelease(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name             string
		status           domaintypes.RunStatus
		cancelErr        error
		wantStatus       int
		wantCancelCalled bool
		wantToken        bool
	}{
		{
			name:             "active run releases token after successful cancel",
			status:           domaintypes.RunStatusRunning,
			wantStatus:       http.StatusOK,
			wantCancelCalled: true,
		},
		{
			name:       "terminal run releases token on idempotent cancel",
			status:     domaintypes.RunStatusCancelled,
			wantStatus: http.StatusOK,
		},
		{
			name:             "cancel error keeps token registered",
			status:           domaintypes.RunStatusRunning,
			cancelErr:        errors.New("db exploded"),
			wantStatus:       http.StatusInternalServerError,
			wantCancelCalled: true,
			wantToken:        true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			runID := domaintypes.NewRunID()
			hash := "hash-" + runID.String()
			token := "glpat-secret"
			st := &runStore{}
			st.getRun.val = store.Run{
				ID:        runID,
				MigID:     domaintypes.NewMigID(),
				SpecID:    domaintypes.NewSpecID(),
				Status:    tc.status,
				CreatedAt: pgtype.Timestamptz{Time: time.Now().UTC(), Valid: true},
			}
			st.cancelRun.err = tc.cancelErr
			registry := gitlabtokens.NewRegistry()
			registry.Register(hash, token, []domaintypes.RunID{runID})

			req := httptest.NewRequest(http.MethodPost, "/v1/runs/"+runID.String()+"/cancel", nil)
			req.SetPathValue("run_id", runID.String())
			rr := httptest.NewRecorder()

			cancelRunHandlerV1(st, registry).ServeHTTP(rr, req)

			assertStatus(t, rr, tc.wantStatus)
			if st.cancelRun.called != tc.wantCancelCalled {
				t.Fatalf("CancelRun called = %v, want %v", st.cancelRun.called, tc.wantCancelCalled)
			}
			if tc.wantCancelCalled && st.cancelRun.params != runID.String() {
				t.Fatalf("CancelRun run id = %q, want %q", st.cancelRun.params, runID)
			}
			_, gotToken := registry.Token(hash)
			if gotToken != tc.wantToken {
				t.Fatalf("registry token present = %v, want %v", gotToken, tc.wantToken)
			}
		})
	}
}

func TestRestartRunHandler(t *testing.T) {
	t.Parallel()

	runID := domaintypes.NewRunID()
	token := "glpat-restart-secret"
	tokenHash := gitlabtoken.Hash(token)
	tokenStats := mustRunStatsWithMarkerForRestartTest(t, tokenHash)

	tests := []struct {
		name       string
		setup      func(*runStore)
		body       any
		wantStatus int
		wantToken  bool
		verify     func(*testing.T, *runStore, *gitlabtokens.Registry)
	}{
		{
			name: "bodyless success",
			setup: func(st *runStore) {
				st.restartRun.val = store.Run{
					ID:        runID,
					MigID:     domaintypes.NewMigID(),
					SpecID:    domaintypes.NewSpecID(),
					Status:    domaintypes.RunStatusQueued,
					Attempt:   2,
					CreatedAt: pgtype.Timestamptz{Time: time.Now().UTC(), Valid: true},
				}
			},
			wantStatus: http.StatusOK,
			verify: func(t *testing.T, st *runStore, registry *gitlabtokens.Registry) {
				t.Helper()
				if !st.restartRun.called {
					t.Fatal("expected RestartRun to be called")
				}
				if st.restartRun.params.RunID != runID {
					t.Fatalf("RestartRun id=%q, want %q", st.restartRun.params.RunID, runID)
				}
				if st.restartRun.params.Stats != nil {
					t.Fatalf("RestartRun stats=%s, want nil", string(st.restartRun.params.Stats))
				}
			},
		},
		{
			name: "token success stores marker stats and keeps registry entry",
			setup: func(st *runStore) {
				st.getRun.val = store.Run{
					ID:        runID,
					RepoID:    "repo1",
					MigID:     domaintypes.NewMigID(),
					SpecID:    domaintypes.NewSpecID(),
					Status:    domaintypes.RunStatusFail,
					CreatedAt: pgtype.Timestamptz{Time: time.Now().UTC(), Valid: true},
				}
				st.repoByID = map[domaintypes.RepoID]store.Repo{
					"repo1": {ID: "repo1", Url: "https://gitlab.example.com/acme/service"},
				}
				st.restartRun.val = store.Run{
					ID:        runID,
					MigID:     domaintypes.NewMigID(),
					SpecID:    domaintypes.NewSpecID(),
					Status:    domaintypes.RunStatusQueued,
					Attempt:   2,
					Stats:     tokenStats,
					CreatedAt: pgtype.Timestamptz{Time: time.Now().UTC(), Valid: true},
				}
			},
			body:       map[string]any{"gitlab_token": token},
			wantStatus: http.StatusOK,
			wantToken:  true,
			verify: func(t *testing.T, st *runStore, registry *gitlabtokens.Registry) {
				t.Helper()
				if got := gitlabtoken.HashFromRunStats(st.restartRun.params.Stats); got != tokenHash {
					t.Fatalf("RestartRun stats marker=%q, want %q", got, tokenHash)
				}
			},
		},
		{
			name: "token restart error releases registry entry",
			setup: func(st *runStore) {
				st.getRun.val = store.Run{ID: runID, RepoID: "repo1"}
				st.repoByID = map[domaintypes.RepoID]store.Repo{
					"repo1": {ID: "repo1", Url: "https://gitlab.example.com/acme/service"},
				}
				st.restartRun.err = errors.New("database connection failed")
			},
			body:       map[string]any{"gitlab_token": token},
			wantStatus: http.StatusInternalServerError,
		},
		{
			name: "active run conflict",
			setup: func(st *runStore) {
				st.restartRun.err = store.ErrRunRestartActive
			},
			wantStatus: http.StatusConflict,
		},
		{
			name: "cancelled wave conflict",
			setup: func(st *runStore) {
				st.restartRun.err = store.ErrRunRestartWaveCancelled
			},
			wantStatus: http.StatusConflict,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			st := &runStore{}
			tt.setup(st)
			registry := gitlabtokens.NewRegistry()

			var body *bytes.Reader
			if tt.body == nil {
				body = bytes.NewReader(nil)
			} else {
				payload, err := json.Marshal(tt.body)
				if err != nil {
					t.Fatalf("marshal request: %v", err)
				}
				body = bytes.NewReader(payload)
			}
			req := httptest.NewRequest(http.MethodPost, "/v1/runs/"+runID.String()+"/restart", body)
			req.SetPathValue("run_id", runID.String())
			rr := httptest.NewRecorder()

			restartRunHandler(st, gitauth.Options{GitLabDomain: "gitlab.example.com"}, registry).ServeHTTP(rr, req)

			assertStatus(t, rr, tt.wantStatus)
			_, gotToken := registry.Token(tokenHash)
			if gotToken != tt.wantToken {
				t.Fatalf("registry token present = %v, want %v", gotToken, tt.wantToken)
			}
			if tt.verify != nil {
				tt.verify(t, st, registry)
			}
		})
	}
}

func mustRunStatsWithMarkerForRestartTest(t *testing.T, hash string) []byte {
	t.Helper()
	stats, err := gitlabtoken.RunStatsWithMarker(hash)
	if err != nil {
		t.Fatalf("RunStatsWithMarker() failed: %v", err)
	}
	return stats
}
