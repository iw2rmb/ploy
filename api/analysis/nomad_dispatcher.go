package analysis

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"text/template"
	"time"

	"github.com/google/uuid"
	consulapi "github.com/hashicorp/consul/api"
	nomadapi "github.com/hashicorp/nomad/api"
	"github.com/iw2rmb/ploy/internal/storage"
)

// AnalysisJob represents a code analysis job
type AnalysisJob struct {
	ID          string                 `json:"id"`
	Analyzer    string                 `json:"analyzer"`    // pylint, eslint, golangci-lint, etc.
	Language    string                 `json:"language"`    // python, javascript, go, etc.
	InputURL    string                 `json:"input_url"`
	OutputURL   string                 `json:"output_url"`
	Config      map[string]interface{} `json:"config,omitempty"`
	Status      string                 `json:"status"` // pending, running, completed, failed
	CreatedAt   time.Time              `json:"created_at"`
	StartedAt   *time.Time             `json:"started_at"`
	CompletedAt *time.Time             `json:"completed_at"`
	Result      *LanguageAnalysisResult `json:"result,omitempty"`
	Error       string                 `json:"error,omitempty"`
}

// AnalysisDispatcher handles job dispatch and monitoring for code analysis
type AnalysisDispatcher struct {
	nomadClient    *nomadapi.Client
	consulClient   *consulapi.Client
	storageClient  *storage.StorageClient
	jobTemplates   map[string]*template.Template
	storageBaseURL string
}

// NewAnalysisDispatcher creates a new dispatcher
func NewAnalysisDispatcher(nomadAddr, consulAddr string, storageClient *storage.StorageClient) (*AnalysisDispatcher, error) {
	// Create Nomad client
	nomadConfig := nomadapi.DefaultConfig()
	if nomadAddr != "" {
		nomadConfig.Address = nomadAddr
	}
	nomadClient, err := nomadapi.NewClient(nomadConfig)
	if err != nil {
		return nil, fmt.Errorf("failed to create Nomad client: %w", err)
	}

	// Create Consul client
	consulConfig := consulapi.DefaultConfig()
	if consulAddr != "" {
		consulConfig.Address = consulAddr
	}
	consulClient, err := consulapi.NewClient(consulConfig)
	if err != nil {
		return nil, fmt.Errorf("failed to create Consul client: %w", err)
	}

	// Get storage base URL from environment
	storageBaseURL := "http://seaweedfs.service.consul:8888"
	if url := os.Getenv("SEAWEEDFS_URL"); url != "" {
		storageBaseURL = url
	}

	dispatcher := &AnalysisDispatcher{
		nomadClient:    nomadClient,
		consulClient:   consulClient,
		storageClient:  storageClient,
		jobTemplates:   make(map[string]*template.Template),
		storageBaseURL: storageBaseURL,
	}

	// Load job templates for different analyzers
	if err := dispatcher.loadJobTemplates(); err != nil {
		return nil, fmt.Errorf("failed to load job templates: %w", err)
	}

	return dispatcher, nil
}

