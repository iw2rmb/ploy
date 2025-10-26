package httpserver_test

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/iw2rmb/ploy/internal/api/httpserver"
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

	server := httptest.NewServer(newTestControlPlaneHandler(t, httpserver.ControlPlaneOptions{
		Scheduler: sched,
		Etcd:      client,
	}))
	defer server.Close()

	submit := map[string]any{
		"ticket":       "mod-900",
		"step_id":      "plan",
		"priority":     "default",
		"max_attempts": 2,
	}
	job := postJSON(t, server.URL+"/v1/jobs", submit)

	if job["state"].(string) != "queued" {
		t.Fatalf("expected queued state, got %v", job["state"])
	}

	claim := postJSON(t, server.URL+"/v1/jobs/claim", map[string]any{"node_id": "node-http"})
	if claim["status"].(string) != "claimed" {
		t.Fatalf("claim status: %v", claim)
	}
	claimedJob := claim["job"].(map[string]any)
	jobID := claimedJob["id"].(string)

	postJSON(t, server.URL+"/v1/jobs/"+jobID+"/heartbeat", map[string]any{
		"ticket":  "mod-900",
		"node_id": "node-http",
	})

	complete := postJSON(t, server.URL+"/v1/jobs/"+jobID+"/complete", map[string]any{
		"ticket":  "mod-900",
		"node_id": "node-http",
		"state":   "succeeded",
	})
	if complete["state"].(string) != "succeeded" {
		t.Fatalf("completion state: %v", complete["state"])
	}

	listURL := fmt.Sprintf("%s/v1/jobs?ticket=%s", server.URL, url.QueryEscape("mod-900"))
	resp := getJSON(t, listURL)
	jobs := resp["jobs"].([]any)
	if len(jobs) != 1 {
		t.Fatalf("expected one job, got %d", len(jobs))
	}
}

func TestJobRetention(t *testing.T) {
	t.Parallel()

	etcd, client := startTestEtcd(t)
	defer etcd.Close()
	defer func() {
		_ = client.Close()
	}()

	completedAt := time.Date(2025, 10, 22, 17, 0, 0, 0, time.UTC)
	sched, err := scheduler.New(client, scheduler.Options{
		LeaseTTL: 3 * time.Second,
		Now:      func() time.Time { return completedAt },
	})
	if err != nil {
		t.Fatalf("new scheduler: %v", err)
	}
	defer func() {
		_ = sched.Close()
	}()

	server := httptest.NewServer(newTestControlPlaneHandler(t, httpserver.ControlPlaneOptions{
		Scheduler: sched,
		Etcd:      client,
	}))
	defer server.Close()

	submit := map[string]any{
		"ticket":       "mod-retention",
		"step_id":      "logs",
		"priority":     "default",
		"max_attempts": 1,
	}
	job := postJSON(t, server.URL+"/v1/jobs", submit)
	jobID := job["id"].(string)

	claim := postJSON(t, server.URL+"/v1/jobs/claim", map[string]any{"node_id": "node-retention"})
	if claim["status"].(string) != "claimed" {
		t.Fatalf("claim status: %v", claim)
	}

	complete := postJSON(t, server.URL+"/v1/jobs/"+jobID+"/complete", map[string]any{
		"ticket":     "mod-retention",
		"node_id":    "node-retention",
		"state":      "failed",
		"inspection": true,
		"bundles": map[string]any{
			"logs": map[string]any{
				"cid":      "bafy-observed",
				"digest":   "sha256:bundle",
				"size":     8192,
				"retained": true,
				"ttl":      "96h",
			},
		},
	})
	if complete["state"].(string) != "inspection_ready" {
		t.Fatalf("expected inspection_ready state, got %v", complete["state"])
	}

	getURL := fmt.Sprintf("%s/v1/jobs/%s?ticket=%s", server.URL, jobID, url.QueryEscape("mod-retention"))
	jobResp := getJSON(t, getURL)
	retention, ok := jobResp["retention"].(map[string]any)
	if !ok {
		t.Fatalf("expected retention block in job response")
	}
	wantExpires := completedAt.Add(96 * time.Hour).UTC().Format(time.RFC3339Nano)
	if retained, _ := retention["retained"].(bool); !retained {
		t.Fatalf("expected retained flag in job response")
	}
	if bundle, _ := retention["bundle"].(string); bundle != "logs" {
		t.Fatalf("unexpected retention bundle: %v", bundle)
	}
	if cid, _ := retention["bundle_cid"].(string); cid != "bafy-observed" {
		t.Fatalf("unexpected retention cid: %v", cid)
	}
	if ttl, _ := retention["ttl"].(string); ttl != "96h" {
		t.Fatalf("unexpected retention ttl: %v", ttl)
	}
	if expires, _ := retention["expires_at"].(string); expires != wantExpires {
		t.Fatalf("unexpected retention expires_at: %v want %s", expires, wantExpires)
	}
	if inspect, _ := retention["inspection"].(bool); !inspect {
		t.Fatalf("expected inspection hint true")
	}

	bundles, ok := jobResp["bundles"].(map[string]any)
	if !ok {
		t.Fatalf("expected bundles map in job response")
	}
	logBundle, ok := bundles["logs"].(map[string]any)
	if !ok {
		t.Fatalf("expected logs bundle in response")
	}
	if expires, _ := logBundle["expires_at"].(string); expires != wantExpires {
		t.Fatalf("unexpected bundle expires_at: %v want %s", expires, wantExpires)
	}

	listURL := fmt.Sprintf("%s/v1/jobs?ticket=%s", server.URL, url.QueryEscape("mod-retention"))
	listResp := getJSON(t, listURL)
	items, ok := listResp["jobs"].([]any)
	if !ok || len(items) != 1 {
		t.Fatalf("expected job listing")
	}
	item := items[0].(map[string]any)
	retList, ok := item["retention"].(map[string]any)
	if !ok {
		t.Fatalf("expected retention in listing entry")
	}
	if expires, _ := retList["expires_at"].(string); expires != wantExpires {
		t.Fatalf("unexpected list retention expires_at: %v want %s", expires, wantExpires)
	}
}

