package httpapi_test

import (
	"bufio"
	"bytes"
	"context"
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
	"github.com/iw2rmb/ploy/internal/node/logstream"
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

	server := httptest.NewServer(httpapi.New(sched, nil, nil))
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

	server := httptest.NewServer(httpapi.New(sched, signer, nil))
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

func TestLogsStreamDeliversEvents(t *testing.T) {
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

	streams := logstream.NewHub(logstream.Options{BufferSize: 8, HistorySize: 16})
	jobID := "job-stream-1"
	streams.Ensure(jobID)

	server := httptest.NewServer(httpapi.New(sched, nil, streams))
	defer server.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	events := make(chan sseEvent, 4)
	errCh := make(chan error, 1)
	go func() {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, fmt.Sprintf("%s/v2/jobs/%s/logs/stream", server.URL, jobID), nil)
		if err != nil {
			errCh <- err
			return
		}
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			errCh <- err
			return
		}
		if resp.StatusCode != http.StatusOK {
			body, _ := io.ReadAll(resp.Body)
			_ = resp.Body.Close()
			errCh <- fmt.Errorf("http %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
			return
		}
		defer func() {
			_ = resp.Body.Close()
		}()
		reader := bufio.NewReader(resp.Body)
		for {
			evt, err := readSSEEvent(reader)
			if err != nil {
				errCh <- err
				return
			}
			events <- evt
			if evt.Type == "done" {
				return
			}
		}
	}()

	time.Sleep(50 * time.Millisecond)

	if err := streams.PublishLog(context.Background(), jobID, logstream.LogRecord{
		Timestamp: "2025-10-22T12:00:00Z",
		Stream:    "stdout",
		Line:      "starting job",
	}); err != nil {
		t.Fatalf("publish log: %v", err)
	}
	if err := streams.PublishRetention(context.Background(), jobID, logstream.RetentionHint{
		Retained: true,
		TTL:      "72h",
		Expires:  "2025-10-25T12:00:00Z",
		Bundle:   "bafy-log-bundle",
	}); err != nil {
		t.Fatalf("publish retention: %v", err)
	}
	if err := streams.PublishStatus(context.Background(), jobID, logstream.Status{Status: "completed"}); err != nil {
		t.Fatalf("publish status: %v", err)
	}

	expect := []struct {
		event string
		check func(data string)
	}{
		{
			event: "log",
			check: func(data string) {
				var payload logstream.LogRecord
				if err := json.Unmarshal([]byte(data), &payload); err != nil {
					t.Fatalf("decode log payload: %v", err)
				}
				if payload.Line != "starting job" {
					t.Fatalf("unexpected log line: %q", payload.Line)
				}
			},
		},
		{
			event: "retention",
			check: func(data string) {
				var payload logstream.RetentionHint
				if err := json.Unmarshal([]byte(data), &payload); err != nil {
					t.Fatalf("decode retention payload: %v", err)
				}
				if !payload.Retained || payload.Bundle != "bafy-log-bundle" {
					t.Fatalf("unexpected retention payload: %+v", payload)
				}
			},
		},
		{
			event: "done",
			check: func(data string) {
				var payload logstream.Status
				if err := json.Unmarshal([]byte(data), &payload); err != nil {
					t.Fatalf("decode status payload: %v", err)
				}
				if payload.Status != "completed" {
					t.Fatalf("unexpected status payload: %+v", payload)
				}
			},
		},
	}

	for _, want := range expect {
		select {
		case evt := <-events:
			if evt.Type != want.event {
				t.Fatalf("expected event %q, got %q", want.event, evt.Type)
			}
			want.check(evt.Data)
		case err := <-errCh:
			t.Fatalf("stream error: %v", err)
		case <-time.After(1 * time.Second):
			t.Fatalf("timed out waiting for %s event", want.event)
		}
	}
}

