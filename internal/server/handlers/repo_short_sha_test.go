package handlers

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/iw2rmb/ploy/internal/gitauth"
)

func TestGitLabShortSHAResolvesNestedProjectCommit(t *testing.T) {
	const sha = "abcd1234abcd1234abcd1234abcd1234abcd1234"
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if want := "/api/v4/projects/team%2Fsub%2Frepo/repository/commits/abcd123?stats=false"; r.RequestURI != want {
			t.Errorf("request URI = %q, want %q", r.RequestURI, want)
		}
		if r.Header.Get("PRIVATE-TOKEN") != "run-token" {
			t.Error("missing GitLab token")
		}
		fmt.Fprintf(w, `{"id":%q}`, sha)
	}))
	defer server.Close()
	got, err := resolveGitLabShortSHA(t.Context(), server.URL+"/team/sub/repo.git", "ABCD123", gitauth.Options{
		GitLabDomain: server.URL, GitLabPAT: "run-token",
	})
	if err != nil || got != sha {
		t.Fatalf("resolved SHA = %q, error = %v", got, err)
	}
}

func TestGitLabShortSHARejectsInvalidResponses(t *testing.T) {
	for _, tc := range []struct {
		name   string
		status int
		body   string
		want   string
	}{
		{"unresolved prefix", 404, "", "could not resolve"},
		{"insufficient token scope", 403, "", "HTTP 403"},
		{"invalid JSON", 200, "<html>", "invalid GitLab commit response"},
		{"short response ID", 200, `{"id":"abcd1234"}`, "does not match"},
		{"different commit", 200, `{"id":"cccc1234abcd1234abcd1234abcd1234abcd1234"}`, "does not match"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(tc.status)
				fmt.Fprint(w, tc.body)
			}))
			defer server.Close()
			_, err := resolveGitLabShortSHA(t.Context(), server.URL+"/team/repo.git", "abcd1234", gitauth.Options{GitLabDomain: server.URL})
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("error = %v, want %q", err, tc.want)
			}
		})
	}
}

func TestGitLabShortSHARequestsStayOnConfiguredOrigin(t *testing.T) {
	other := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		t.Error("unexpected request to a different origin")
	}))
	defer other.Close()
	origin := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, other.URL, http.StatusFound)
	}))
	defer origin.Close()
	auth := gitauth.Options{GitLabDomain: origin.URL, GitLabPAT: "run-token"}
	for _, tc := range []struct {
		url  string
		want string
	}{{origin.URL, "HTTP 302"}, {other.URL, "configured GitLab origin"}} {
		_, err := resolveGitLabShortSHA(t.Context(), tc.url+"/team/repo.git", "abcd1234", auth)
		if err == nil || !strings.Contains(err.Error(), tc.want) {
			t.Errorf("lookup at %s: got %v, want %q", tc.url, err, tc.want)
		}
	}
}
