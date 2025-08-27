package integration

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TransformRequest represents a transformation request
type TransformRequest struct {
	JobID        string       `json:"job_id"`
	TarArchive   string       `json:"tar_archive"`
	RecipeConfig RecipeConfig `json:"recipe_config"`
}

// RecipeConfig represents OpenRewrite recipe configuration
type RecipeConfig struct {
	Recipe    string `json:"recipe"`
	Artifacts string `json:"artifacts,omitempty"`
}

// TransformationResult represents the result of an OpenRewrite transformation
type TransformationResult struct {
	RecipeID        string                 `json:"recipe_id"`
	Success         bool                   `json:"success"`
	ChangesApplied  int                    `json:"changes_applied"`
	FilesModified   []string               `json:"files_modified"`
	Diff            string                 `json:"diff"`
	ValidationScore float64                `json:"validation_score"`
	ExecutionTime   time.Duration          `json:"execution_time"`
	Errors          []TransformationError  `json:"errors,omitempty"`
	Warnings        []TransformationError  `json:"warnings,omitempty"`
	Metadata        map[string]interface{} `json:"metadata"`
}

// TransformationError represents an error during transformation
type TransformationError struct {
	Type    string `json:"type"`
	Message string `json:"message"`
	File    string `json:"file,omitempty"`
	Line    int    `json:"line,omitempty"`
}

// CreateJobRequest represents a request to create an asynchronous job
type CreateJobRequest struct {
	TarArchive   string       `json:"tar_archive"`
	RecipeConfig RecipeConfig `json:"recipe_config"`
}

// JobStatus represents job status information
type JobStatus struct {
	JobID     string `json:"job_id"`
	Status    string `json:"status"`
	Progress  int    `json:"progress"`
	StartTime string `json:"start_time"`
	EndTime   string `json:"end_time,omitempty"`
	Error     string `json:"error,omitempty"`
}

func getServiceURL() string {
	serviceURL := os.Getenv("OPENREWRITE_SERVICE_URL")
	if serviceURL == "" {
		serviceURL = "http://localhost:8090"
	}
	return serviceURL
}

func TestHealthEndpoint(t *testing.T) {
	serviceURL := getServiceURL()
	
	resp, err := http.Get(serviceURL + "/v1/openrewrite/health")
	require.NoError(t, err)
	defer resp.Body.Close()
	
	assert.Equal(t, http.StatusOK, resp.StatusCode)
	
	var health map[string]interface{}
	err = json.NewDecoder(resp.Body).Decode(&health)
	require.NoError(t, err)
	
	assert.Equal(t, "healthy", health["status"])
	assert.Equal(t, "1.0.0", health["version"])
}

func TestReadinessEndpoint(t *testing.T) {
	serviceURL := getServiceURL()
	
	resp, err := http.Get(serviceURL + "/v1/openrewrite/ready")
	require.NoError(t, err)
	defer resp.Body.Close()
	
	assert.Equal(t, http.StatusOK, resp.StatusCode)
	
	var ready map[string]interface{}
	err = json.NewDecoder(resp.Body).Decode(&ready)
	require.NoError(t, err)
	
	assert.Equal(t, true, ready["ready"])
}

func TestSynchronousJava11to17Migration(t *testing.T) {
	serviceURL := getServiceURL()
	
	// Create test tar archive
	tarData := createTestMavenProject(t)
	
	// Send synchronous transformation request
	req := TransformRequest{
		JobID:      "test-sync-migration",
		TarArchive: base64.StdEncoding.EncodeToString(tarData),
		RecipeConfig: RecipeConfig{
			Recipe:    "org.openrewrite.java.migrate.Java11toJava17",
			Artifacts: "org.openrewrite.recipe:rewrite-migrate-java:2.25.0",
		},
	}
	
	body, err := json.Marshal(req)
	require.NoError(t, err)
	
	resp, err := http.Post(
		serviceURL+"/v1/openrewrite/transform",
		"application/json",
		bytes.NewReader(body),
	)
	require.NoError(t, err)
	defer resp.Body.Close()
	
	// Should return 200 OK for synchronous requests
	assert.Equal(t, http.StatusOK, resp.StatusCode)
	
	var result TransformationResult
	err = json.NewDecoder(resp.Body).Decode(&result)
	require.NoError(t, err)
	
	// Verify transformation results
	assert.Equal(t, "org.openrewrite.java.migrate.Java11toJava17", result.RecipeID)
	assert.True(t, result.Success, "Transformation should succeed")
	assert.NotEmpty(t, result.Diff, "Should have generated diff")
	assert.GreaterOrEqual(t, result.ValidationScore, 0.0)
	assert.Greater(t, result.ExecutionTime, time.Duration(0))
	
	// Should have modified pom.xml for Java version upgrade
	assert.Contains(t, result.FilesModified, "pom.xml")
	assert.Contains(t, result.Diff, "17", "Diff should contain Java version 17")
}

