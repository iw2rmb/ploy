package gitlab

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestHTTPTokenRevokerBulkSuccess(t *testing.T) {
	t.Helper()

	var bulkCalled bool
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Fatalf("expected POST, got %s", r.Method)
		}
		if r.URL.Path != "/api/v4/personal_access_tokens/revoke" {
			t.Fatalf("unexpected path %s", r.URL.Path)
		}
		var payload struct {
			TokenIDs []string `json:"token_ids"`
		}
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Fatalf("decode bulk payload: %v", err)
		}
		bulkCalled = true
		if len(payload.TokenIDs) != 2 {
			t.Fatalf("expected two token ids, got %d", len(payload.TokenIDs))
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	revoker := NewHTTPTokenRevoker(server.URL, "admin-token", server.Client())
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	report := revoker.Revoke(ctx, "deploy", []RevocableToken{{ID: "1", NodeID: "node-a"}, {ID: "2", NodeID: "node-b"}})
	if !bulkCalled {
		t.Fatalf("expected bulk endpoint to be invoked")
	}
	if len(report.Failed) != 0 {
		t.Fatalf("expected no failures, got %+v", report.Failed)
	}
	if len(report.Revoked) != 2 {
		t.Fatalf("expected two revoked tokens, got %d", len(report.Revoked))
	}
}

func TestHTTPTokenRevokerFallbackPerToken(t *testing.T) {
	t.Helper()

	var singleCalls []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v4/personal_access_tokens/revoke":
			http.Error(w, "bulk error", http.StatusInternalServerError)
		default:
			singleCalls = append(singleCalls, r.URL.Path)
			w.WriteHeader(http.StatusNoContent)
		}
	}))
	defer server.Close()

	revoker := NewHTTPTokenRevoker(server.URL, "admin-token", server.Client())
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	report := revoker.Revoke(ctx, "deploy", []RevocableToken{{ID: "9", NodeID: "node-a"}, {ID: "10", NodeID: "node-b"}})
	if len(singleCalls) != 2 {
		t.Fatalf("expected two single revoke calls, got %d", len(singleCalls))
	}
	if len(report.Revoked) != 2 {
		t.Fatalf("expected both tokens revoked via fallback, got %+v", report.Revoked)
	}
}

func TestHTTPTokenRevokerRecordsFailures(t *testing.T) {
	t.Helper()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v4/personal_access_tokens/revoke":
			http.Error(w, "bulk error", http.StatusInternalServerError)
		default:
			http.Error(w, "not found", http.StatusNotFound)
		}
	}))
	defer server.Close()

	revoker := NewHTTPTokenRevoker(server.URL, "admin-token", server.Client())
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	token := RevocableToken{ID: "42", NodeID: "node-a"}
	report := revoker.Revoke(ctx, "deploy", []RevocableToken{token})
	if len(report.Revoked) != 0 {
		t.Fatalf("expected no revoked tokens, got %d", len(report.Revoked))
	}
	if len(report.Failed) != 1 {
		t.Fatalf("expected failure recorded, got %+v", report.Failed)
	}
	if report.Failed[0].Token.ID != "42" {
		t.Fatalf("unexpected failed token id %s", report.Failed[0].Token.ID)
	}
	if report.Failed[0].Err == nil {
		t.Fatalf("expected failure error populated")
	}
}
