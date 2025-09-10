package transflow

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/gofiber/fiber/v2"
	internalStorage "github.com/iw2rmb/ploy/internal/storage"
	"github.com/stretchr/testify/assert"
)

// --- Mocks ---

type mockKV struct{ m map[string][]byte }

func newMockKV() *mockKV { return &mockKV{m: map[string][]byte{}} }

func (k *mockKV) Put(key string, value []byte) error {
	k.m[key] = append([]byte(nil), value...)
	return nil
}
func (k *mockKV) Get(key string) ([]byte, error) {
	v, ok := k.m[key]
	if !ok {
		return nil, nil
	}
	return append([]byte(nil), v...), nil
}
func (k *mockKV) Keys(prefix, sep string) ([]string, error) {
	keys := []string{}
	for k := range k.m {
		if len(prefix) == 0 || (len(k) >= len(prefix) && k[:len(prefix)] == prefix) {
			keys = append(keys, k)
		}
	}
	return keys, nil
}
func (k *mockKV) Delete(key string) error { delete(k.m, key); return nil }

type mockStorage struct{ m map[string][]byte }

func newMockStorage() *mockStorage { return &mockStorage{m: map[string][]byte{}} }

func (s *mockStorage) Get(ctx context.Context, key string) (io.ReadCloser, error) {
	b, ok := s.m[key]
	if !ok {
		return io.NopCloser(bytes.NewReader(nil)), nil
	}
	return io.NopCloser(bytes.NewReader(b)), nil
}
func (s *mockStorage) Put(ctx context.Context, key string, reader io.Reader, opts ...internalStorage.PutOption) error {
	b, _ := io.ReadAll(reader)
	s.m[key] = append([]byte(nil), b...)
	return nil
}
func (s *mockStorage) Delete(ctx context.Context, key string) error { delete(s.m, key); return nil }
func (s *mockStorage) Exists(ctx context.Context, key string) (bool, error) {
	_, ok := s.m[key]
	return ok, nil
}
func (s *mockStorage) List(ctx context.Context, opts internalStorage.ListOptions) ([]internalStorage.Object, error) {
	return []internalStorage.Object{}, nil
}
func (s *mockStorage) DeleteBatch(ctx context.Context, keys []string) error {
	for _, k := range keys {
		delete(s.m, k)
	}
	return nil
}
func (s *mockStorage) Head(ctx context.Context, key string) (*internalStorage.Object, error) {
	if b, ok := s.m[key]; ok {
		return &internalStorage.Object{Key: key, Size: int64(len(b)), ContentType: ""}, nil
	}
	return nil, nil
}
func (s *mockStorage) UpdateMetadata(ctx context.Context, key string, metadata map[string]string) error {
	return nil
}
func (s *mockStorage) Copy(ctx context.Context, src, dst string) error {
	if b, ok := s.m[src]; ok {
		s.m[dst] = append([]byte(nil), b...)
	}
	return nil
}
func (s *mockStorage) Move(ctx context.Context, src, dst string) error {
	if b, ok := s.m[src]; ok {
		s.m[dst] = b
		delete(s.m, src)
	}
	return nil
}
func (s *mockStorage) Health(ctx context.Context) error { return nil }
func (s *mockStorage) Metrics() *internalStorage.StorageMetrics {
	return &internalStorage.StorageMetrics{}
}

// --- Tests ---

func TestPersistArtifacts_CapturesDiffAndErrorLog(t *testing.T) {
	tmp := t.TempDir()
	// Create planner/reducer outputs
	os.MkdirAll(filepath.Join(tmp, "planner", "out"), 0755)
	os.WriteFile(filepath.Join(tmp, "planner", "out", "plan.json"), []byte(`{"ok":true}`), 0644)
	os.MkdirAll(filepath.Join(tmp, "reducer", "out"), 0755)
	os.WriteFile(filepath.Join(tmp, "reducer", "out", "next.json"), []byte(`{"next":[]}`), 0644)
	// Create ORW outputs
	os.MkdirAll(filepath.Join(tmp, "orw-apply", "opt1", "out"), 0755)
	os.WriteFile(filepath.Join(tmp, "orw-apply", "opt1", "out", "diff.patch"), []byte("diff --git a b"), 0644)
	os.WriteFile(filepath.Join(tmp, "orw-apply", "opt1", "out", "error.log"), []byte("No build file found"), 0644)

	st := newMockStorage()
	h := &Handler{storage: st}

	arts, err := h.persistArtifacts("tf-xyz", tmp)
	assert.NoError(t, err)
	assert.Contains(t, arts, "plan_json")
	assert.Contains(t, arts, "next_json")
	assert.Contains(t, arts, "diff_patch")
	assert.Contains(t, arts, "error_log")

	// Verify storage keys exist
	for _, k := range arts {
		ok, _ := st.Exists(context.Background(), k)
		assert.True(t, ok, "expected storage to contain %s", k)
	}
}

func TestRecordErrorAndStatusFlow(t *testing.T) {
	kv := newMockKV()
	h := &Handler{statusStore: kv}
	id := "tf-test"
	// Store initial running status
	start := time.Now().Add(-2 * time.Minute)
	st := TransflowStatus{ID: id, Status: "running", StartTime: start}
	assert.NoError(t, h.storeStatus(st))

	// Record error
	h.recordError(id, io.EOF)

	// Retrieve
	app := fiber.New()
	h.RegisterRoutes(app)
	req := httptest.NewRequest("GET", "/v1/transflow/status/"+id, nil)
	resp, _ := app.Test(req)
	defer resp.Body.Close()
	var got TransflowStatus
	dec := json.NewDecoder(resp.Body)
	_ = dec.Decode(&got)
	assert.Equal(t, "failed", got.Status)
	assert.NotNil(t, got.EndTime)
}

