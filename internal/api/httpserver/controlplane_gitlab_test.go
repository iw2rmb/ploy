package httpserver_test

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/iw2rmb/ploy/internal/api/httpserver"
	"github.com/iw2rmb/ploy/internal/config/gitlab"
	"github.com/iw2rmb/ploy/internal/controlplane/scheduler"
)

func TestServerGitLabConfig(t *testing.T) {
	t.Parallel()

	etcd, client := startTestEtcd(t)
	defer etcd.Close()
	defer func() { _ = client.Close() }()

	sched, err := scheduler.New(client, scheduler.Options{LeaseTTL: 3 * time.Second})
	if err != nil {
		t.Fatalf("new scheduler: %v", err)
	}
	defer func() { _ = sched.Close() }()

	server := httptest.NewServer(newTestControlPlaneHandler(t, httpserver.ControlPlaneOptions{
		Scheduler: sched,
		Etcd:      client,
	}))
	defer server.Close()

	status, _ := getJSONStatus(t, server.URL+"/v1/config/gitlab")
	if status != http.StatusNotFound {
		t.Fatalf("expected 404 for missing config, got %d", status)
	}

	createPayload := map[string]any{
		"revision": 0,
		"config": map[string]any{
			"api_base_url":     "https://gitlab.local/api/v4",
			"allowed_projects": []any{"acme/ploy"},
			"default_token":    map[string]any{"name": "default", "value": "glpat-secret", "scopes": []any{"api"}},
			"deploy_tokens": []any{
				map[string]any{"name": "deploy", "value": "glpat-deploy", "scopes": []any{"read_repository"}},
			},
			"branch_policies": []any{},
			"rbac":            map[string]any{"readers": []any{"ops"}, "updaters": []any{"ops", "release"}},
		},
	}

	putStatus, putResp := putJSONStatus(t, server.URL+"/v1/config/gitlab", createPayload)
	if putStatus != http.StatusOK {
		t.Fatalf("expected put status 200, got %d", putStatus)
	}
	revision := int64(putResp["revision"].(float64))
	if revision == 0 {
		t.Fatalf("expected non-zero revision after create")
	}

	getStatus, getResp := getJSONStatus(t, server.URL+"/v1/config/gitlab")
	if getStatus != http.StatusOK {
		t.Fatalf("expected get status 200, got %d", getStatus)
	}
	cfg, ok := getResp["config"].(map[string]any)
	if !ok {
		t.Fatalf("expected config object in get response")
	}
	defaultToken, _ := cfg["default_token"].(map[string]any)
	if defaultToken == nil {
		t.Fatalf("expected default_token in config response")
	}
	if value, _ := defaultToken["value"].(string); value != "***redacted***" {
		t.Fatalf("expected default token to be masked, got %q", value)
	}

	updatePayload := map[string]any{
		"revision": revision,
		"config": map[string]any{
			"api_base_url":     "https://gitlab.local/api/v4",
			"allowed_projects": []any{"acme/ploy", "acme/api"},
			"default_token":    map[string]any{"name": "default", "value": "glpat-secret", "scopes": []any{"api", "read_repository"}},
			"deploy_tokens": []any{
				map[string]any{"name": "deploy", "value": "glpat-deploy", "scopes": []any{"read_repository"}},
			},
			"branch_policies": []any{
				map[string]any{"pattern": "main", "protected": true, "require_approvals": 1},
			},
			"rbac": map[string]any{
				"readers":  []any{"ops"},
				"updaters": []any{"ops", "release"},
			},
		},
	}

	updateStatus, updateResp := putJSONStatus(t, server.URL+"/v1/config/gitlab", updatePayload)
	if updateStatus != http.StatusOK {
		t.Fatalf("expected update status 200, got %d", updateStatus)
	}
	newRevision := int64(updateResp["revision"].(float64))
	if newRevision == revision || newRevision == 0 {
		t.Fatalf("expected new revision different from previous")
	}

	stalePayload := map[string]any{
		"revision": revision,
		"config":   updatePayload["config"],
	}
	staleStatus, staleResp := putJSONStatus(t, server.URL+"/v1/config/gitlab", stalePayload)
	if staleStatus != http.StatusConflict {
		t.Fatalf("expected conflict status, got %d", staleStatus)
	}
	if message, _ := staleResp["error"].(string); !strings.Contains(message, "revision mismatch") {
		t.Fatalf("expected revision mismatch error, got %q", message)
	}
}