func TestAsynchronousJobWorkflow(t *testing.T) {
	serviceURL := getServiceURL()
	
	// Create test tar archive
	tarData := createTestMavenProject(t)
	
	// Create asynchronous job
	createReq := CreateJobRequest{
		TarArchive: base64.StdEncoding.EncodeToString(tarData),
		RecipeConfig: RecipeConfig{
			Recipe:    "org.openrewrite.java.migrate.Java11toJava17",
			Artifacts: "org.openrewrite.recipe:rewrite-migrate-java:2.25.0",
		},
	}
	
	body, err := json.Marshal(createReq)
	require.NoError(t, err)
	
	resp, err := http.Post(
		serviceURL+"/v1/openrewrite/jobs",
		"application/json",
		bytes.NewReader(body),
	)
	require.NoError(t, err)
	defer resp.Body.Close()
	
	assert.Equal(t, http.StatusCreated, resp.StatusCode)
	
	var jobResp map[string]string
	err = json.NewDecoder(resp.Body).Decode(&jobResp)
	require.NoError(t, err)
	
	jobID, exists := jobResp["job_id"]
	require.True(t, exists, "Job ID should be returned")
	require.NotEmpty(t, jobID, "Job ID should not be empty")
	
	// Poll job status until completion (with timeout)
	timeout := time.After(2 * time.Minute) // 2 minute timeout for job completion
	ticker := time.Tick(2 * time.Second)   // Check every 2 seconds
	
	var finalStatus JobStatus
	for {
		select {
		case <-timeout:
			t.Fatal("Job did not complete within timeout")
		case <-ticker:
			// Check job status
			statusResp, err := http.Get(serviceURL + "/v1/openrewrite/jobs/" + jobID + "/status")
			require.NoError(t, err)
			
			err = json.NewDecoder(statusResp.Body).Decode(&finalStatus)
			statusResp.Body.Close()
			require.NoError(t, err)
			
			t.Logf("Job %s status: %s (progress: %d%%)", jobID, finalStatus.Status, finalStatus.Progress)
			
			if finalStatus.Status == "completed" || finalStatus.Status == "failed" {
				goto jobComplete
			}
		}
	}
	
jobComplete:
	// Verify job completed successfully
	assert.Equal(t, "completed", finalStatus.Status, "Job should complete successfully")
	assert.Equal(t, 100, finalStatus.Progress, "Progress should be 100%")
	assert.NotEmpty(t, finalStatus.StartTime, "Should have start time")
	assert.NotEmpty(t, finalStatus.EndTime, "Should have end time")
	
	if finalStatus.Status == "failed" {
		t.Logf("Job failed with error: %s", finalStatus.Error)
		t.FailNow()
	}
	
	// Get job diff
	diffResp, err := http.Get(serviceURL + "/v1/openrewrite/jobs/" + jobID + "/diff")
	require.NoError(t, err)
	defer diffResp.Body.Close()
	
	assert.Equal(t, http.StatusOK, diffResp.StatusCode)
	
	diffData, err := json.Marshal(diffResp.Body)
	require.NoError(t, err)
	
	assert.NotEmpty(t, diffData, "Should have diff data")
}