func TestArtifactsEndpoints(t *testing.T) {
	kv := newMockKV()
	st := newMockStorage()
	h := &Handler{statusStore: kv, storage: st}
	id := "tf-art"
	// Store status with artifacts map
	status := TransflowStatus{
		ID:     id,
		Status: "completed",
		Result: map[string]any{
			"artifacts": map[string]any{
				"error_log": "artifacts/transflow/" + id + "/error.log",
			},
		},
	}
	_ = h.storeStatus(status)
	// Put content in storage
	_ = st.Put(context.Background(), "artifacts/transflow/"+id+"/error.log", bytes.NewBufferString("fail detail"))

	app := fiber.New()
	h.RegisterRoutes(app)

	// List artifacts
	req := httptest.NewRequest("GET", "/v1/transflow/artifacts/"+id, nil)
	resp, _ := app.Test(req)
	assert.Equal(t, 200, resp.StatusCode)

	// Download artifact
	req2 := httptest.NewRequest("GET", "/v1/transflow/artifacts/"+id+"/error_log", nil)
	resp2, _ := app.Test(req2)
	assert.Equal(t, 200, resp2.StatusCode)
}

func TestSSELogsStub(t *testing.T) {
	kv := newMockKV()
	// Seed minimal status to ensure immediate snapshot
	st := TransflowStatus{ID: "tf-abc", Status: "running", StartTime: time.Now()}
	data, _ := json.Marshal(st)
	_ = kv.Put("transflow/status/"+st.ID, data)
	h := &Handler{statusStore: kv}
	app := fiber.New()
	h.RegisterRoutes(app)

	req := httptest.NewRequest("GET", "/v1/transflow/logs/tf-abc?follow=false", nil)
	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer resp.Body.Close()
	if got := resp.StatusCode; got != 200 {
		t.Fatalf("expected 200, got %d", got)
	}
	if ctype := resp.Header.Get("Content-Type"); ctype != "text/event-stream" {
		t.Fatalf("expected text/event-stream, got %s", ctype)
	}
	b, _ := io.ReadAll(resp.Body)
	s := string(b)
	if !strings.Contains(s, "event: init") || !strings.Contains(s, "tf-abc") {
		t.Fatalf("unexpected SSE body: %s", s)
	}
}

func TestSSELogsSnapshotFollowFalse(t *testing.T) {
	kv := newMockKV()
	// Seed status with steps
	st := TransflowStatus{ID: "tf-sse1", Status: "running", StartTime: time.Now()}
	st.Steps = []TransflowStepStatus{{Step: "clone", Phase: "clone", Level: "info", Message: "Cloning", Time: time.Now()}}
	data, _ := json.Marshal(st)
	_ = kv.Put("transflow/status/"+st.ID, data)

	h := &Handler{statusStore: kv}
	app := fiber.New()
	h.RegisterRoutes(app)

	req := httptest.NewRequest("GET", "/v1/transflow/logs/tf-sse1?follow=false", nil)
	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}
	b, _ := io.ReadAll(resp.Body)
	s := string(b)
	if !strings.Contains(s, "event: init") {
		t.Fatalf("missing init: %s", s)
	}
	if !strings.Contains(s, "event: step") {
		t.Fatalf("missing step: %s", s)
	}
	if !strings.Contains(s, "event: end") {
		t.Fatalf("missing end: %s", s)
	}
}

func TestReportEvent_UpdatesStatusWithStepsAndLastJob(t *testing.T) {
	kv := newMockKV()
	h := &Handler{statusStore: kv}
	app := fiber.New()
	h.RegisterRoutes(app)

	id := "tf-ev1"
	// Seed initial status
	_ = h.storeStatus(TransflowStatus{ID: id, Status: "running", StartTime: time.Now().Add(-time.Minute)})

	// Send a phase event
	ev1 := TransflowEvent{ExecutionID: id, Phase: "clone", Step: "clone", Level: "info", Message: "Cloning repo"}
	b1, _ := json.Marshal(ev1)
	req1 := httptest.NewRequest("POST", "/v1/transflow/event", bytes.NewReader(b1))
	req1.Header.Set("Content-Type", "application/json")
	resp1, _ := app.Test(req1)
	assert.Equal(t, 200, resp1.StatusCode)

	// Send a job metadata event
	ev2 := TransflowEvent{ExecutionID: id, Phase: "apply", Step: "orw-apply", Level: "info", Message: "Submitted orw-apply", JobName: "orw-apply-123"}
	b2, _ := json.Marshal(ev2)
	req2 := httptest.NewRequest("POST", "/v1/transflow/event", bytes.NewReader(b2))
	req2.Header.Set("Content-Type", "application/json")
	resp2, _ := app.Test(req2)
	assert.Equal(t, 200, resp2.StatusCode)

	// Read back status
	req := httptest.NewRequest("GET", "/v1/transflow/status/"+id, nil)
	resp, _ := app.Test(req)
	defer resp.Body.Close()
	var got TransflowStatus
	_ = json.NewDecoder(resp.Body).Decode(&got)
	assert.Equal(t, id, got.ID)
	assert.Equal(t, "apply", got.Phase)
	if assert.NotNil(t, got.LastJob) {
		assert.Equal(t, "orw-apply-123", got.LastJob.JobName)
	}
	if assert.True(t, len(got.Steps) >= 2) {
		assert.Equal(t, "clone", got.Steps[0].Step)
		assert.Equal(t, "orw-apply", got.Steps[1].Step)
	}
}
