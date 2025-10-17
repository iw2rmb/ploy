package grid

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	workflowsdk "github.com/iw2rmb/grid/sdk/workflowrpc/go"
)

func TestClientCollectEvidence(t *testing.T) {
	runID := "run-123"
	logTail := "workflow stdout line\nworkflow stderr line\n"
	now := time.Now().UTC().Format(time.RFC3339Nano)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasSuffix(r.URL.Path, "/logs"):
			_, _ = w.Write([]byte(logTail))
		case strings.HasSuffix(r.URL.Path, "/events"):
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"events":[{"type":"state","time":"` + now + `","job":{"state":"succeeded","exit_code":0,"reason":"","terminal_log":""}}]}`))
		default:
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"state":"succeeded","exit_code":0}`))
		}
	}))
	t.Cleanup(server.Close)

	client := &Client{
		controlHTTP: func(context.Context) (*http.Client, error) {
			return server.Client(), nil
		},
		controlStatus: func() ControlPlaneStatus {
			return ControlPlaneStatus{APIEndpoint: server.URL}
		},
		logTail: 500,
	}

	term := terminalRun{
		status:   workflowsdk.RunStatusSucceeded,
		metadata: map[string]string{"stage": "mods-plan"},
		result:   map[string]any{"archive_export_id": "abc"},
	}

	evidence := client.collectEvidence(context.Background(), runID, term)
	if evidence == nil {
		t.Fatal("expected evidence to be collected")
	}
	if evidence.JobState != "succeeded" {
		t.Fatalf("expected job state succeeded, got %q", evidence.JobState)
	}
	if evidence.ExitCode == nil || *evidence.ExitCode != 0 {
		t.Fatalf("expected exit code 0, got %#v", evidence.ExitCode)
	}
	if evidence.LogTail != logTail {
		t.Fatalf("expected log tail %q, got %q", logTail, evidence.LogTail)
	}
	if len(evidence.Events) != 1 {
		t.Fatalf("expected single event, got %d", len(evidence.Events))
	}
	if evidence.Metadata["stage"] != "mods-plan" {
		t.Fatalf("expected metadata propagated, got %#v", evidence.Metadata)
	}
	if evidence.Result["archive_export_id"] != "abc" {
		t.Fatalf("expected result propagated, got %#v", evidence.Result)
	}
}
