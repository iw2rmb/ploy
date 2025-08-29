package arf

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"text/template"
	"time"

	"github.com/google/uuid"
	consulapi "github.com/hashicorp/consul/api"
	nomadapi "github.com/hashicorp/nomad/api"
	"github.com/iw2rmb/ploy/internal/storage"
)

// OpenRewriteJob represents a transformation job
type OpenRewriteJob struct {
	ID         string                 `json:"id"`
	Recipe     string                 `json:"recipe"`
	InputURL   string                 `json:"input_url"`
	OutputURL  string                 `json:"output_url"`
	Status     string                 `json:"status"` // pending, running, completed, failed
	CreatedAt  time.Time              `json:"created_at"`
	StartedAt  *time.Time             `json:"started_at"`
	CompletedAt *time.Time            `json:"completed_at"`
	Result     map[string]interface{} `json:"result,omitempty"`
	Error      string                 `json:"error,omitempty"`
}

// OpenRewriteDispatcher handles job dispatch and monitoring
type OpenRewriteDispatcher struct {
	nomadClient   *nomadapi.Client
	consulClient  *consulapi.Client
	storageClient *storage.StorageClient
	jobTemplate   *template.Template
	storageBaseURL string
}

// NewOpenRewriteDispatcher creates a new dispatcher
func NewOpenRewriteDispatcher(nomadAddr, consulAddr string, storageClient *storage.StorageClient) (*OpenRewriteDispatcher, error) {
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

	// Load job template for JVM-based OpenRewrite with SeaweedFS caching
	jobTemplateContent := `
job "openrewrite-{{.JobID}}" {
  datacenters = ["dc1"]
  type = "batch"
  priority = 80
  
  group "transform" {
    count = 1
    
    ephemeral_disk {
      size = 2048  # Increased for Maven cache
    }
    
    task "openrewrite" {
      driver = "docker"
      
      config {
        image = "registry.dev.ployman.app/openrewrite-jvm:latest"
        volumes = ["local:/workspace"]
      }
      
      env {
        JOB_ID = "{{.JobID}}"
        RECIPE = "{{.RecipeClass}}"
        RECIPE_GROUP = "{{.RecipeGroup}}"
        RECIPE_ARTIFACT = "{{.RecipeArtifact}}"
        RECIPE_VERSION = "{{.RecipeVersion}}"
        INPUT_URL = "{{.InputURL}}"
        OUTPUT_URL = "{{.OutputURL}}"
        CONSUL_HTTP_ADDR = "{{.ConsulAddr}}"
        SEAWEEDFS_URL = "http://seaweedfs.service.consul:8888"
        MAVEN_CACHE_PATH = "maven-repository"
      }
      
      template {
        data = <<EOF
#!/bin/bash
set -e

echo "[Job] Starting OpenRewrite transformation job {{.JobID}}"
echo "[Job] Recipe: {{.RecipeClass}}"

# Download input from SeaweedFS
echo "[Job] Downloading input from {{.InputURL}}..."
wget -q -O /workspace/input.tar "{{.InputURL}}" || {
  echo "[Job] Failed to download input"
  consul kv put "ploy/openrewrite/jobs/{{.JobID}}/status" "failed"
  consul kv put "ploy/openrewrite/jobs/{{.JobID}}/error" "Failed to download input archive"
  exit 1
}

# Run OpenRewrite transformation with SeaweedFS caching
echo "[Job] Running transformation..."
/usr/local/bin/openrewrite /workspace/input.tar /workspace/output.tar "{{.RecipeClass}}"
TRANSFORM_EXIT=$?

if [ $TRANSFORM_EXIT -eq 0 ]; then
  # Upload output to SeaweedFS
  echo "[Job] Uploading output to {{.OutputURL}}..."
  curl -X PUT "{{.OutputURL}}" --data-binary @/workspace/output.tar || {
    echo "[Job] Failed to upload output"
    consul kv put "ploy/openrewrite/jobs/{{.JobID}}/status" "failed"
    consul kv put "ploy/openrewrite/jobs/{{.JobID}}/error" "Failed to upload output archive"
    exit 1
  }
  
  # Update job status in Consul
  consul kv put "ploy/openrewrite/jobs/{{.JobID}}/status" "completed"
  consul kv put "ploy/openrewrite/jobs/{{.JobID}}/completed_at" "$(date -u +%Y-%m-%dT%H:%M:%SZ)"
  
  # Store transformation report if available
  if [ -f /workspace/transformation-report.json ]; then
    consul kv put "ploy/openrewrite/jobs/{{.JobID}}/result" "$(cat /workspace/transformation-report.json)"
  fi
  
  echo "[Job] Transformation completed successfully"
else
  consul kv put "ploy/openrewrite/jobs/{{.JobID}}/status" "failed"
  consul kv put "ploy/openrewrite/jobs/{{.JobID}}/error" "Transformation failed with exit code $TRANSFORM_EXIT"
  echo "[Job] Transformation failed"
  exit $TRANSFORM_EXIT
fi
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
        memory = 256
      }
      
      kill_timeout = "30s"
    }
  }
  
  reschedule {
    attempts = 3
    interval = "1m"
    delay = "10s"
    unlimited = false
  }
}
`

	tmpl, err := template.New("job").Parse(jobTemplateContent)
	if err != nil {
		return nil, fmt.Errorf("failed to parse job template: %w", err)
	}

	// Get storage base URL from environment
	storageBaseURL := "http://seaweedfs.service.consul:8888"
	if url := os.Getenv("SEAWEEDFS_URL"); url != "" {
		storageBaseURL = url
	}
	
	return &OpenRewriteDispatcher{
		nomadClient:    nomadClient,
		consulClient:   consulClient,
		storageClient:  storageClient,
		jobTemplate:    tmpl,
		storageBaseURL: storageBaseURL,
	}, nil
}

