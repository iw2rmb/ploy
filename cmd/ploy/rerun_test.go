package main

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	domaintypes "github.com/iw2rmb/ploy/internal/domain/types"
	"github.com/iw2rmb/ploy/internal/testutil/clienv"
)

func TestRerunCallsControlPlane(t *testing.T) {
	sourceJobID := domaintypes.NewJobID()
	runID := domaintypes.NewRunID()
	repoID := domaintypes.NewRepoID()
	rootJobID := domaintypes.NewJobID()

	var called bool
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost && r.URL.Path == "/v1/jobs/"+sourceJobID.String()+"/rerun" {
			called = true
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusCreated)
			_, _ = w.Write([]byte(`{"run_id":"` + runID.String() + `","repo_id":"` + repoID.String() + `","attempt":3,"root_job_id":"` + rootJobID.String() + `","copied_from_job_id":"` + sourceJobID.String() + `"}`))
			return
		}
		http.NotFound(w, r)
	}))
	defer server.Close()
	clienv.UseServerDescriptor(t, server.URL)

	tmp := t.TempDir()
	alterPath := filepath.Join(tmp, "alter.yaml")
	if err := os.WriteFile(alterPath, []byte("image: docker.io/test/heal:debug\nenvs:\n  DEBUG: \"1\"\nin:\n  - abc1234:/in/build-log.txt\n"), 0o644); err != nil {
		t.Fatalf("write alter file: %v", err)
	}

	var buf bytes.Buffer
	err := executeCmd([]string{"rerun", "--job", sourceJobID.String(), "--alter", alterPath}, &buf)
	if err != nil {
		t.Fatalf("rerun error: %v", err)
	}
	if !called {
		t.Fatal("expected rerun endpoint to be called")
	}
	out := buf.String()
	if !strings.Contains(out, rootJobID.String()) {
		t.Fatalf("expected output to include root job id, got %q", out)
	}
}

func TestRerunRequiresFlags(t *testing.T) {
	var buf bytes.Buffer
	err := executeCmd([]string{"rerun"}, &buf)
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "--job is required") {
		t.Fatalf("unexpected error: %v", err)
	}
}
