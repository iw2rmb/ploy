package snapshots

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestIPFSGatewayPublisherUploadsArtifacts(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Fatalf("expected POST, got %s", r.Method)
		}
		if r.URL.Path != "/api/v0/add" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		if r.URL.Query().Get("pin") != "true" {
			t.Fatalf("expected pin=true query param")
		}
		if err := r.ParseMultipartForm(2 << 20); err != nil {
			t.Fatalf("parse multipart form: %v", err)
		}
		file, _, err := r.FormFile("file")
		if err != nil {
			t.Fatalf("read multipart file: %v", err)
		}
		defer func() { _ = file.Close() }()
		body, err := io.ReadAll(file)
		if err != nil {
			t.Fatalf("read file body: %v", err)
		}
		if string(body) != "artifact-data" {
			t.Fatalf("unexpected artifact body: %q", string(body))
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"Hash":"bafyreexamplecid","Name":"artifact","Size":"12"}`)
	}))
	defer server.Close()

	publisher, err := NewIPFSGatewayPublisher(server.URL, IPFSGatewayOptions{Pin: true})
	if err != nil {
		t.Fatalf("new IPFS gateway publisher: %v", err)
	}
	cid, err := publisher.Publish(context.Background(), []byte("artifact-data"))
	if err != nil {
		t.Fatalf("publish artifact: %v", err)
	}
	if cid != "bafyreexamplecid" {
		t.Fatalf("expected cid bafyreexamplecid, got %s", cid)
	}
}

func TestIPFSGatewayPublisherRejectsNon200Responses(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "boom", http.StatusBadGateway)
	}))
	defer server.Close()

	publisher, err := NewIPFSGatewayPublisher(server.URL, IPFSGatewayOptions{})
	if err != nil {
		t.Fatalf("new IPFS gateway publisher: %v", err)
	}
	_, err = publisher.Publish(context.Background(), []byte("artifact"))
	if err == nil || !strings.Contains(err.Error(), "unexpected status 502") {
		t.Fatalf("expected error for non-200 response, got %v", err)
	}
}

func TestNewIPFSGatewayPublisherValidatesEndpoint(t *testing.T) {
	if _, err := NewIPFSGatewayPublisher("", IPFSGatewayOptions{}); err == nil {
		t.Fatal("expected error for empty endpoint")
	}
	if _, err := NewIPFSGatewayPublisher("localhost:5001", IPFSGatewayOptions{}); err == nil {
		t.Fatal("expected error for endpoint missing scheme")
	}
}

func TestIPFSGatewayPublisherWithoutPinOmitsQuery(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("pin") != "" {
			t.Fatalf("expected pin query param to be omitted, got %q", r.URL.Query().Get("pin"))
		}
		_ = r.ParseMultipartForm(2 << 20)
		_, _ = io.ReadAll(r.Body)
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"Hash":"bafynominal","Name":"artifact","Size":"12"}`)
	}))
	defer server.Close()

	publisher, err := NewIPFSGatewayPublisher(server.URL, IPFSGatewayOptions{Pin: false})
	if err != nil {
		t.Fatalf("new IPFS gateway publisher: %v", err)
	}
	if _, err := publisher.Publish(context.Background(), []byte("data")); err != nil {
		t.Fatalf("unexpected publish error: %v", err)
	}
}

func TestIPFSGatewayPublisherRequiresPayload(t *testing.T) {
	publisher, err := NewIPFSGatewayPublisher("https://example.com", IPFSGatewayOptions{})
	if err != nil {
		t.Fatalf("new IPFS gateway publisher: %v", err)
	}
	if _, err := publisher.Publish(context.Background(), nil); err == nil || !strings.Contains(err.Error(), "artifact payload empty") {
		t.Fatalf("expected error for empty payload, got %v", err)
	}
}

func TestIPFSGatewayPublisherSurfacesMissingCID(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = r.ParseMultipartForm(2 << 20)
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"Name":"artifact","Size":"12"}`)
	}))
	defer server.Close()

	publisher, err := NewIPFSGatewayPublisher(server.URL, IPFSGatewayOptions{Pin: true})
	if err != nil {
		t.Fatalf("new IPFS gateway publisher: %v", err)
	}
	_, err = publisher.Publish(context.Background(), []byte("data"))
	if err == nil || !strings.Contains(err.Error(), "response missing cid") {
		t.Fatalf("expected error for missing CID, got %v", err)
	}
}

func TestMaskLast4Variants(t *testing.T) {
	if got := maskLast4(""); got != "last4-" {
		t.Fatalf("expected last4- for empty string, got %s", got)
	}
	if got := maskLast4("1234"); got != "last4-1234" {
		t.Fatalf("expected last4-1234, got %s", got)
	}
	if got := maskLast4("  12345678  "); got != "last4-5678" {
		t.Fatalf("expected last4-5678, got %s", got)
	}
}

func TestApplySyntheticRejectsUnknownStrategy(t *testing.T) {
	data := dataset{"users": {row{"id": "1"}}}
	err := applySynthetic(data, []SyntheticRule{{Table: "users", Column: "token", Strategy: "unknown"}}, DiffSummary{SyntheticColumns: map[string][]string{}})
	if err == nil || !strings.Contains(err.Error(), "synthetic strategy") {
		t.Fatalf("expected error for unknown synthetic strategy, got %v", err)
	}
}

func TestIPFSGatewayPublisherUsesBackgroundWhenContextNil(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = r.ParseMultipartForm(2 << 20)
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"Hash":"bafyctx","Name":"artifact"}`)
	}))
	defer server.Close()

	publisher, err := NewIPFSGatewayPublisher(server.URL, IPFSGatewayOptions{Pin: true})
	if err != nil {
		t.Fatalf("new IPFS gateway publisher: %v", err)
	}
	cid, err := publisher.Publish(nil, []byte("data")) //nolint:staticcheck // exercise nil context branch
	if err != nil || cid != "bafyctx" {
		t.Fatalf("expected cid bafyctx, got cid=%s err=%v", cid, err)
	}
}

func TestExtractCIDSupportsCidField(t *testing.T) {
	cid, err := extractCID([]byte(`{"Cid":"bafyCid"}`))
	if err != nil {
		t.Fatalf("extract cid: %v", err)
	}
	if cid != "bafyCid" {
		t.Fatalf("expected bafyCid, got %s", cid)
	}
}

func TestExtractCIDReadsFirstValidLine(t *testing.T) {
	cid, err := extractCID([]byte("\n{\"Name\":\"artifact\"}\n{\"Hash\":\"bafyline\"}\n"))
	if err != nil {
		t.Fatalf("extract cid: %v", err)
	}
	if cid != "bafyline" {
		t.Fatalf("expected bafyline, got %s", cid)
	}
}

func TestExtractCIDReportsEmptyPayload(t *testing.T) {
	if _, err := extractCID([]byte("   \n\t")); err == nil || !strings.Contains(err.Error(), "<empty>") {
		t.Fatalf("expected error noting empty payload, got %v", err)
	}
}
