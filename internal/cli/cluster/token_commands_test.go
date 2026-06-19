package cluster

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/iw2rmb/ploy/internal/testutil/clienv"
)

func TestTokenCreateRequiresUsernameForControlPlane(t *testing.T) {
	var stderr bytes.Buffer
	err := Handle([]string{"token", "create", "--role", "control-plane"}, &stderr)
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "--username is required") {
		t.Fatalf("error = %q", err.Error())
	}
}

func TestTokenCreateSendsUsername(t *testing.T) {
	var captured map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/v1/tokens" {
			http.NotFound(w, r)
			return
		}
		if err := json.NewDecoder(r.Body).Decode(&captured); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"token":      "token",
			"token_id":   "token-1",
			"role":       "control-plane",
			"username":   "alice",
			"expires_at": time.Now().UTC().Add(time.Hour),
			"warning":    "save it",
		})
	}))
	defer server.Close()
	clienv.UseControlPlaneEnv(t, server.URL)

	var stderr bytes.Buffer
	err := Handle([]string{"token", "create", "--role", "control-plane", "--username", "alice"}, &stderr)
	if err != nil {
		t.Fatalf("token create: %v", err)
	}
	if captured["username"] != "alice" {
		t.Fatalf("username = %v, want alice", captured["username"])
	}
}