// loadJobTemplates loads Nomad job templates for different analyzers
func (d *AnalysisDispatcher) loadJobTemplates() error {
	// Pylint template for Python analysis
	pylintTemplate := `
job "analysis-pylint-{{.JobID}}" {
  datacenters = ["dc1"]
  type = "batch"
  priority = 70
  
  group "analyze" {
    count = 1
    
    ephemeral_disk {
      size = 512
    }
    
    task "pylint" {
      driver = "docker"
      
      config {
        image = "registry.dev.ployman.app/analysis-pylint:latest"
        volumes = ["local:/workspace"]
        readonly_rootfs = true
      }
      
      env {
        JOB_ID = "{{.JobID}}"
        ANALYZER = "pylint"
        INPUT_URL = "{{.InputURL}}"
        OUTPUT_URL = "{{.OutputURL}}"
        CONSUL_HTTP_ADDR = "{{.ConsulAddr}}"
        CONFIG = "{{.ConfigJSON}}"
      }
      
      template {
        data = <<EOF
#!/bin/sh
set -e

# Download input code archive
wget -q -O /workspace/input.tar.gz "$INPUT_URL"

# Extract code
cd /workspace
tar -xzf input.tar.gz

# Run Pylint analysis
pylint --output-format=json --reports=no --score=no \
  $(find . -name "*.py" -type f) > /workspace/analysis.json 2>&1 || true

# Convert to our format
python3 -c "
import json
import sys

try:
    with open('/workspace/analysis.json', 'r') as f:
        pylint_output = json.load(f)
except:
    pylint_output = []

issues = []
for msg in pylint_output:
    issues.append({
        'file': msg.get('path', ''),
        'line': msg.get('line', 0),
        'column': msg.get('column', 0),
        'severity': msg.get('type', 'info'),
        'rule': msg.get('message-id', ''),
        'message': msg.get('message', ''),
        'category': msg.get('category', 'general')
    })

result = {
    'language': 'python',
    'analyzer': 'pylint',
    'success': True,
    'issues': issues,
    'metrics': {
        'total_issues': len(issues)
    }
}

with open('/workspace/output.json', 'w') as f:
    json.dump(result, f)
"

# Upload output
curl -X PUT "$OUTPUT_URL" --data-binary @/workspace/output.json

# Update job status in Consul
consul kv put "ploy/analysis/jobs/$JOB_ID/status" "completed"
consul kv put "ploy/analysis/jobs/$JOB_ID/completed_at" "$(date -u +%Y-%m-%dT%H:%M:%SZ)"

# Store result in Consul
consul kv put "ploy/analysis/jobs/$JOB_ID/result" "$(cat /workspace/output.json)"
EOF
        destination = "local/run.sh"
        perms = "0755"
      }
      
      config {
        command = "/bin/sh"
        args = ["local/run.sh"]
      }
      
      resources {
        cpu = 256
        memory = 256
      }
      
      kill_timeout = "30s"
    }
  }
  
  reschedule {
    attempts = 2
    interval = "1m"
    delay = "10s"
    unlimited = false
  }
}
`

	// ESLint template for JavaScript/TypeScript analysis
	eslintTemplate := `
job "analysis-eslint-{{.JobID}}" {
  datacenters = ["dc1"]
  type = "batch"
  priority = 70
  
  group "analyze" {
    count = 1
    
    ephemeral_disk {
      size = 512
    }
    
    task "eslint" {
      driver = "docker"
      
      config {
        image = "registry.dev.ployman.app/analysis-eslint:latest"
        volumes = ["local:/workspace"]
        readonly_rootfs = true
      }
      
      env {
        JOB_ID = "{{.JobID}}"
        ANALYZER = "eslint"
        INPUT_URL = "{{.InputURL}}"
        OUTPUT_URL = "{{.OutputURL}}"
        CONSUL_HTTP_ADDR = "{{.ConsulAddr}}"
        CONFIG = "{{.ConfigJSON}}"
      }
      
      template {
        data = <<EOF
#!/bin/sh
set -e

# Download and extract
wget -q -O /workspace/input.tar.gz "$INPUT_URL"
cd /workspace
tar -xzf input.tar.gz

# Run ESLint
npx eslint --format json --ext .js,.jsx,.ts,.tsx . > /workspace/analysis.json 2>&1 || true

# Convert to our format and upload
node -e "
const fs = require('fs');
const analysis = JSON.parse(fs.readFileSync('/workspace/analysis.json', 'utf8'));

const issues = [];
for (const file of analysis) {
  for (const msg of file.messages) {
    issues.push({
      file: file.filePath,
      line: msg.line || 0,
      column: msg.column || 0,
      severity: msg.severity === 2 ? 'error' : 'warning',
      rule: msg.ruleId || '',
      message: msg.message
    });
  }
}

const result = {
  language: 'javascript',
  analyzer: 'eslint',
  success: true,
  issues: issues,
  metrics: { total_issues: issues.length }
};

fs.writeFileSync('/workspace/output.json', JSON.stringify(result));
"

# Upload and update status
curl -X PUT "$OUTPUT_URL" --data-binary @/workspace/output.json
consul kv put "ploy/analysis/jobs/$JOB_ID/status" "completed"
consul kv put "ploy/analysis/jobs/$JOB_ID/result" "$(cat /workspace/output.json)"
EOF
        destination = "local/run.sh"
        perms = "0755"
      }
      
      config {
        command = "/bin/sh"
        args = ["local/run.sh"]
      }
      
      resources {
        cpu = 256
        memory = 512
      }
    }
  }
}
`

	// GolangCI-Lint template for Go analysis
	golangciTemplate := `
job "analysis-golangci-{{.JobID}}" {
  datacenters = ["dc1"]
  type = "batch"
  priority = 70
  
  group "analyze" {
    count = 1
    
    ephemeral_disk {
      size = 1024
    }
    
    task "golangci" {
      driver = "docker"
      
      config {
        image = "golangci/golangci-lint:latest"
        volumes = ["local:/workspace"]
        readonly_rootfs = false
      }
      
      env {
        JOB_ID = "{{.JobID}}"
        ANALYZER = "golangci-lint"
        INPUT_URL = "{{.InputURL}}"
        OUTPUT_URL = "{{.OutputURL}}"
        CONSUL_HTTP_ADDR = "{{.ConsulAddr}}"
      }
      
      template {
        data = <<EOF
#!/bin/sh
set -e

# Download and extract
wget -q -O /workspace/input.tar.gz "$INPUT_URL"
cd /workspace
tar -xzf input.tar.gz

# Run GolangCI-Lint
golangci-lint run --out-format json > /workspace/analysis.json 2>&1 || true

# Process and upload results
# (Processing logic would be similar to pylint/eslint)
curl -X PUT "$OUTPUT_URL" --data-binary @/workspace/analysis.json

# Update status
consul kv put "ploy/analysis/jobs/$JOB_ID/status" "completed"
EOF
        destination = "local/run.sh"
        perms = "0755"
      }
      
      config {
        command = "/bin/sh"
        args = ["local/run.sh"]
      }
      
      resources {
        cpu = 500
        memory = 1024
      }
    }
  }
}
`

	// Parse templates
	templates := map[string]string{
		"pylint":       pylintTemplate,
		"eslint":       eslintTemplate,
		"golangci-lint": golangciTemplate,
	}

	for name, content := range templates {
		tmpl, err := template.New(name).Parse(content)
		if err != nil {
			return fmt.Errorf("failed to parse %s template: %w", name, err)
		}
		d.jobTemplates[name] = tmpl
	}

	return nil
}