func TestLogsStreamResumesWithLastEventID(t *testing.T) {
	t.Parallel()

	etcd, client := startTestEtcd(t)
	defer etcd.Close()
	defer func() { _ = client.Close() }()

	sched, err := scheduler.New(client, scheduler.Options{LeaseTTL: 3 * time.Second})
	if err != nil {
		t.Fatalf("new scheduler: %v", err)
	}
	defer func() { _ = sched.Close() }()

	streams := logstream.NewHub(logstream.Options{BufferSize: 8, HistorySize: 16})
	jobID := "job-resume-1"
	streams.Ensure(jobID)

	server := httptest.NewServer(httpapi.New(sched, nil, streams))
	defer server.Close()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	firstEvents := make(chan sseEvent, 3)
	errCh := make(chan error, 1)

	go func() {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, fmt.Sprintf("%s/v2/jobs/%s/logs/stream", server.URL, jobID), nil)
		if err != nil {
			errCh <- err
			return
		}
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			errCh <- err
			return
		}
		if resp.StatusCode != http.StatusOK {
			body, _ := io.ReadAll(resp.Body)
			_ = resp.Body.Close()
			errCh <- fmt.Errorf("http %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
			return
		}
		defer func() {
			_ = resp.Body.Close()
		}()

		reader := bufio.NewReader(resp.Body)
		for {
			evt, err := readSSEEvent(reader)
			if err != nil {
				errCh <- err
				return
			}
			firstEvents <- evt
			if len(firstEvents) == 2 {
				cancel()
				return
			}
		}
	}()

	go func() {
		_ = streams.PublishLog(context.Background(), jobID, logstream.LogRecord{Timestamp: "2025-10-22T12:10:00Z", Stream: "stdout", Line: "phase one"})
		time.Sleep(50 * time.Millisecond)
		_ = streams.PublishRetention(context.Background(), jobID, logstream.RetentionHint{Retained: false, TTL: "", Bundle: ""})
		time.Sleep(50 * time.Millisecond)
		_ = streams.PublishStatus(context.Background(), jobID, logstream.Status{Status: "completed"})
	}()

	var lastID string
	for i := 0; i < 2; i++ {
		select {
		case evt := <-firstEvents:
			lastID = evt.ID
		case err := <-errCh:
			t.Fatalf("stream error: %v", err)
		case <-time.After(2 * time.Second):
			t.Fatalf("timeout waiting for initial events")
		}
	}

	if lastID == "" {
		t.Fatalf("expected last event id to be captured")
	}

	resumeReq, err := http.NewRequest(http.MethodGet, fmt.Sprintf("%s/v2/jobs/%s/logs/stream", server.URL, jobID), nil)
	if err != nil {
		t.Fatalf("resume request: %v", err)
	}
	resumeReq.Header.Set("Last-Event-ID", lastID)

	resp, err := http.DefaultClient.Do(resumeReq)
	if err != nil {
		t.Fatalf("resume stream: %v", err)
	}
	defer func() {
		_ = resp.Body.Close()
	}()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("resume http %d: %s", resp.StatusCode, string(body))
	}

	reader := bufio.NewReader(resp.Body)
	evt, err := readSSEEvent(reader)
	if err != nil {
		t.Fatalf("resume read: %v", err)
	}
	if evt.Type != "done" {
		t.Fatalf("expected done event on resume, got %s", evt.Type)
	}
}

type sseEvent struct {
	ID   string
	Type string
	Data string
}

func readSSEEvent(r *bufio.Reader) (sseEvent, error) {
	var evt sseEvent
	for {
		line, err := r.ReadString('\n')
		if err != nil {
			return evt, err
		}
		line = strings.TrimRight(line, "\r\n")
		if line == "" {
			if evt.Type == "" && evt.Data == "" && evt.ID == "" {
				continue
			}
			return evt, nil
		}
		switch {
		case strings.HasPrefix(line, "event:"):
			evt.Type = strings.TrimSpace(strings.TrimPrefix(line, "event:"))
		case strings.HasPrefix(line, "data:"):
			if evt.Data != "" {
				evt.Data += "\n"
			}
			evt.Data += strings.TrimSpace(strings.TrimPrefix(line, "data:"))
		case strings.HasPrefix(line, "id:"):
			evt.ID = strings.TrimSpace(strings.TrimPrefix(line, "id:"))
		default:
			// ignore comments and unknown fields
		}
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
