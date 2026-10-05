package handlers

import (
	"fmt"
	"net/http"
	"strings"
	"testing"

	domaintypes "github.com/iw2rmb/ploy/internal/domain/types"
	"github.com/iw2rmb/ploy/internal/gitauth"
	"github.com/iw2rmb/ploy/internal/server/gitlabtokens"
	"github.com/iw2rmb/ploy/internal/store"
)

func TestRunLaunchHandlers_RepoConflictReturnsCurrentRunID(t *testing.T) {
	conflict := &store.ActiveRepoRunError{
		RepoID: domaintypes.NewRepoID(), RepoURL: "https://github.com/org/repo", RunID: domaintypes.NewRunID(),
	}
	for _, route := range []string{"submit", "wave", "restart"} {
		t.Run(route, func(t *testing.T) {
			st := activeMigWithSpec(domaintypes.NewSpecID())
			wrapped := fmt.Errorf("materialize run: %w", conflict)
			st.createWaveWithRuns.err = wrapped
			st.restartRun.err = wrapped
			var handler http.HandlerFunc
			var body any
			var pathValues []string
			switch route {
			case "submit":
				handler = createSingleRepoRunHandler(st, nil, gitauth.Options{}, runSubmitSpecServices{})
				body = validRunRequestBody()
			case "wave":
				handler = createMigRunHandler(st, gitauth.Options{})
				body = allReposSelector()
				pathValues = []string{"mig_id", "mig123"}
			case "restart":
				runID := domaintypes.NewRunID()
				st.getRun.val = store.Run{ID: runID, SpecID: *st.getMig.val.SpecID, Attempt: 1}
				handler = restartRunHandler(st, gitauth.Options{}, gitlabtokens.NewRegistry())
				pathValues = []string{"run_id", runID.String()}
			}
			rr := doRequest(t, handler, http.MethodPost, "/", body, pathValues...)
			// Every launch entry point preserves the conflict's current Run ID.
			assertStatus(t, rr, http.StatusConflict)
			if message := strings.TrimSpace(rr.Body.String()); message != conflict.Error() {
				t.Fatalf("error=%q, want %q", message, conflict.Error())
			}
		})
	}
}