// SubmitJob submits a new analysis job
func (d *AnalysisDispatcher) SubmitJob(ctx context.Context, analyzer string, inputTar io.Reader, config map[string]interface{}) (*AnalysisJob, error) {
	jobID := uuid.New().String()

	// Upload input to storage
	inputURL, err := d.uploadInput(ctx, jobID, inputTar)
	if err != nil {
		return nil, fmt.Errorf("failed to upload input: %w", err)
	}

	// Prepare output URL
	outputURL := fmt.Sprintf("%s/ploy-artifacts/analysis/outputs/%s.json", d.storageBaseURL, jobID)

	// Determine language from analyzer
	language := d.getLanguageForAnalyzer(analyzer)

	// Create job record
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

	// Store job in Consul KV
	if err := d.storeJob(job); err != nil {
		return nil, fmt.Errorf("failed to store job: %w", err)
	}

	// Submit to Nomad
	if err := d.submitToNomad(job); err != nil {
		job.Status = "failed"
		job.Error = err.Error()
		_ = d.storeJob(job)
		return nil, fmt.Errorf("failed to submit to Nomad: %w", err)
	}

	// Update status
	job.Status = "submitted"
	now := time.Now()
	job.StartedAt = &now
	_ = d.storeJob(job)

	return job, nil
}

