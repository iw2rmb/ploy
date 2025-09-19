package analysis

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"testing"
	"text/template"
	"time"
)

func TestSubmitJobSuccess(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	kv := newTestKV().(*testKV)
	storage := newTestStorage().(*testStorage)

	dispatcher := &AnalysisDispatcher{
		kv:             kv,
		storage:        storage,
		jobTemplates:   map[string]*template.Template{"pylint": template.Must(template.New("noop").Parse("job \"analysis-pylint-{{.JobID}}\" {}"))},
		storageBaseURL: "http://storage",
	}
	var submitted *AnalysisJob
	dispatcher.submitFn = func(job *AnalysisJob) error {
		submitted = job
		return nil
	}

	input := bytes.NewBufferString("input tarball")
	job, err := dispatcher.SubmitJob(ctx, "pylint", input, map[string]interface{}{"level": "strict"})
	if err != nil {
		t.Fatalf("SubmitJob returned error: %v", err)
	}
	if job == nil {
		t.Fatalf("SubmitJob returned nil job")
	}
	if submitted == nil {
		t.Fatalf("expected submitFn to capture job")
	}
	if job.Status != "submitted" {
		t.Fatalf("job status = %q, want submitted", job.Status)
	}
	if job.StartedAt == nil {
		t.Fatalf("job.StartedAt is nil")
	}
	if !strings.HasPrefix(job.InputURL, "http://storage/artifacts/analysis/inputs/") {
		t.Fatalf("unexpected input URL: %s", job.InputURL)
	}
	storageKey := fmt.Sprintf("artifacts/analysis/inputs/%s.tar.gz", job.ID)
	if _, ok := storage.objects[storageKey]; !ok {
		t.Fatalf("storage missing uploaded key %s", storageKey)
	}
	kvKey := fmt.Sprintf("ploy/analysis/jobs/%s", job.ID)
	if _, ok := kv.data[kvKey]; !ok {
		t.Fatalf("kv missing job key %s", kvKey)
	}
}

func TestSubmitJobNomadFailure(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	kv := newTestKV().(*testKV)
	storage := newTestStorage().(*testStorage)

	dispatcher := &AnalysisDispatcher{
		kv:             kv,
		storage:        storage,
		jobTemplates:   make(map[string]*template.Template),
		storageBaseURL: "http://storage",
	}
	dispatcher.submitFn = func(job *AnalysisJob) error {
		return errors.New("nomad offline")
	}

	_, err := dispatcher.SubmitJob(ctx, "pylint", bytes.NewBufferString("input"), nil)
	if err == nil {
		t.Fatalf("expected error from SubmitJob")
	}
	if !strings.Contains(err.Error(), "failed to submit to Nomad") {
		t.Fatalf("unexpected error: %v", err)
	}

	keys, errKeys := kv.Keys("ploy/analysis/jobs/", "/")
	if errKeys != nil {
		t.Fatalf("Keys error: %v", errKeys)
	}
	if len(keys) != 1 {
		t.Fatalf("expected 1 job key, got %d", len(keys))
	}
	raw, errGet := kv.Get(keys[0])
	if errGet != nil {
		t.Fatalf("Get error: %v", errGet)
	}
	var stored AnalysisJob
	if err := json.Unmarshal(raw, &stored); err != nil {
		t.Fatalf("unmarshal error: %v", err)
	}
	if stored.Status != "failed" {
		t.Fatalf("stored status = %q, want failed", stored.Status)
	}
	if stored.Error == "" {
		t.Fatalf("stored job missing error message")
	}
}

func TestGetJobIncludesResult(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	kv := newTestKV().(*testKV)
	dispatcher := &AnalysisDispatcher{kv: kv}

	job := &AnalysisJob{ID: "job-42", Analyzer: "pylint", Status: "completed"}
	if err := dispatcher.storeJob(job); err != nil {
		t.Fatalf("storeJob error: %v", err)
	}

	result := LanguageAnalysisResult{Analyzer: "pylint", Success: true}
	resultBytes, _ := json.Marshal(result)
	kv.data["ploy/analysis/jobs/job-42/status"] = []byte("completed")
	kv.data["ploy/analysis/jobs/job-42/result"] = resultBytes

	got, err := dispatcher.GetJob(ctx, "job-42")
	if err != nil {
		t.Fatalf("GetJob error: %v", err)
	}
	if got.Result == nil || !got.Result.Success {
		t.Fatalf("expected result with success=true, got %#v", got.Result)
	}
}

func TestListJobsRespectsLimit(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	kv := newTestKV().(*testKV)
	dispatcher := &AnalysisDispatcher{kv: kv}

	for i := 0; i < 3; i++ {
		job := &AnalysisJob{ID: fmt.Sprintf("job-%d", i), Analyzer: "pylint", Status: "completed"}
		if err := dispatcher.storeJob(job); err != nil {
			t.Fatalf("storeJob error: %v", err)
		}
	}

	jobs, err := dispatcher.ListJobs(ctx, 2)
	if err != nil {
		t.Fatalf("ListJobs error: %v", err)
	}
	if len(jobs) != 2 {
		t.Fatalf("ListJobs length = %d, want 2", len(jobs))
	}
}

func TestCleanupOldJobsDeletesAndDeregisters(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	kv := newTestKV().(*testKV)
	dispatcher := &AnalysisDispatcher{kv: kv}

	oldTime := time.Now().Add(-2 * time.Hour)
	jobOld := &AnalysisJob{ID: "old", Analyzer: "pylint", Status: "completed", CompletedAt: &oldTime}
	if err := dispatcher.storeJob(jobOld); err != nil {
		t.Fatalf("storeJob old error: %v", err)
	}

	_ = kv.Put("ploy/analysis/jobs/old/status", []byte("completed"))

	recentTime := time.Now().Add(-10 * time.Minute)
	jobRecent := &AnalysisJob{ID: "recent", Analyzer: "pylint", Status: "completed", CompletedAt: &recentTime}
	if err := dispatcher.storeJob(jobRecent); err != nil {
		t.Fatalf("storeJob recent error: %v", err)
	}

	_ = kv.Put("ploy/analysis/jobs/recent/status", []byte("completed"))

	var deregistered []string
	dispatcher.deregisterFn = func(name string, purge bool) error {
		deregistered = append(deregistered, fmt.Sprintf("%s:%t", name, purge))
		return nil
	}

	if err := dispatcher.CleanupOldJobs(ctx, time.Hour); err != nil {
		t.Fatalf("CleanupOldJobs error: %v", err)
	}

	if _, ok := kv.data["ploy/analysis/jobs/old"]; ok {
		t.Fatalf("old job still present in kv")
	}
	if _, ok := kv.data["ploy/analysis/jobs/recent"]; !ok {
		t.Fatalf("recent job unexpectedly deleted")
	}
	if len(deregistered) != 1 || !strings.HasPrefix(deregistered[0], "analysis-pylint-old") {
		t.Fatalf("unexpected deregister calls: %v", deregistered)
	}
}
