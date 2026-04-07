package main

import (
	"bytes"
	"encoding/json"
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

func TestRerunWithoutAlterSendsEmptyAlterObject(t *testing.T) {
	sourceJobID := domaintypes.NewJobID()
	runID := domaintypes.NewRunID()
	repoID := domaintypes.NewRepoID()
	rootJobID := domaintypes.NewJobID()

	type rerunPayload struct {
		Alter map[string]any `json:"alter"`
	}
	var gotPayload rerunPayload

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost && r.URL.Path == "/v1/jobs/"+sourceJobID.String()+"/rerun" {
			if err := json.NewDecoder(r.Body).Decode(&gotPayload); err != nil {
				t.Fatalf("decode rerun payload: %v", err)
			}
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusCreated)
			_, _ = w.Write([]byte(`{"run_id":"` + runID.String() + `","repo_id":"` + repoID.String() + `","attempt":3,"root_job_id":"` + rootJobID.String() + `","copied_from_job_id":"` + sourceJobID.String() + `"}`))
			return
		}
		http.NotFound(w, r)
	}))
	defer server.Close()
	clienv.UseServerDescriptor(t, server.URL)

	var buf bytes.Buffer
	err := executeCmd([]string{"rerun", "--job", sourceJobID.String()}, &buf)
	if err != nil {
		t.Fatalf("rerun error: %v", err)
	}
	if gotPayload.Alter == nil {
		t.Fatal("expected alter object in request, got nil")
	}
	if len(gotPayload.Alter) != 0 {
		t.Fatalf("expected empty alter object, got %#v", gotPayload.Alter)
	}
}

func TestRerunCompilesAlterInLocalPathToHashAndBundleMap(t *testing.T) {
	sourceJobID := domaintypes.NewJobID()
	runID := domaintypes.NewRunID()
	repoID := domaintypes.NewRepoID()
	rootJobID := domaintypes.NewJobID()

	type rerunPayload struct {
		Alter map[string]any `json:"alter"`
	}
	var gotPayload rerunPayload
	var rerunCalled bool

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodHead && r.URL.Path == "/v1/spec-bundles":
			w.WriteHeader(http.StatusNotFound)
			return
		case r.Method == http.MethodPost && r.URL.Path == "/v1/spec-bundles":
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusCreated)
			_, _ = w.Write([]byte(`{"bundle_id":"bundle_123","cid":"bafytestcid","digest":"sha256:abc","size":12,"deduplicated":false}`))
			return
		case r.Method == http.MethodPost && r.URL.Path == "/v1/jobs/"+sourceJobID.String()+"/rerun":
			rerunCalled = true
			if err := json.NewDecoder(r.Body).Decode(&gotPayload); err != nil {
				t.Fatalf("decode rerun payload: %v", err)
			}
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusCreated)
			_, _ = w.Write([]byte(`{"run_id":"` + runID.String() + `","repo_id":"` + repoID.String() + `","attempt":3,"root_job_id":"` + rootJobID.String() + `","copied_from_job_id":"` + sourceJobID.String() + `"}`))
			return
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	clienv.UseServerDescriptor(t, server.URL)

	tmp := t.TempDir()
	src := filepath.Join(tmp, "gate_profile.schema.json")
	if err := os.WriteFile(src, []byte(`{"type":"object"}`), 0o644); err != nil {
		t.Fatalf("write src file: %v", err)
	}
	alterPath := filepath.Join(tmp, "alter.yaml")
	if err := os.WriteFile(alterPath, []byte("in:\n  - "+src+":gate_profile.schema.json\n"), 0o644); err != nil {
		t.Fatalf("write alter file: %v", err)
	}

	var buf bytes.Buffer
	err := executeCmd([]string{"rerun", "--job", sourceJobID.String(), "--alter", alterPath}, &buf)
	if err != nil {
		t.Fatalf("rerun error: %v", err)
	}
	if !rerunCalled {
		t.Fatal("expected rerun endpoint to be called")
	}

	inRaw, ok := gotPayload.Alter["in"].([]any)
	if !ok || len(inRaw) != 1 {
		t.Fatalf("alter.in=%T/%v want []any len=1", gotPayload.Alter["in"], gotPayload.Alter["in"])
	}
	inEntry, ok := inRaw[0].(string)
	if !ok {
		t.Fatalf("alter.in[0] type=%T want string", inRaw[0])
	}
	idx := strings.Index(inEntry, ":")
	if idx <= 0 {
		t.Fatalf("alter.in[0]=%q want shortHash:/in/...", inEntry)
	}
	hash := inEntry[:idx]
	dst := inEntry[idx+1:]
	if !shortHashPattern.MatchString(hash) {
		t.Fatalf("alter.in hash=%q is not canonical short hash", hash)
	}
	if dst != "/in/gate_profile.schema.json" {
		t.Fatalf("alter.in dst=%q want /in/gate_profile.schema.json", dst)
	}

	bundleMap, ok := gotPayload.Alter["bundle_map"].(map[string]any)
	if !ok {
		t.Fatalf("alter.bundle_map=%T want map[string]any", gotPayload.Alter["bundle_map"])
	}
	if got := bundleMap[hash]; got != "bundle_123" {
		t.Fatalf("alter.bundle_map[%q]=%v want bundle_123", hash, got)
	}
}