func TestJobEventsStream(t *testing.T) {
	t.Parallel()

	fixture := newModsServerFixture(t)
	ticket := "mod-job-events"

	jobResp := postJSON(t, fixture.server.URL+"/v1/jobs", map[string]any{
		"ticket":       ticket,
		"step_id":      "plan",
		"priority":     "default",
		"max_attempts": 1,
	})
	jobID, _ := jobResp["id"].(string)
	if strings.TrimSpace(jobID) == "" {
		t.Fatalf("expected job id in response")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	eventCh := make(chan sseEvent, 8)
	errCh := make(chan error, 1)
	go func() {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, fmt.Sprintf("%s/v1/jobs/%s/events", fixture.server.URL, jobID), nil)
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
		defer func() { _ = resp.Body.Close() }()
		reader := bufio.NewReader(resp.Body)
		for {
			evt, err := readSSEEvent(reader)
			if err != nil {
				errCh <- err
				return
			}
			eventCh <- evt
		}
	}()

	waitForState := func(want string) {
		timeout := time.After(4 * time.Second)
		for {
			select {
			case evt := <-eventCh:
				if evt.Type != "job" {
					continue
				}
				var payload map[string]any
				if err := json.Unmarshal([]byte(evt.Data), &payload); err != nil {
					t.Fatalf("decode job event %s: %v", want, err)
				}
				if state, _ := payload["state"].(string); state == want {
					return
				}
			case err := <-errCh:
				t.Fatalf("job events stream error: %v", err)
			case <-timeout:
				t.Fatalf("timed out waiting for job state %s", want)
			}
		}
	}

	waitForState("queued")

	postJSON(t, fixture.server.URL+"/v1/jobs/claim", map[string]any{"node_id": "node-events"})
	waitForState("running")

	postJSON(t, fmt.Sprintf("%s/v1/jobs/%s/heartbeat", fixture.server.URL, jobID), map[string]any{
		"ticket":  ticket,
		"node_id": "node-events",
	})

	postJSON(t, fmt.Sprintf("%s/v1/jobs/%s/complete", fixture.server.URL, jobID), map[string]any{
		"ticket":  ticket,
		"node_id": "node-events",
		"state":   "succeeded",
	})

	waitForState("succeeded")
}
