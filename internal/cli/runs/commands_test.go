package runs

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
)

func TestInspectCommand(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/v1/mods/j1", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{"run_id": "j1", "status": "failed"})
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()
	base, _ := url.Parse(srv.URL)

	var out bytes.Buffer
	if err := (InspectCommand{Client: srv.Client(), BaseURL: base, JobID: "j1", Output: &out}).Run(context.Background()); err != nil {
		t.Fatalf("inspect err=%v", err)
	}
	if out.Len() == 0 {
		t.Fatalf("expected inspect output")
	}
}
