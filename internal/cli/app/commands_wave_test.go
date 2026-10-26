package app

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	domainapi "github.com/iw2rmb/ploy/internal/domain/api"
	domaintypes "github.com/iw2rmb/ploy/internal/domain/types"
	"github.com/iw2rmb/ploy/internal/testutil/clienv"
)

func TestWaveCommandsPreserveJSONOutput(t *testing.T) {
	createdBy, finishedAt := "alice", "Mon, 26 Oct 2026 09:00:00 GMT"
	for _, tc := range []struct {
		name   string
		args   []string
		status domaintypes.WaveStatus
		full   bool
	}{
		{"status", []string{"status"}, domaintypes.WaveStatusFinished, true},
		{"status cancelled", []string{"status"}, domaintypes.WaveStatusCancelled, false},
		{"follow finished", []string{"status", "--follow"}, domaintypes.WaveStatusFinished, true},
		{"follow cancelled", []string{"status", "--follow"}, domaintypes.WaveStatusCancelled, false},
		{"cancel", []string{"cancel"}, domaintypes.WaveStatusCancelled, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			wave := domainapi.WaveSummary{
				ID: domaintypes.NewWaveID(), MigID: domaintypes.NewMigID(), SpecID: domaintypes.NewSpecID(),
				Status: tc.status, CreatedAt: "Mon, 26 Oct 2026 08:00:00 GMT",
			}
			if tc.full {
				wave.CreatedBy, wave.StartedAt, wave.FinishedAt = &createdBy, &wave.CreatedAt, &finishedAt
				wave.Counts = &domaintypes.RunCounts{Total: 2, Success: 1, Fail: 1}
			}
			var serverJSON bytes.Buffer
			_ = json.NewEncoder(&serverJSON).Encode(wave)
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				method, path := http.MethodGet, "/api/v1/waves/"+wave.ID.String()
				if tc.args[0] == "cancel" {
					method, path = http.MethodPost, path+"/cancel"
				}
				if r.Method != method || r.URL.Path != path || r.Header.Get("Authorization") != "Bearer test-token" {
					t.Errorf("unexpected request: %s %s, authorization %q", r.Method, r.URL.Path, r.Header.Get("Authorization"))
				}
				_, _ = w.Write(serverJSON.Bytes())
			}))
			defer server.Close()
			clienv.UseControlPlaneEnv(t, server.URL+"/api")
			t.Setenv("PLOY_AUTH_TOKEN", "test-token")
			args := append([]string{"wave"}, tc.args...)
			args = append(args, wave.ID.String())
			var output bytes.Buffer
			if err := executeCmd(args, &output); err != nil {
				t.Fatal(err)
			}
			if want := serverJSON.String() + "\n"; output.String() != want {
				t.Fatalf("output = %q, want %q", output.String(), want)
			}
		})
	}
}