func TestMetricsEndpoint(t *testing.T) {
	serviceURL := getServiceURL()
	
	resp, err := http.Get(serviceURL + "/v1/openrewrite/metrics")
	require.NoError(t, err)
	defer resp.Body.Close()
	
	assert.Equal(t, http.StatusOK, resp.StatusCode)
	
	var metrics map[string]interface{}
	err = json.NewDecoder(resp.Body).Decode(&metrics)
	require.NoError(t, err)
	
	// Should have basic metrics structure
	assert.Contains(t, metrics, "total_jobs")
	assert.Contains(t, metrics, "job_status")
	assert.Contains(t, metrics, "service_info")
}

func TestInvalidRequestHandling(t *testing.T) {
	serviceURL := getServiceURL()
	
	// Test invalid transform request (missing required fields)
	invalidReq := map[string]interface{}{
		"job_id": "test-invalid",
		// Missing tar_archive and recipe_config
	}
	
	body, err := json.Marshal(invalidReq)
	require.NoError(t, err)
	
	resp, err := http.Post(
		serviceURL+"/v1/openrewrite/transform",
		"application/json",
		bytes.NewReader(body),
	)
	require.NoError(t, err)
	defer resp.Body.Close()
	
	// Should return 400 Bad Request for invalid request
	assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
	
	var errorResp map[string]string
	err = json.NewDecoder(resp.Body).Decode(&errorResp)
	require.NoError(t, err)
	
	assert.Contains(t, errorResp, "error")
	assert.NotEmpty(t, errorResp["error"])
}

// Helper function to create a test Maven project
func createTestMavenProject(t *testing.T) []byte {
	var buf bytes.Buffer
	gzWriter := gzip.NewWriter(&buf)
	tarWriter := tar.NewWriter(gzWriter)
	
	// Create a simple pom.xml for Java 11 project
	pomXML := `<?xml version="1.0" encoding="UTF-8"?>
<project xmlns="http://maven.apache.org/POM/4.0.0"
         xmlns:xsi="http://www.w3.org/2001/XMLSchema-instance"
         xsi:schemaLocation="http://maven.apache.org/POM/4.0.0 http://maven.apache.org/xsd/maven-4.0.0.xsd">
    <modelVersion>4.0.0</modelVersion>
    
    <groupId>com.example</groupId>
    <artifactId>test-project</artifactId>
    <version>1.0.0</version>
    
    <properties>
        <maven.compiler.source>11</maven.compiler.source>
        <maven.compiler.target>11</maven.compiler.target>
        <java.version>11</java.version>
    </properties>
    
    <dependencies>
        <dependency>
            <groupId>junit</groupId>
            <artifactId>junit</artifactId>
            <version>4.13.2</version>
            <scope>test</scope>
        </dependency>
    </dependencies>
    
    <build>
        <plugins>
            <plugin>
                <groupId>org.apache.maven.plugins</groupId>
                <artifactId>maven-compiler-plugin</artifactId>
                <version>3.8.1</version>
                <configuration>
                    <source>11</source>
                    <target>11</target>
                </configuration>
            </plugin>
        </plugins>
    </build>
</project>`
	
	// Add pom.xml to tar
	pomHeader := &tar.Header{
		Name: "pom.xml",
		Mode: 0644,
		Size: int64(len(pomXML)),
	}
	err := tarWriter.WriteHeader(pomHeader)
	require.NoError(t, err)
	_, err = tarWriter.Write([]byte(pomXML))
	require.NoError(t, err)
	
	// Create a simple Java file
	javaCode := `package com.example;

import java.util.List;
import java.util.ArrayList;

public class TestClass {
    public static void main(String[] args) {
        List<String> items = new ArrayList<>();
        items.add("Java 11 code");
        System.out.println("Hello from " + items.get(0));
    }
}`
	
	// Create src/main/java directory structure
	javaHeader := &tar.Header{
		Name: "src/main/java/com/example/TestClass.java",
		Mode: 0644,
		Size: int64(len(javaCode)),
	}
	err = tarWriter.WriteHeader(javaHeader)
	require.NoError(t, err)
	_, err = tarWriter.Write([]byte(javaCode))
	require.NoError(t, err)
	
	// Close tar and gzip writers
	tarWriter.Close()
	gzWriter.Close()
	
	return buf.Bytes()
}