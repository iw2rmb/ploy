package common

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/iw2rmb/ploy/internal/testutil/clienv"
)

func TestResolveControlPlaneHTTP(t *testing.T) {
	tests := []struct {
		name        string
		serverURL   string
		authToken   string
		wantBaseURL string
		wantErr     string
	}{
		{name: "http url", serverURL: "http://127.0.0.1:9094", wantBaseURL: "http://127.0.0.1:9094"},
		{name: "host defaults", serverURL: "control.example", wantBaseURL: "http://control.example:8080"},
		{name: "missing server url", wantErr: "PLOY_SERVER_URL is required"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv("PLOY_SERVER_URL", tt.serverURL)
			t.Setenv("PLOY_AUTH_TOKEN", tt.authToken)

			u, client, err := ResolveControlPlaneHTTP(context.TODO())
			if tt.wantErr != "" {
				if err == nil {
					t.Fatalf("expected error containing %q", tt.wantErr)
				}
				if got := err.Error(); got != tt.wantErr {
					t.Fatalf("error=%q want %q", got, tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("ResolveControlPlaneHTTP error: %v", err)
			}
			if got := u.String(); got != tt.wantBaseURL {
				t.Fatalf("base url=%s want %s", got, tt.wantBaseURL)
			}
			if client == nil {
				t.Fatalf("expected client, got nil")
			}
			if client.Timeout <= 0 {
				t.Fatalf("expected default Timeout to be set, got %v", client.Timeout)
			}
		})
	}
}

func TestResolveControlPlaneHTTP_AddsBearerToken(t *testing.T) {
	const token = "test-token"
	var gotAuth string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()
	clienv.UseControlPlaneEnvWithToken(t, srv.URL, token)

	_, client, err := ResolveControlPlaneHTTP(context.TODO())
	if err != nil {
		t.Fatalf("ResolveControlPlaneHTTP error: %v", err)
	}
	req, err := http.NewRequestWithContext(context.TODO(), http.MethodGet, srv.URL, nil)
	if err != nil {
		t.Fatalf("NewRequest: %v", err)
	}
	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("Do: %v", err)
	}
	_ = resp.Body.Close()
	if gotAuth != "Bearer "+token {
		t.Fatalf("Authorization=%q want bearer token", gotAuth)
	}
}

func TestCloneForStreamDisablesTimeout(t *testing.T) {
	c := &http.Client{Timeout: 5 * time.Second}
	clone := CloneForStream(c)
	if clone.Timeout != 0 {
		t.Fatalf("expected stream clone Timeout=0, got %v", clone.Timeout)
	}
	if clone == c {
		t.Fatal("expected a distinct client clone instance")
	}
}