func TestServerGitLabSignerEndpoints(t *testing.T) {
	t.Parallel()

	etcd, client := startTestEtcd(t)
	defer etcd.Close()
	defer func() {
		_ = client.Close()
	}()

	sched, err := scheduler.New(client, scheduler.Options{LeaseTTL: 3 * time.Second})
	if err != nil {
		t.Fatalf("new scheduler: %v", err)
	}
	defer func() {
		_ = sched.Close()
	}()

	key := strings.Repeat("l", 32)
	cipher, err := gitlab.NewAESCipher([]byte(key))
	if err != nil {
		t.Fatalf("new cipher: %v", err)
	}
	signer, err := gitlab.NewSigner(client, cipher)
	if err != nil {
		t.Fatalf("new signer: %v", err)
	}
	defer func() {
		_ = signer.Close()
	}()

	server := httptest.NewServer(newTestControlPlaneHandler(t, httpserver.ControlPlaneOptions{
		Scheduler: sched,
		Signer:    signer,
		Etcd:      client,
	}))
	defer server.Close()

	rotateResp := putJSON(t, server.URL+"/v1/gitlab/signer/secrets", map[string]any{
		"secret":  "runner",
		"api_key": "glpat-first",
		"scopes":  []string{"api", "read_repository"},
	})
	initialRevision := int64(rotateResp["revision"].(float64))
	if initialRevision == 0 {
		t.Fatalf("expected initial revision > 0")
	}

	tokenResp := postJSON(t, server.URL+"/v1/gitlab/signer/tokens", map[string]any{
		"secret":      "runner",
		"scopes":      []string{"read_repository"},
		"ttl_seconds": 300,
		"node_id":     "node-http",
	})
	if tokenResp["secret"].(string) != "runner" {
		t.Fatalf("unexpected token secret: %v", tokenResp["secret"])
	}
	if tokenResp["token"].(string) == "" {
		t.Fatalf("expected token value")
	}
	if tokenResp["token_id"].(string) == "" {
		t.Fatalf("expected token_id in response")
	}
	if ttl := int64(tokenResp["ttl_seconds"].(float64)); ttl != 300 {
		t.Fatalf("expected ttl_seconds 300, got %d", ttl)
	}

	eventCh := make(chan map[string]any, 1)
	errCh := make(chan error, 1)

	go func() {
		url := fmt.Sprintf("%s/v1/gitlab/signer/rotations?timeout=5s&since=%d", server.URL, initialRevision)
		resp, err := http.Get(url)
		if err != nil {
			errCh <- err
			return
		}
		defer func() {
			_ = resp.Body.Close()
		}()
		if resp.StatusCode >= 400 {
			body, _ := io.ReadAll(resp.Body)
			errCh <- fmt.Errorf("rotation http %d: %s", resp.StatusCode, string(body))
			return
		}
		if resp.StatusCode == http.StatusNoContent {
			errCh <- fmt.Errorf("rotation returned no content")
			return
		}
		var payload map[string]any
		if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
			errCh <- fmt.Errorf("decode rotation: %w", err)
			return
		}
		eventCh <- payload
	}()

	time.Sleep(150 * time.Millisecond)
	putJSON(t, server.URL+"/v1/gitlab/signer/secrets", map[string]any{
		"secret":  "runner",
		"api_key": "glpat-second",
		"scopes":  []string{"api", "read_repository"},
	})

	select {
	case err := <-errCh:
		t.Fatalf("rotation watcher: %v", err)
	case evt := <-eventCh:
		if evt["secret"].(string) != "runner" {
			t.Fatalf("expected rotation secret runner, got %v", evt["secret"])
		}
		if rev := int64(evt["revision"].(float64)); rev <= initialRevision {
			t.Fatalf("expected revision > %d, got %d", initialRevision, rev)
		}
	case <-time.After(3 * time.Second):
		t.Fatalf("timed out waiting for rotation event")
	}
}