// GetJob retrieves job status from Consul
func (d *AnalysisDispatcher) GetJob(ctx context.Context, jobID string) (*AnalysisJob, error) {
	kv := d.consulClient.KV()

	// Get job data
	pair, _, err := kv.Get(fmt.Sprintf("ploy/analysis/jobs/%s", jobID), nil)
	if err != nil {
		return nil, fmt.Errorf("failed to get job from Consul: %w", err)
	}
	if pair == nil {
		return nil, fmt.Errorf("job not found: %s", jobID)
	}

	var job AnalysisJob
	if err := json.Unmarshal(pair.Value, &job); err != nil {
		return nil, fmt.Errorf("failed to unmarshal job: %w", err)
	}

	// Check for completion
	statusPair, _, _ := kv.Get(fmt.Sprintf("ploy/analysis/jobs/%s/status", jobID), nil)
	if statusPair != nil {
		job.Status = string(statusPair.Value)
	}

	// Get result if completed
	if job.Status == "completed" {
		resultPair, _, _ := kv.Get(fmt.Sprintf("ploy/analysis/jobs/%s/result", jobID), nil)
		if resultPair != nil {
			var result LanguageAnalysisResult
			if err := json.Unmarshal(resultPair.Value, &result); err == nil {
				job.Result = &result
			}
		}
	}

	return &job, nil
}

// WaitForCompletion waits for a job to complete or timeout
func (d *AnalysisDispatcher) WaitForCompletion(ctx context.Context, jobID string, timeout time.Duration) (*AnalysisJob, error) {
	deadline := time.Now().Add(timeout)
	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-ticker.C:
			job, err := d.GetJob(ctx, jobID)
			if err != nil {
				return nil, err
			}

			if job.Status == "completed" || job.Status == "failed" {
				return job, nil
			}

			if time.Now().After(deadline) {
				return nil, fmt.Errorf("timeout waiting for job %s", jobID)
			}
		}
	}
}

// ListJobs lists all analysis jobs from Consul
func (d *AnalysisDispatcher) ListJobs(ctx context.Context, limit int) ([]*AnalysisJob, error) {
	kv := d.consulClient.KV()

	// List all job keys
	keys, _, err := kv.Keys("ploy/analysis/jobs/", "/", nil)
	if err != nil {
		return nil, fmt.Errorf("failed to list jobs: %w", err)
	}

	jobs := make([]*AnalysisJob, 0)
	for i, key := range keys {
		if i >= limit && limit > 0 {
			break
		}

		// Skip sub-keys
		if key == "ploy/analysis/jobs/" {
			continue
		}

		// Extract job ID
		jobID := key[len("ploy/analysis/jobs/"):]
		if jobID == "" {
			continue
		}

		job, err := d.GetJob(ctx, jobID)
		if err == nil {
			jobs = append(jobs, job)
		}
	}

	return jobs, nil
}

// uploadInput uploads the input tar to storage
func (d *AnalysisDispatcher) uploadInput(ctx context.Context, jobID string, inputTar io.Reader) (string, error) {
	bucket := "ploy-artifacts"
	key := fmt.Sprintf("analysis/inputs/%s.tar.gz", jobID)

	// Read input into buffer for ReadSeeker
	buf := new(bytes.Buffer)
	if _, err := io.Copy(buf, inputTar); err != nil {
		return "", fmt.Errorf("failed to read input: %w", err)
	}

	// Upload to storage
	reader := bytes.NewReader(buf.Bytes())
	_, err := d.storageClient.PutObject(bucket, key, reader, "application/gzip")
	if err != nil {
		return "", fmt.Errorf("failed to upload to storage: %w", err)
	}

	return fmt.Sprintf("%s/%s/%s", d.storageBaseURL, bucket, key), nil
}

