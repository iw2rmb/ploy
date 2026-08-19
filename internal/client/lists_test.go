package client

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	domainapi "github.com/iw2rmb/ploy/internal/domain/api"
	domaintypes "github.com/iw2rmb/ploy/internal/domain/types"
)

func TestListMigsCommand(t *testing.T) {
	t.Parallel()

	archived := true
	name := "java"
	repo := "https://example.com/repo.git"
	migID := domaintypes.NewMigID()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Path != "/api/v1/migs" {
			t.Fatalf("request = %s %s, want GET /api/v1/migs", r.Method, r.URL.Path)
		}
		wantQuery := map[string]string{
			"limit": "10", "offset": "5", "name_substring": name,
			"archived": "true", "repo_url": repo,
		}
		for key, want := range wantQuery {
			if got := r.URL.Query().Get(key); got != want {
				t.Errorf("%s = %q, want %q", key, got, want)
			}
		}
		_ = json.NewEncoder(w).Encode(domainapi.MigListResponse{Migs: []domainapi.MigSummary{{ID: migID, Name: "java17"}}})
	}))
	t.Cleanup(srv.Close)
	baseURL, _ := url.Parse(srv.URL + "/api")

	got, err := (ListMigsCommand{
		Client: srv.Client(), BaseURL: baseURL, Limit: 10, Offset: 5,
		NameSubstring: &name, Archived: &archived, RepoURL: &repo,
	}).Run(context.Background())
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if len(got) != 1 || got[0].ID != migID {
		t.Fatalf("Run() = %+v, want mig %s", got, migID)
	}
}

func TestListRunsCommand(t *testing.T) {
	t.Parallel()

	runID := domaintypes.NewRunID()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Path != "/api/v1/runs" {
			t.Fatalf("request = %s %s, want GET /api/v1/runs", r.Method, r.URL.Path)
		}
		wantQuery := map[string]string{
			"limit": "20", "offset": "40", "repo_url": "https://example.com/repo.git",
			"created_by": "alice", "all": "true",
		}
		for key, want := range wantQuery {
			if got := r.URL.Query().Get(key); got != want {
				t.Errorf("%s = %q, want %q", key, got, want)
			}
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"runs": []domaintypes.RunSummary{{
			ID: runID, Status: domaintypes.RunStatusQueued,
			MigID: domaintypes.NewMigID(), SpecID: domaintypes.NewSpecID(),
		}}})
	}))
	t.Cleanup(srv.Close)
	baseURL, _ := url.Parse(srv.URL + "/api")

	got, err := (ListRunsCommand{
		Client: srv.Client(), BaseURL: baseURL, Limit: 20, Offset: 40,
		RepoURL: " https://example.com/repo.git ", CreatedBy: " alice ", All: true,
	}).Run(context.Background())
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if len(got) != 1 || got[0].ID != runID {
		t.Fatalf("Run() = %+v, want run %s", got, runID)
	}
}

func TestListCommandsUseCanonicalErrors(t *testing.T) {
	t.Parallel()

	baseURL, _ := url.Parse("http://localhost")
	tests := []struct {
		name string
		run  func() error
		want string
	}{
		{
			name: "migs missing client",
			run: func() error {
				_, err := (ListMigsCommand{BaseURL: baseURL}).Run(context.Background())
				return err
			},
			want: "mig list: http client required",
		},
		{
			name: "runs missing base URL",
			run: func() error {
				_, err := (ListRunsCommand{Client: http.DefaultClient}).Run(context.Background())
				return err
			},
			want: "run list: base url required",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if err := tc.run(); err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("error = %v, want substring %q", err, tc.want)
			}
		})
	}
}

func TestListCommandsReturnHTTPError(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte(`{"error":"internal server error"}`))
	}))
	t.Cleanup(srv.Close)
	baseURL, _ := url.Parse(srv.URL)

	for name, run := range map[string]func() error{
		"migs": func() error {
			_, err := (ListMigsCommand{Client: srv.Client(), BaseURL: baseURL}).Run(context.Background())
			return err
		},
		"runs": func() error {
			_, err := (ListRunsCommand{Client: srv.Client(), BaseURL: baseURL}).Run(context.Background())
			return err
		},
	} {
		t.Run(name, func(t *testing.T) {
			if err := run(); err == nil || !strings.Contains(err.Error(), "internal server error") {
				t.Fatalf("error = %v, want server message", err)
			}
		})
	}
}
