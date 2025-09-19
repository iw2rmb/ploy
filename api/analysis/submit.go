package analysis

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"time"

	"github.com/google/uuid"
)

// SubmitJob uploads the analysis input, persists metadata, and submits a Nomad job.
func (d *AnalysisDispatcher) SubmitJob(ctx context.Context, analyzer string, inputTar io.Reader, config map[string]interface{}) (*AnalysisJob, error) {
	jobID := uuid.New().String()

	inputURL, err := d.uploadInput(ctx, jobID, inputTar)
	if err != nil {
		return nil, fmt.Errorf("failed to upload input: %w", err)
	}

	outputURL := fmt.Sprintf("%s/analysis/outputs/%s.json", d.storageBaseURL, jobID)
	language := d.getLanguageForAnalyzer(analyzer)

	job := &AnalysisJob{
		ID:        jobID,
		Analyzer:  analyzer,
		Language:  language,
		InputURL:  inputURL,
		OutputURL: outputURL,
		Config:    config,
		Status:    "pending",
		CreatedAt: time.Now(),
	}

	if err := d.storeJob(job); err != nil {
		return nil, fmt.Errorf("failed to store job: %w", err)
	}

	if err := d.submitToNomad(job); err != nil {
		job.Status = "failed"
		job.Error = err.Error()
		_ = d.storeJob(job)
		return nil, fmt.Errorf("failed to submit to Nomad: %w", err)
	}

	now := time.Now()
	job.Status = "submitted"
	job.StartedAt = &now
	_ = d.storeJob(job)

	return job, nil
}

func (d *AnalysisDispatcher) uploadInput(ctx context.Context, jobID string, inputTar io.Reader) (string, error) {
	bucket := "artifacts"
	key := fmt.Sprintf("analysis/inputs/%s.tar.gz", jobID)

	buf := new(bytes.Buffer)
	if _, err := io.Copy(buf, inputTar); err != nil {
		return "", fmt.Errorf("failed to read input: %w", err)
	}

	reader := bytes.NewReader(buf.Bytes())
	if err := d.storage.Put(ctx, fmt.Sprintf("%s/%s", bucket, key), reader); err != nil {
		return "", fmt.Errorf("failed to upload to storage: %w", err)
	}

	return fmt.Sprintf("%s/%s/%s", d.storageBaseURL, bucket, key), nil
}

func (d *AnalysisDispatcher) storeJob(job *AnalysisJob) error {
	data, err := json.Marshal(job)
	if err != nil {
		return fmt.Errorf("failed to marshal job: %w", err)
	}
	return d.kv.Put(fmt.Sprintf("ploy/analysis/jobs/%s", job.ID), data)
}
