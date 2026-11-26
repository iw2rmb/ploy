package handlers

import (
	"bytes"
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/iw2rmb/ploy/internal/gitauth"
	"github.com/iw2rmb/ploy/internal/testutil/fakegit"
)

func TestRunSubmissionResolvesRemoteTarget(t *testing.T) {
	const historical = "abcdef0123456789abcdef0123456789abcdef01"
	const tip = testSourceCommitSHA
	for _, tc := range []struct {
		name, ref, advertised, baseRef, sha string
		apiStatus                           int
		apiID                               string
	}{
		{name: "historical commit", ref: historical[:7], apiStatus: 200, apiID: historical, baseRef: historical, sha: historical},
		{name: "exact branch before tag", ref: "feature/login", advertised: historical + "\trefs/tags/feature/login\n" + tip + "\trefs/heads/feature/login", baseRef: "feature/login", sha: tip},
		{name: "branch starting with refs", ref: "refs/feature/login", advertised: tip + "\trefs/heads/refs/feature/login", baseRef: "refs/feature/login", sha: tip},
		{name: "hexadecimal branch", ref: historical[:7], advertised: tip + "\trefs/heads/" + historical[:7], baseRef: historical[:7], sha: tip},
		{name: "branch suffix", ref: "login", advertised: tip + "\trefs/heads/feature/login"},
		{name: "unknown prefix", ref: "fffffff", apiStatus: 404},
		{name: "ambiguous prefix", ref: "abcd", apiStatus: 400},
		{name: "denied lookup", ref: historical[:7], apiStatus: 403},
		{name: "different commit", ref: historical[:7], apiStatus: 200, apiID: tip},
	} {
		t.Run(tc.name, func(t *testing.T) {
			fakegit.Install(t, tc.advertised)
			api := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if tc.apiStatus == 0 {
					t.Errorf("unexpected API lookup %s", r.URL)
					http.Error(w, "unexpected", 500)
					return
				}
				w.WriteHeader(tc.apiStatus)
				fmt.Fprintf(w, `{"id":%q}`, tc.apiID)
			}))
			defer api.Close()
			auth := gitauth.Options{GitLabDomain: api.URL}
			st := &handlerStore{}
			handler := createSingleRepoRunHandler(st, nil, auth, runSubmitSpecServices{})
			body := validRunRequestBodyWith(map[string]any{"repo_url": "https://gitlab.example.com/team/repo.git", "ref": tc.ref})
			req := httptest.NewRequest(http.MethodPost, "/v1/runs", bytes.NewReader(asJSONBytes(t, body)))
			req = req.WithContext(withSourceCommitSHAResolver(req.Context(), func(ctx context.Context, repoURL, ref string) (resolvedSourceCommit, error) {
				if repoURL != "https://gitlab.example.com/team/repo.git" {
					t.Fatalf("unexpected repository URL %q", repoURL)
				}
				return resolveSourceCommitSHA(ctx, api.URL+"/team/repo.git", ref, auth)
			}))
			rr := httptest.NewRecorder()
			handler.ServeHTTP(rr, req)
			if tc.sha == "" {
				assertStatus(t, rr, http.StatusBadRequest)
				if st.createSpec.called || st.createMig.called || st.createMigRepo.called || st.createWaveWithRuns.called {
					t.Fatal("submission wrote rows after target resolution failed")
				}
				return
			}
			assertStatus(t, rr, http.StatusCreated)
			if got := st.createMigRepo.params.BaseRef; got != tc.baseRef {
				t.Fatalf("mig repo base_ref = %q, want %q", got, tc.baseRef)
			}
			if len(st.createRunParams) != 1 {
				t.Fatalf("created %d runs, want 1", len(st.createRunParams))
			}
			run := st.createRunParams[0]
			if run.RepoBaseRef != tc.baseRef || run.SourceCommitSha != tc.sha || run.RepoSha0 != tc.sha {
				t.Fatalf("base_ref=%q source_commit_sha=%q repo_sha0=%q, want %q %q %q",
					run.RepoBaseRef, run.SourceCommitSha, run.RepoSha0, tc.baseRef, tc.sha, tc.sha)
			}
		})
	}
}
