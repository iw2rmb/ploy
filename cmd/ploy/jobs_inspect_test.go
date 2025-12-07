package main

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestJobsInspectRequiresArgsAndPrintsSummary(t *testing.T) {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet && r.URL.Path == "/v1/mods/job-55" {
			_, _ = w.Write([]byte(`{"run_id":"job-55","status":"running","base_ref":"main","target_ref":"feat"}`))
			return
		}
		http.NotFound(w, r)
	}))
	defer server.Close()

	useServerDescriptor(t, server.URL)

	// Missing args
	if err := execute([]string{"runs", "inspect"}, &bytes.Buffer{}); err == nil {
		t.Fatal("expected error when args missing")
	}

	buf := &bytes.Buffer{}
	err := execute([]string{"runs", "inspect", "job-55"}, buf)
	if err != nil {
		t.Fatalf("runs inspect error: %v", err)
	}
	if !bytes.Contains(buf.Bytes(), []byte("job-55")) {
		t.Fatalf("expected job id in output, got %q", buf.String())
	}
}
