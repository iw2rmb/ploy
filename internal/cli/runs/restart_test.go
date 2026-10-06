package runs

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	domainapi "github.com/iw2rmb/ploy/internal/domain/api"
	"github.com/iw2rmb/ploy/internal/domain/types"
)

// The new option shares the existing restart endpoint and remains opt-in.
func TestRestartCommand_FromFailed(t *testing.T) {
	for _, fromFailed := range []bool{false, true} {
		t.Run(map[bool]string{false: "full", true: "from failed"}[fromFailed], func(t *testing.T) {
			id := types.NewRunID()
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method != "POST" || r.URL.Path != "/v1/runs/"+id.String()+"/restart" {
					t.Errorf("request: %s %s", r.Method, r.URL.Path)
				}
				var req domainapi.RunRestartRequest
				if fromFailed {
					if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
						t.Error(err)
					}
				}
				if req.FromFailed != fromFailed {
					t.Errorf("from_failed=%v", req.FromFailed)
				}
				if err := json.NewEncoder(w).Encode(types.RunSummary{ID: id, Attempt: 1, Status: types.RunStatusRunning, MigID: types.NewMigID(), SpecID: types.NewSpecID()}); err != nil {
					t.Error(err)
				}
			}))
			defer server.Close()
			base, _ := url.Parse(server.URL)
			if _, err := (RestartCommand{Client: server.Client(), BaseURL: base, RunID: id, FromFailed: fromFailed}).Run(context.Background()); err != nil {
				t.Fatal(err)
			}
		})
	}
}