// SubmitJob submits a new OpenRewrite transformation job
func (d *OpenRewriteDispatcher) SubmitJob(ctx context.Context, recipe string, inputTar io.Reader) (*OpenRewriteJob, error) {
	jobID := uuid.New().String()
	
	// Upload input to storage
	inputURL, err := d.uploadInput(ctx, jobID, inputTar)
	if err != nil {
		return nil, fmt.Errorf("failed to upload input: %w", err)
	}
	
	// Prepare output URL
	outputURL := fmt.Sprintf("%s/openrewrite/outputs/%s.tar", d.storageBaseURL, jobID)
	
	// Create job record
	job := &OpenRewriteJob{
		ID:        jobID,
		Recipe:    recipe,
		InputURL:  inputURL,
		OutputURL: outputURL,
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
func (d *OpenRewriteDispatcher) GetJob(ctx context.Context, jobID string) (*OpenRewriteJob, error) {
	kv := d.consulClient.KV()
	
	// Get job data
	pair, _, err := kv.Get(fmt.Sprintf("ploy/openrewrite/jobs/%s", jobID), nil)
	if err != nil {
		return nil, fmt.Errorf("failed to get job from Consul: %w", err)
	}
	if pair == nil {
		return nil, fmt.Errorf("job not found: %s", jobID)
	}
	
	var job OpenRewriteJob
	if err := json.Unmarshal(pair.Value, &job); err != nil {
		return nil, fmt.Errorf("failed to unmarshal job: %w", err)
	}
	
	// Check for completion
	statusPair, _, _ := kv.Get(fmt.Sprintf("ploy/openrewrite/jobs/%s/status", jobID), nil)
	if statusPair != nil {
		job.Status = string(statusPair.Value)
	}
	
	// Get result if completed
	if job.Status == "completed" {
		resultPair, _, _ := kv.Get(fmt.Sprintf("ploy/openrewrite/jobs/%s/result", jobID), nil)
		if resultPair != nil {
			var result map[string]interface{}
			if err := json.Unmarshal(resultPair.Value, &result); err == nil {
				job.Result = result
			}
		}
	}
	
	return &job, nil
}

// ListJobs lists all jobs from Consul
func (d *OpenRewriteDispatcher) ListJobs(ctx context.Context, limit int) ([]*OpenRewriteJob, error) {
	kv := d.consulClient.KV()
	
	// List all job keys
	keys, _, err := kv.Keys("ploy/openrewrite/jobs/", "/", nil)
	if err != nil {
		return nil, fmt.Errorf("failed to list jobs: %w", err)
	}
	
	jobs := make([]*OpenRewriteJob, 0)
	for i, key := range keys {
		if i >= limit && limit > 0 {
			break
		}
		
		// Skip sub-keys
		if key == "ploy/openrewrite/jobs/" {
			continue
		}
		
		// Extract job ID
		jobID := key[len("ploy/openrewrite/jobs/"):]
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

// GetQueueDepth returns the number of pending jobs
func (d *OpenRewriteDispatcher) GetQueueDepth(ctx context.Context) (int, error) {
	jobs, err := d.ListJobs(ctx, 0)
	if err != nil {
		return 0, err
	}
	
	pending := 0
	for _, job := range jobs {
		if job.Status == "pending" || job.Status == "submitted" {
			pending++
		}
	}
	
	return pending, nil
}

// uploadInput uploads the input tar to storage
func (d *OpenRewriteDispatcher) uploadInput(ctx context.Context, jobID string, inputTar io.Reader) (string, error) {
	// Upload to SeaweedFS or other storage
	bucket := "ploy-artifacts"
	key := fmt.Sprintf("openrewrite/inputs/%s.tar", jobID)
	
	// Read input into buffer for ReadSeeker
	buf := new(bytes.Buffer)
	if _, err := io.Copy(buf, inputTar); err != nil {
		return "", fmt.Errorf("failed to read input: %w", err)
	}
	
	// Upload to storage using PutObject method
	reader := bytes.NewReader(buf.Bytes())
	_, err := d.storageClient.PutObject(bucket, key, reader, "application/x-tar")
	if err != nil {
		return "", fmt.Errorf("failed to upload to storage: %w", err)
	}
	
	return fmt.Sprintf("%s/%s/%s", d.storageBaseURL, bucket, key), nil
}

// storeJob stores job in Consul KV
func (d *OpenRewriteDispatcher) storeJob(job *OpenRewriteJob) error {
	kv := d.consulClient.KV()
	
	data, err := json.Marshal(job)
	if err != nil {
		return fmt.Errorf("failed to marshal job: %w", err)
	}
	
	p := &consulapi.KVPair{
		Key:   fmt.Sprintf("ploy/openrewrite/jobs/%s", job.ID),
		Value: data,
	}
	
	_, err = kv.Put(p, nil)
	return err
}

// submitToNomad submits the job to Nomad
func (d *OpenRewriteDispatcher) submitToNomad(job *OpenRewriteJob) error {
	// Check if Docker image exists before submitting job
	if err := d.checkImageExists(); err != nil {
		// Log the specific registry URL being checked for debugging
		fmt.Printf("[OpenRewrite] Image check failed: %v\n", err)
		return err
	}
	
	// Parse recipe information
	recipeClass := job.Recipe
	recipeGroup := "org.openrewrite.recipe"
	recipeArtifact := "rewrite-migrate-java"
	recipeVersion := "2.11.0"
	
	// Map common recipe names to their coordinates
	recipeMap := map[string]struct{
		group    string
		artifact string
		version  string
		class    string
	}{
		"java11to17": {
			group:    "org.openrewrite.recipe",
			artifact: "rewrite-migrate-java",
			version:  "2.11.0",
			class:    "org.openrewrite.java.migrate.Java11toJava17",
		},
		"java8to11": {
			group:    "org.openrewrite.recipe",
			artifact: "rewrite-migrate-java",
			version:  "2.11.0",
			class:    "org.openrewrite.java.migrate.Java8toJava11",
		},
		"spring-boot-3": {
			group:    "org.openrewrite.recipe",
			artifact: "rewrite-spring",
			version:  "5.7.0",
			class:    "org.openrewrite.java.spring.boot3.UpgradeSpringBoot_3_2",
		},
		"junit5": {
			group:    "org.openrewrite.recipe",
			artifact: "rewrite-testing-frameworks",
			version:  "2.6.0",
			class:    "org.openrewrite.java.testing.junit5.JUnit4to5Migration",
		},
	}
	
	// Check if we have a mapping for this recipe
	if mapping, ok := recipeMap[job.Recipe]; ok {
		recipeGroup = mapping.group
		recipeArtifact = mapping.artifact
		recipeVersion = mapping.version
		recipeClass = mapping.class
	} else if strings.Contains(job.Recipe, ".") {
		// It's already a full class name
		recipeClass = job.Recipe
		// Try to guess the artifact from the class name
		if strings.Contains(recipeClass, "spring") {
			recipeArtifact = "rewrite-spring"
			recipeVersion = "5.7.0"
		} else if strings.Contains(recipeClass, "testing") || strings.Contains(recipeClass, "junit") {
			recipeArtifact = "rewrite-testing-frameworks"
			recipeVersion = "2.6.0"
		}
	}
	
	// Generate HCL from template
	var buf bytes.Buffer
	
	// Get Consul address from environment or default
	consulAddr := os.Getenv("CONSUL_HTTP_ADDR")
	if consulAddr == "" {
		consulAddr = "http://localhost:8500"
	}
	
	err := d.jobTemplate.Execute(&buf, map[string]string{
		"JobID":         job.ID,
		"RecipeClass":   recipeClass,
		"RecipeGroup":   recipeGroup,
		"RecipeArtifact": recipeArtifact,
		"RecipeVersion": recipeVersion,
		"InputURL":      job.InputURL,
		"OutputURL":     job.OutputURL,
		"ConsulAddr":    consulAddr,
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

// SubmitTransformation submits a transformation job with given parameters
func (d *OpenRewriteDispatcher) SubmitTransformation(jobID string, params map[string]string) error {
	// Create job object
	job := &OpenRewriteJob{
		ID:        jobID,
		Recipe:    params["recipes"],
		InputURL:  params["project_url"],
		Status:    "pending",
		CreatedAt: time.Now(),
	}
	
	// Store job in Consul
	if err := d.storeJob(job); err != nil {
		return fmt.Errorf("failed to store job: %w", err)
	}
	
	// Submit to Nomad
	if err := d.submitToNomad(job); err != nil {
		job.Status = "failed"
		job.Error = err.Error()
		_ = d.storeJob(job)
		return fmt.Errorf("failed to submit to Nomad: %w", err)
	}
	
	return nil
}

// GetJobStatus retrieves the status of a job
func (d *OpenRewriteDispatcher) GetJobStatus(jobID string) (map[string]interface{}, error) {
	job, err := d.GetJob(context.Background(), jobID)
	if err != nil {
		return nil, err
	}
	
	// Convert to status response
	status := map[string]interface{}{
		"job_id":     job.ID,
		"status":     job.Status,
		"created_at": job.CreatedAt,
		"recipe":     job.Recipe,
	}
	
	if job.StartedAt != nil {
		status["started_at"] = job.StartedAt
	}
	if job.CompletedAt != nil {
		status["completed_at"] = job.CompletedAt
	}
	if job.Error != "" {
		status["error"] = job.Error
	}
	if job.Result != nil {
		status["result"] = job.Result
	}
	
	return status, nil
}

// CleanupOldJobs removes completed jobs older than specified duration
func (d *OpenRewriteDispatcher) CleanupOldJobs(ctx context.Context, maxAge time.Duration) error {
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
				_, err := kv.Delete(fmt.Sprintf("ploy/openrewrite/jobs/%s", job.ID), nil)
				if err != nil {
					return fmt.Errorf("failed to delete job %s: %w", job.ID, err)
				}
				
				// Stop Nomad job if still exists
				_, _, _ = d.nomadClient.Jobs().Deregister(fmt.Sprintf("openrewrite-%s", job.ID), false, nil)
			}
		}
	}
	
	return nil
}

// checkImageExists verifies if the OpenRewrite Docker image exists in the registry
func (d *OpenRewriteDispatcher) checkImageExists() error {
	registryURL := "https://registry.dev.ployman.app"
	imageName := "openrewrite-native"
	tag := "latest"
	
	// Docker Registry v2 API endpoint to check if manifest exists
	manifestURL := fmt.Sprintf("%s/v2/%s/manifests/%s", registryURL, imageName, tag)
	
	// Create HTTP client with timeout
	client := &http.Client{
		Timeout: 10 * time.Second,
	}
	
	// Create request
	req, err := http.NewRequest("GET", manifestURL, nil)
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}
	
	// Docker Registry v2 requires Accept header for manifest
	req.Header.Set("Accept", "application/vnd.docker.distribution.manifest.v2+json")
	
	// Make request
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("failed to check image: %w", err)
	}
	defer resp.Body.Close()
	
	// Check response status
	switch resp.StatusCode {
	case http.StatusOK:
		// Image exists
		return nil
	case http.StatusNotFound:
		// Image doesn't exist
		return fmt.Errorf("OpenRewrite Docker image not found at %s/%s:%s. Please deploy the image first by running: ansible-playbook playbooks/openrewrite-native.yml -e target_host=$TARGET_HOST", 
			registryURL, imageName, tag)
	case http.StatusUnauthorized:
		// Registry requires authentication (shouldn't happen for our anonymous registry)
		return fmt.Errorf("registry authentication required")
	default:
		// Other error
		return fmt.Errorf("unexpected registry response: %d", resp.StatusCode)
	}
}