// storeJob stores job in Consul KV
func (d *AnalysisDispatcher) storeJob(job *AnalysisJob) error {
	kv := d.consulClient.KV()

	data, err := json.Marshal(job)
	if err != nil {
		return fmt.Errorf("failed to marshal job: %w", err)
	}

	p := &consulapi.KVPair{
		Key:   fmt.Sprintf("ploy/analysis/jobs/%s", job.ID),
		Value: data,
	}

	_, err = kv.Put(p, nil)
	return err
}

// submitToNomad submits the job to Nomad
func (d *AnalysisDispatcher) submitToNomad(job *AnalysisJob) error {
	// Get the appropriate template
	tmpl, ok := d.jobTemplates[job.Analyzer]
	if !ok {
		return fmt.Errorf("no template for analyzer: %s", job.Analyzer)
	}

	// Generate HCL from template
	var buf bytes.Buffer

	// Get Consul address from environment or default
	consulAddr := os.Getenv("CONSUL_HTTP_ADDR")
	if consulAddr == "" {
		consulAddr = "http://localhost:8500"
	}

	// Marshal config to JSON
	configJSON := "{}"
	if job.Config != nil {
		configBytes, _ := json.Marshal(job.Config)
		configJSON = string(configBytes)
	}

	err := tmpl.Execute(&buf, map[string]string{
		"JobID":      job.ID,
		"InputURL":   job.InputURL,
		"OutputURL":  job.OutputURL,
		"ConsulAddr": consulAddr,
		"ConfigJSON": configJSON,
	})
	if err != nil {
		return fmt.Errorf("failed to generate job HCL: %w", err)
	}

	// Parse HCL to Job struct
	nomadJob, err := d.nomadClient.Jobs().ParseHCL(buf.String(), true)
	if err != nil {
		return fmt.Errorf("failed to parse job HCL: %w", err)
	}

	// Submit job
	_, _, err = d.nomadClient.Jobs().Register(nomadJob, nil)
	if err != nil {
		return fmt.Errorf("failed to register job with Nomad: %w", err)
	}

	return nil
}

// getLanguageForAnalyzer returns the language associated with an analyzer
func (d *AnalysisDispatcher) getLanguageForAnalyzer(analyzer string) string {
	switch analyzer {
	case "pylint", "flake8", "mypy", "bandit":
		return "python"
	case "eslint", "jshint", "tslint":
		return "javascript"
	case "golangci-lint", "go-vet":
		return "go"
	case "rubocop":
		return "ruby"
	case "phpstan", "psalm":
		return "php"
	case "spotbugs", "checkstyle", "pmd":
		return "java"
	default:
		return "unknown"
	}
}

// CleanupOldJobs removes completed jobs older than specified duration
func (d *AnalysisDispatcher) CleanupOldJobs(ctx context.Context, maxAge time.Duration) error {
	jobs, err := d.ListJobs(ctx, 0)
	if err != nil {
		return err
	}

	kv := d.consulClient.KV()
	cutoff := time.Now().Add(-maxAge)

	for _, job := range jobs {
		if job.Status == "completed" || job.Status == "failed" {
			if job.CompletedAt != nil && job.CompletedAt.Before(cutoff) {
				// Delete from Consul
				_, err := kv.Delete(fmt.Sprintf("ploy/analysis/jobs/%s", job.ID), nil)
				if err != nil {
					return fmt.Errorf("failed to delete job %s: %w", job.ID, err)
				}

				// Stop Nomad job if still exists
				jobName := fmt.Sprintf("analysis-%s-%s", job.Analyzer, job.ID)
				_, _, _ = d.nomadClient.Jobs().Deregister(jobName, false, nil)
			}
		}
	}

	return nil
}