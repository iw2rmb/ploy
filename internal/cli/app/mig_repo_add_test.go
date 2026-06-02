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

func TestMigRepoAddResolvesRunStyleSelector(t *testing.T) {
	migID := domaintypes.NewMigID().String()
	var capturedResolve map[string]string
	var capturedAdd map[string]string

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/v1/migs":
			if got := r.URL.Query().Get("name_substring"); got != "my-wave" {
				t.Fatalf("name_substring = %q, want my-wave", got)
			}
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
		case r.Method == http.MethodPost && r.URL.Path == "/v1/migs/"+migID+"/repos":
			if err := json.NewDecoder(r.Body).Decode(&capturedAdd); err != nil {
				t.Fatalf("decode repo add request: %v", err)
			}
			w.WriteHeader(http.StatusCreated)
			_ = json.NewEncoder(w).Encode(map[string]any{
				"id":         "repo-001",
				"mig_id":     migID,
				"repo_url":   capturedAdd["repo_url"],
				"base_ref":   capturedAdd["base_ref"],
				"created_at": time.Now().UTC(),
			})
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	clienv.UseServerDescriptor(t, server.URL)

	var buf bytes.Buffer
	if err := executeCmd([]string{"mig", "repo", "add", "my-wave", "acme/service:feature/test"}, &buf); err != nil {
		t.Fatalf("mig repo add error: %v", err)
	}

	if capturedResolve["selector"] != "acme/service" || capturedResolve["ref"] != "feature/test" {
		t.Fatalf("unexpected resolve request: %#v", capturedResolve)
	}
	if capturedAdd["repo_url"] != "https://gitlab.example.com/acme/service.git" || capturedAdd["base_ref"] != "feature/test" {
		t.Fatalf("unexpected repo add request: %#v", capturedAdd)
	}
	if !strings.Contains(buf.String(), "Repo added: gitlab.example.com/acme/service") {
		t.Fatalf("unexpected output: %q", buf.String())
	}
}
