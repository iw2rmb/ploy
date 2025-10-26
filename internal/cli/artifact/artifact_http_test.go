package artifact_test

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	artifactcli "github.com/iw2rmb/ploy/internal/cli/artifact"
	"github.com/iw2rmb/ploy/internal/workflow/artifacts"
)

func TestControlPlaneServiceAddParsesResponse(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/v1/artifacts/upload" {
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
		body := mustReadAll(t, r.Body)
		if string(body) != "payload-bytes" {
			t.Fatalf("unexpected payload %s", string(body))
		}
		_, _ = w.Write([]byte(`{"artifact":{"id":"artifact-1","cid":"bafy","digest":"sha256:test","name":"artifact.log","size":14}}`))
	}))
	defer server.Close()

	svc := newTestService(t, server)
	resp, err := svc.Add(context.Background(), artifacts.AddRequest{Name: "artifact.log", Payload: []byte("payload-bytes")})
	if err != nil {
		t.Fatalf("Add: %v", err)
	}
	if resp.CID != "bafy" || resp.Digest != "sha256:test" {
		t.Fatalf("unexpected response: %#v", resp)
	}
}

func TestControlPlaneServiceStatusIncludesPinFields(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/v1/artifacts":
			_, _ = w.Write([]byte(`{"artifacts":[{"id":"artifact-2","cid":"bafy2","digest":"sha256:z","name":"artifact.bin","pin_state":"pinning","pin_replicas":2,"pin_retry_count":3,"pin_error":"cluster timeout","pin_next_attempt_at":"2025-10-26T16:00:00Z"}]}`))
		default:
			t.Fatalf("unexpected request %s %s", r.Method, r.URL.Path)
		}
	}))
	defer server.Close()

	svc := newTestService(t, server)
	status, err := svc.Status(context.Background(), "bafy2")
	if err != nil {
		t.Fatalf("Status: %v", err)
	}
	if status.PinState != "pinning" || status.PinReplicas != 2 || status.PinRetryCount != 3 {
		t.Fatalf("unexpected status: %#v", status)
	}
	if status.PinError != "cluster timeout" {
		t.Fatalf("expected pin error")
	}
}

func TestControlPlaneServiceFetchDownloadsData(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/v1/artifacts":
			_, _ = w.Write([]byte(`{"artifacts":[{"id":"artifact-3","cid":"bafy3","digest":"","name":"art.bin"}]}`))
		case r.Method == http.MethodGet && strings.HasPrefix(r.URL.Path, "/v1/artifacts/artifact-3"):
			_, _ = w.Write([]byte("downloaded"))
		default:
			t.Fatalf("unexpected request %s %s", r.Method, r.URL.Path)
		}
	}))
	defer server.Close()

	svc := newTestService(t, server)
	result, err := svc.Fetch(context.Background(), "bafy3")
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if string(result.Data) != "downloaded" {
		t.Fatalf("unexpected data %q", string(result.Data))
	}
}

func newTestService(t *testing.T, server *httptest.Server) artifactcli.Service {
	t.Helper()
	base, err := url.Parse(server.URL)
	if err != nil {
		t.Fatalf("parse url: %v", err)
	}
	svc, err := artifactcli.NewControlPlaneService(base, server.Client())
	if err != nil {
		t.Fatalf("new service: %v", err)
	}
	return svc
}

func mustReadAll(t *testing.T, body io.ReadCloser) []byte {
	t.Helper()
	data, err := io.ReadAll(body)
	if err != nil {
		t.Fatalf("read body: %v", err)
	}
	return data
}
