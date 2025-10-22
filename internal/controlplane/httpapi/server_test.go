package httpapi_test

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"strings"
	"testing"
	"time"

	clientv3 "go.etcd.io/etcd/client/v3"
	"go.etcd.io/etcd/server/v3/embed"

	"github.com/iw2rmb/ploy/internal/config/gitlab"
	"github.com/iw2rmb/ploy/internal/controlplane/httpapi"
	"github.com/iw2rmb/ploy/internal/controlplane/scheduler"
)

func TestServerJobLifecycle(t *testing.T) {
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

	server := httptest.NewServer(httpapi.New(sched, nil))
	defer server.Close()

	submit := map[string]any{
		"ticket":       "mod-900",
		"step_id":      "plan",
		"priority":     "default",
		"max_attempts": 2,
	}
	job := postJSON(t, server.URL+"/v2/jobs", submit)

	if job["state"].(string) != "queued" {
		t.Fatalf("expected queued state, got %v", job["state"])
	}

	claim := postJSON(t, server.URL+"/v2/jobs/claim", map[string]any{"node_id": "node-http"})
	if claim["status"].(string) != "claimed" {
		t.Fatalf("claim status: %v", claim)
	}
	claimedJob := claim["job"].(map[string]any)
	jobID := claimedJob["id"].(string)

	postJSON(t, server.URL+"/v2/jobs/"+jobID+"/heartbeat", map[string]any{
		"ticket":  "mod-900",
		"node_id": "node-http",
	})

	complete := postJSON(t, server.URL+"/v2/jobs/"+jobID+"/complete", map[string]any{
		"ticket":  "mod-900",
		"node_id": "node-http",
		"state":   "succeeded",
	})
	if complete["state"].(string) != "succeeded" {
		t.Fatalf("completion state: %v", complete["state"])
	}

	listURL := fmt.Sprintf("%s/v2/jobs?ticket=%s", server.URL, url.QueryEscape("mod-900"))
	resp := getJSON(t, listURL)
	jobs := resp["jobs"].([]any)
	if len(jobs) != 1 {
		t.Fatalf("expected one job, got %d", len(jobs))
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

	server := httptest.NewServer(httpapi.New(sched, signer))
	defer server.Close()

	rotateResp := putJSON(t, server.URL+"/v2/gitlab/signer/secrets", map[string]any{
		"secret":  "runner",
		"api_key": "glpat-first",
		"scopes":  []string{"api", "read_repository"},
	})
	initialRevision := int64(rotateResp["revision"].(float64))
	if initialRevision == 0 {
		t.Fatalf("expected initial revision > 0")
	}

	tokenResp := postJSON(t, server.URL+"/v2/gitlab/signer/tokens", map[string]any{
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
		url := fmt.Sprintf("%s/v2/gitlab/signer/rotations?timeout=5s&since=%d", server.URL, initialRevision)
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
	putJSON(t, server.URL+"/v2/gitlab/signer/secrets", map[string]any{
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

func startTestEtcd(t *testing.T) (*embed.Etcd, *clientv3.Client) {
	t.Helper()
	dir := t.TempDir()
	cfg := embed.NewConfig()
	cfg.Dir = dir
	clientURL := mustParseURL("http://127.0.0.1:0")
	peerURL := mustParseURL("http://127.0.0.1:0")
	cfg.ListenClientUrls = []url.URL{clientURL}
	cfg.ListenPeerUrls = []url.URL{peerURL}
	cfg.AdvertiseClientUrls = []url.URL{clientURL}
	cfg.AdvertisePeerUrls = []url.URL{peerURL}
	cfg.Name = "default"
	cfg.InitialCluster = fmt.Sprintf("%s=%s", cfg.Name, peerURL.String())
	cfg.ClusterState = embed.ClusterStateFlagNew
	cfg.InitialClusterToken = "httpapi-test"
	cfg.LogLevel = "panic"
	cfg.Logger = "zap"
	cfg.LogOutputs = []string{filepath.Join(dir, "etcd.log")}

	e, err := embed.StartEtcd(cfg)
	if err != nil {
		t.Fatalf("start etcd: %v", err)
	}
	select {
	case <-e.Server.ReadyNotify():
	case <-time.After(10 * time.Second):
		e.Server.Stop()
		t.Fatalf("etcd start timeout")
	}

	client, err := clientv3.New(clientv3.Config{
		Endpoints:   []string{e.Clients[0].Addr().String()},
		DialTimeout: 5 * time.Second,
	})
	if err != nil {
		e.Close()
		t.Fatalf("client: %v", err)
	}

	return e, client
}

func mustParseURL(raw string) url.URL {
	parsed, err := url.Parse(raw)
	if err != nil {
		panic(err)
	}
	return *parsed
}

func postJSON(t *testing.T, endpoint string, payload map[string]any) map[string]any {
	return sendJSON(t, http.MethodPost, endpoint, payload)
}

func putJSON(t *testing.T, endpoint string, payload map[string]any) map[string]any {
	return sendJSON(t, http.MethodPut, endpoint, payload)
}

func getJSON(t *testing.T, endpoint string) map[string]any {
	t.Helper()
	resp, err := http.Get(endpoint)
	if err != nil {
		t.Fatalf("get %s: %v", endpoint, err)
	}
	defer func() {
		_ = resp.Body.Close()
	}()
	if resp.StatusCode >= 400 {
		data, _ := io.ReadAll(resp.Body)
		t.Fatalf("http %d: %s", resp.StatusCode, string(data))
	}
	var out map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	return out
}

func sendJSON(t *testing.T, method, endpoint string, payload map[string]any) map[string]any {
	t.Helper()
	body, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("marshal payload: %v", err)
	}
	req, err := http.NewRequest(method, endpoint, bytes.NewReader(body))
	if err != nil {
		t.Fatalf("build request: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("%s %s: %v", method, endpoint, err)
	}
	defer func() {
		_ = resp.Body.Close()
	}()
	if resp.StatusCode >= 400 {
		data, _ := io.ReadAll(resp.Body)
		t.Fatalf("%s %s -> http %d: %s", method, endpoint, resp.StatusCode, string(data))
	}
	var out map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	return out
}
