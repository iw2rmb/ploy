package arf

import (
	"bytes"
	"context"
	"encoding/base64"
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

// LLMJob represents an LLM transformation job
type LLMJob struct {
	ID              string                 `json:"id"`
	Provider        string                 `json:"provider"`        // ollama, openai, anthropic
	Model           string                 `json:"model"`           // specific model name
	Prompt          string                 `json:"prompt"`          // transformation prompt
	InputURL        string                 `json:"input_url"`       // code archive location
	OutputURL       string                 `json:"output_url"`      // transformed code location
	Language        string                 `json:"language"`        // programming language
	Framework       string                 `json:"framework,omitempty"`
	Status          string                 `json:"status"` // pending, running, completed, failed
	CreatedAt       time.Time              `json:"created_at"`
	StartedAt       *time.Time             `json:"started_at"`
	CompletedAt     *time.Time             `json:"completed_at"`
	Result          map[string]interface{} `json:"result,omitempty"`
	Error           string                 `json:"error,omitempty"`
	Temperature     float64                `json:"temperature"`
	MaxTokens       int                    `json:"max_tokens"`
}

// LLMDispatcher handles job dispatch and monitoring for LLM transformations
type LLMDispatcher struct {
	nomadClient    *nomadapi.Client
	consulClient   *consulapi.Client
	storageClient  *storage.StorageClient
	jobTemplates   map[string]*template.Template
	storageBaseURL string
}

// NewLLMDispatcher creates a new dispatcher
func NewLLMDispatcher(nomadAddr, consulAddr string, storageClient *storage.StorageClient) (*LLMDispatcher, error) {
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

	dispatcher := &LLMDispatcher{
		nomadClient:    nomadClient,
		consulClient:   consulClient,
		storageClient:  storageClient,
		jobTemplates:   make(map[string]*template.Template),
		storageBaseURL: storageBaseURL,
	}

	// Load job templates
	if err := dispatcher.loadJobTemplates(); err != nil {
		return nil, fmt.Errorf("failed to load job templates: %w", err)
	}

	return dispatcher, nil
}

// loadJobTemplates loads Nomad job templates for external LLM providers only
func (d *LLMDispatcher) loadJobTemplates() error {
	// Only external API templates are supported
	// OpenAI template
	openaiTemplate := `
job "llm-openai-{{.JobID}}" {
  datacenters = ["dc1"]
  type = "batch"
  priority = 75
  
  group "transform" {
    count = 1
    
    ephemeral_disk {
      size = 1024
    }
    
    task "openai-transform" {
      driver = "docker"
      
      config {
        image = "python:3.11-slim"
        volumes = ["local:/workspace"]
        network_mode = "host"
      }
      
      env {
        JOB_ID = "{{.JobID}}"
        MODEL = "{{.Model}}"
        PROMPT = "{{.PromptBase64}}"
        INPUT_URL = "{{.InputURL}}"
        OUTPUT_URL = "{{.OutputURL}}"
        CONSUL_HTTP_ADDR = "{{.ConsulAddr}}"
        OPENAI_API_KEY = "{{.APIKey}}"
        TEMPERATURE = "{{.Temperature}}"
        MAX_TOKENS = "{{.MaxTokens}}"
      }
      
      template {
        data = <<EOF
#!/bin/bash
set -e

# Install OpenAI SDK
pip install --no-cache-dir openai requests

# Download input
wget -q -O /workspace/input.tar.gz "$INPUT_URL"
cd /workspace
tar -xzf input.tar.gz

# Python script for OpenAI transformation
cat > transform.py <<'PYTHON'
import os
import base64
import json
import tarfile
from openai import OpenAI

# Initialize OpenAI client
client = OpenAI(api_key=os.environ['OPENAI_API_KEY'])

# Decode prompt
prompt = base64.b64decode(os.environ['PROMPT']).decode('utf-8')

# Read code files
code_files = []
for root, dirs, files in os.walk('.'):
    for file in files:
        if file.endswith('.{{.Language}}'):
            with open(os.path.join(root, file), 'r') as f:
                code_files.append(f.read()[:2000])  # Limit per file

code_context = '\n'.join(code_files[:5])  # Limit number of files

# Create messages
messages = [
    {"role": "system", "content": "You are a code transformation assistant."},
    {"role": "user", "content": f"{prompt}\n\nCode to transform:\n{code_context}"}
]

# Call OpenAI API
response = client.chat.completions.create(
    model=os.environ['MODEL'],
    messages=messages,
    temperature=float(os.environ['TEMPERATURE']),
    max_tokens=int(os.environ['MAX_TOKENS'])
)

# Save result
with open('transformed.txt', 'w') as f:
    f.write(response.choices[0].message.content)

# Create output archive
with tarfile.open('output.tar.gz', 'w:gz') as tar:
    tar.add('transformed.txt')
PYTHON

python transform.py

# Upload result
curl -X PUT "$OUTPUT_URL" --data-binary @output.tar.gz

# Update Consul
consul kv put "ploy/llm/jobs/$JOB_ID/status" "completed"
consul kv put "ploy/llm/jobs/$JOB_ID/completed_at" "$(date -u +%Y-%m-%dT%H:%M:%SZ)"
EOF
        destination = "local/run.sh"
        perms = "0755"
      }
      
      config {
        command = "/bin/bash"
        args = ["local/run.sh"]
      }
      
      resources {
        cpu = 500
        memory = 1024
      }
      
      kill_timeout = "30s"
    }
  }
  
  reschedule {
    attempts = 2
    interval = "5m"
    delay = "30s"
    unlimited = false
  }
}
`

	// Parse templates - only external API providers
	templates := map[string]string{
		"openai": openaiTemplate,
		// Additional external providers can be added here
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

// SubmitLLMTransformation submits a new LLM transformation job
func (d *LLMDispatcher) SubmitLLMTransformation(ctx context.Context, provider, model, prompt string, inputTar io.Reader, params map[string]interface{}) (*LLMJob, error) {
	jobID := uuid.New().String()

	// Upload input to storage
	inputURL, err := d.uploadInput(ctx, jobID, inputTar)
	if err != nil {
		return nil, fmt.Errorf("failed to upload input: %w", err)
	}

	// Prepare output URL
	outputURL := fmt.Sprintf("%s/ploy-artifacts/llm/outputs/%s.tar.gz", d.storageBaseURL, jobID)

	// Extract parameters
	temperature := 0.1
	if t, ok := params["temperature"].(float64); ok {
		temperature = t
	}
	
	maxTokens := 2048
	if mt, ok := params["max_tokens"].(int); ok {
		maxTokens = mt
	}

	language := "java"
	if l, ok := params["language"].(string); ok {
		language = l
	}

	// Create job record
	job := &LLMJob{
		ID:          jobID,
		Provider:    provider,
		Model:       model,
		Prompt:      prompt,
		InputURL:    inputURL,
		OutputURL:   outputURL,
		Language:    language,
		Status:      "pending",
		CreatedAt:   time.Now(),
		Temperature: temperature,
		MaxTokens:   maxTokens,
	}

	// Store job in Consul KV
	if err := d.storeJob(job); err != nil {
		return nil, fmt.Errorf("failed to store job: %w", err)
	}

	// Submit to Nomad
	if err := d.submitToNomad(job, params); err != nil {
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
func (d *LLMDispatcher) GetJob(ctx context.Context, jobID string) (*LLMJob, error) {
	kv := d.consulClient.KV()

	// Get job data
	pair, _, err := kv.Get(fmt.Sprintf("ploy/llm/jobs/%s", jobID), nil)
	if err != nil {
		return nil, fmt.Errorf("failed to get job from Consul: %w", err)
	}
	if pair == nil {
		return nil, fmt.Errorf("job not found: %s", jobID)
	}

	var job LLMJob
	if err := json.Unmarshal(pair.Value, &job); err != nil {
		return nil, fmt.Errorf("failed to unmarshal job: %w", err)
	}

	// Check for completion
	statusPair, _, _ := kv.Get(fmt.Sprintf("ploy/llm/jobs/%s/status", jobID), nil)
	if statusPair != nil {
		job.Status = string(statusPair.Value)
	}

	// Get result if completed
	if job.Status == "completed" {
		resultPair, _, _ := kv.Get(fmt.Sprintf("ploy/llm/jobs/%s/result", jobID), nil)
		if resultPair != nil {
			var result map[string]interface{}
			if err := json.Unmarshal(resultPair.Value, &result); err == nil {
				job.Result = result
			}
		}
	}

	return &job, nil
}

// WaitForCompletion waits for a job to complete or timeout
func (d *LLMDispatcher) WaitForCompletion(ctx context.Context, jobID string, timeout time.Duration) (*LLMJob, error) {
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

// uploadInput uploads the input tar to storage
func (d *LLMDispatcher) uploadInput(ctx context.Context, jobID string, inputTar io.Reader) (string, error) {
	bucket := "ploy-artifacts"
	key := fmt.Sprintf("llm/inputs/%s.tar.gz", jobID)

	// Read input into buffer
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
func (d *LLMDispatcher) storeJob(job *LLMJob) error {
	kv := d.consulClient.KV()

	data, err := json.Marshal(job)
	if err != nil {
		return fmt.Errorf("failed to marshal job: %w", err)
	}

	p := &consulapi.KVPair{
		Key:   fmt.Sprintf("ploy/llm/jobs/%s", job.ID),
		Value: data,
	}

	_, err = kv.Put(p, nil)
	return err
}

// submitToNomad submits the job to Nomad
func (d *LLMDispatcher) submitToNomad(job *LLMJob, params map[string]interface{}) error {
	// Get the appropriate template
	tmpl, ok := d.jobTemplates[job.Provider]
	if !ok {
		return fmt.Errorf("no template for provider: %s", job.Provider)
	}

	// Generate HCL from template
	var buf bytes.Buffer

	// Get Consul address
	consulAddr := os.Getenv("CONSUL_HTTP_ADDR")
	if consulAddr == "" {
		consulAddr = "http://localhost:8500"
	}

	// Encode prompt as base64 to avoid escaping issues
	promptBase64 := base64.StdEncoding.EncodeToString([]byte(job.Prompt))

	// Get API key for OpenAI
	apiKey := ""
	if job.Provider == "openai" {
		apiKey = os.Getenv("OPENAI_API_KEY")
		if apiKey == "" {
			if key, ok := params["api_key"].(string); ok {
				apiKey = key
			}
		}
	}

	templateParams := map[string]string{
		"JobID":       job.ID,
		"Model":       job.Model,
		"PromptBase64": promptBase64,
		"InputURL":    job.InputURL,
		"OutputURL":   job.OutputURL,
		"ConsulAddr":  consulAddr,
		"Temperature": fmt.Sprintf("%f", job.Temperature),
		"MaxTokens":   fmt.Sprintf("%d", job.MaxTokens),
		"Language":    job.Language,
		"APIKey":      apiKey,
	}

	err := tmpl.Execute(&buf, templateParams)
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

// CleanupOldJobs removes completed jobs older than specified duration
func (d *LLMDispatcher) CleanupOldJobs(ctx context.Context, maxAge time.Duration) error {
	kv := d.consulClient.KV()
	
	// List all job keys
	keys, _, err := kv.Keys("ploy/llm/jobs/", "/", nil)
	if err != nil {
		return fmt.Errorf("failed to list jobs: %w", err)
	}

	cutoff := time.Now().Add(-maxAge)

	for _, key := range keys {
		// Skip sub-keys
		if key == "ploy/llm/jobs/" {
			continue
		}

		// Extract job ID
		jobID := key[len("ploy/llm/jobs/"):]
		if jobID == "" {
			continue
		}

		job, err := d.GetJob(ctx, jobID)
		if err != nil {
			continue
		}

		if job.Status == "completed" || job.Status == "failed" {
			if job.CompletedAt != nil && job.CompletedAt.Before(cutoff) {
				// Delete from Consul
				_, err := kv.Delete(fmt.Sprintf("ploy/llm/jobs/%s", job.ID), nil)
				if err != nil {
					return fmt.Errorf("failed to delete job %s: %w", job.ID, err)
				}

				// Stop Nomad job if still exists
				jobName := fmt.Sprintf("llm-%s-%s", job.Provider, job.ID)
				_, _, _ = d.nomadClient.Jobs().Deregister(jobName, false, nil)
			}
		}
	}

	return nil
}