package client

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
	"time"
)

// Token listing preserves usernames and distinguishes forbidden access from other failures.
func TestListTokensCommand(t *testing.T) {
	for _, tc := range []struct {
		name      string
		status    int
		body      string
		wantErr   bool
		forbidden bool
	}{
		{name: "admin", status: http.StatusOK, body: `{"tokens":[{"token_id":"one","role":"control-plane","username":"alice","issued_at":"2026-01-01T00:00:00Z","expires_at":"2027-01-01T00:00:00Z"}]}`},
		{name: "forbidden", status: http.StatusForbidden, body: `{"error":"forbidden"}`, wantErr: true, forbidden: true},
		{name: "unauthenticated", status: http.StatusUnauthorized, body: `{"error":"unauthorized"}`, wantErr: true},
		{name: "server failure", status: http.StatusServiceUnavailable, body: `{"error":"unavailable"}`, wantErr: true},
		{name: "malformed response", status: http.StatusOK, body: `{`, wantErr: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method != http.MethodGet || r.URL.Path != "/api/v1/tokens" {
					t.Errorf("unexpected request: %s %s", r.Method, r.URL)
				}
				w.WriteHeader(tc.status)
				fmt.Fprint(w, tc.body)
			}))
			defer server.Close()
			base, _ := url.Parse(server.URL + "/api")
			result, err := (ListTokensCommand{Client: server.Client(), BaseURL: base}).Run(t.Context())
			if (err != nil) != tc.wantErr || errors.Is(err, ErrTokenListForbidden) != tc.forbidden {
				t.Fatalf("error = %v", err)
			}
			if !tc.wantErr && (len(result.Tokens) != 1 || result.Tokens[0].Username == nil || *result.Tokens[0].Username != "alice") {
				t.Fatalf("tokens = %+v", result.Tokens)
			}
		})
	}
}

// Session cancellation stops capability discovery while its HTTP request is blocked.
func TestListTokensCommandCancellation(t *testing.T) {
	started, stopped, release := make(chan struct{}), make(chan struct{}), make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		close(started)
		select {
		case <-r.Context().Done():
			close(stopped)
		case <-release:
		}
	}))
	defer server.Close()
	defer close(release)
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	base, _ := url.Parse(server.URL)
	done := make(chan error, 1)
	go func() {
		_, err := (ListTokensCommand{Client: server.Client(), BaseURL: base}).Run(ctx)
		done <- err
	}()
	select {
	case <-started:
	case <-time.After(3 * time.Second):
		t.Fatal("request did not start")
	}
	cancel()
	select {
	case err := <-done:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("error = %v", err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("request did not stop")
	}
	select {
	case <-stopped:
	case <-time.After(3 * time.Second):
		t.Fatal("server did not observe cancellation")
	}
}
