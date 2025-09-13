//go:build arf_legacy
// +build arf_legacy

package arf

import (
	"bytes"
	"encoding/json"
	"io/ioutil"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/gofiber/fiber/v2"
)

// TestPhase4Integration_SecurityWorkflow tests the complete security workflow
func TestPhase4Integration_SecurityWorkflow(t *testing.T) {
	// Setup test environment
	app := fiber.New()
	handler := NewHandler(nil, nil)
	handler.RegisterRoutes(app)

	// Create test SBOM file
	tempDir := t.TempDir()
	sbomPath := filepath.Join(tempDir, "test-app.sbom.json")
	sbomData := map[string]interface{}{
		"artifacts": []interface{}{
			map[string]interface{}{
				"name":     "vulnerable-package",
				"version":  "1.0.0",
				"type":     "npm",
				"language": "javascript",
			},
		},
	}
	sbomJSON, _ := json.Marshal(sbomData)
	ioutil.WriteFile(sbomPath, sbomJSON, 0644)

	// Test 1: Scan for vulnerabilities
	scanReq := map[string]interface{}{
		"target":    sbomPath,
		"scan_type": "sbom",
	}
	scanBody, _ := json.Marshal(scanReq)

	req := httptest.NewRequest("POST", "/api/v1/arf/phase4/security/scan", bytes.NewReader(scanBody))
	req.Header.Set("Content-Type", "application/json")
	resp, _ := app.Test(req)

	if resp.StatusCode != http.StatusOK {
		t.Errorf("Expected status 200, got %d", resp.StatusCode)
	}

	var scanResult map[string]interface{}
	body, _ := ioutil.ReadAll(resp.Body)
	json.Unmarshal(body, &scanResult)

	if scanResult["target"] != sbomPath {
		t.Errorf("Expected target %s, got %v", sbomPath, scanResult["target"])
	}

	// Test 2: Generate remediation plan
	vulns := []map[string]interface{}{
		{
			"id":          "CVE-2024-0001",
			"package":     "vulnerable-package",
			"version":     "1.0.0",
			"severity":    "CRITICAL",
			"cvss":        9.5,
			"fix_version": "1.0.1",
		},
	}
	remediationReq := map[string]interface{}{
		"vulnerabilities": vulns,
		"codebase": map[string]interface{}{
			"path":     tempDir,
			"language": "javascript",
		},
	}
	remediationBody, _ := json.Marshal(remediationReq)

	req = httptest.NewRequest("POST", "/api/v1/arf/phase4/security/remediation", bytes.NewReader(remediationBody))
	req.Header.Set("Content-Type", "application/json")
	resp, _ = app.Test(req)

	if resp.StatusCode != http.StatusOK {
		t.Errorf("Expected status 200, got %d", resp.StatusCode)
	}

	var remediationPlan map[string]interface{}
	body, _ = ioutil.ReadAll(resp.Body)
	json.Unmarshal(body, &remediationPlan)

	if remediationPlan["risk_assessment"] == nil {
		t.Error("Expected risk assessment in remediation plan")
	}

	// Test 3: Create approval workflow for critical vulnerability
	approvalReq := map[string]interface{}{
		"type":         "critical_remediation",
		"title":        "Critical Security Update Required",
		"description":  "Fix CVE-2024-0001 in vulnerable-package",
		"priority":     "critical",
		"requested_by": "security-scanner",
	}
	approvalBody, _ := json.Marshal(approvalReq)

	req = httptest.NewRequest("POST", "/api/v1/arf/phase4/workflow/approval", bytes.NewReader(approvalBody))
	req.Header.Set("Content-Type", "application/json")
	resp, _ = app.Test(req)

	if resp.StatusCode != http.StatusOK {
		t.Errorf("Expected status 200, got %d", resp.StatusCode)
	}

	var workflow map[string]interface{}
	body, _ = ioutil.ReadAll(resp.Body)
	json.Unmarshal(body, &workflow)

	workflowID := workflow["id"].(string)
	if workflowID == "" {
		t.Error("Expected workflow ID to be set")
	}

	// Test 4: Monitor performance during remediation
	perfMetrics := map[string]interface{}{
		"service":       "remediation-service",
		"response_time": 150.0,
		"error_rate":    0.01,
		"throughput":    1000,
		"cpu_usage":     45.0,
		"memory_usage":  60.0,
	}
	perfBody, _ := json.Marshal(perfMetrics)

	req = httptest.NewRequest("POST", "/api/v1/arf/phase4/performance/monitor", bytes.NewReader(perfBody))
	req.Header.Set("Content-Type", "application/json")
	resp, _ = app.Test(req)

	if resp.StatusCode != http.StatusOK {
		t.Errorf("Expected status 200, got %d", resp.StatusCode)
	}

	// Clean up
	os.RemoveAll(tempDir)
}

// TestPhase4Integration_SBOMAnalysisWorkflow tests complete SBOM analysis workflow
func TestPhase4Integration_SBOMAnalysisWorkflow(t *testing.T) {
	app := fiber.New()
	handler := NewHandler(nil, nil)
	handler.RegisterRoutes(app)

	// Create test SBOM with multiple formats
	tempDir := t.TempDir()

	// Test Syft format
	syftPath := filepath.Join(tempDir, "syft.sbom.json")
	syftData := map[string]interface{}{
		"artifacts": []interface{}{
			map[string]interface{}{
				"name":     "express",
				"version":  "4.18.0",
				"type":     "npm",
				"licenses": []string{"MIT"},
			},
			map[string]interface{}{
				"name":     "lodash",
				"version":  "4.17.20",
				"type":     "npm",
				"licenses": []string{"MIT"},
			},
		},
	}
	syftJSON, _ := json.Marshal(syftData)
	ioutil.WriteFile(syftPath, syftJSON, 0644)

	// Test SBOM analysis
	analysisReq := map[string]interface{}{
		"sbom_path": syftPath,
	}
	analysisBody, _ := json.Marshal(analysisReq)

	req := httptest.NewRequest("POST", "/api/v1/arf/phase4/sbom/analyze", bytes.NewReader(analysisBody))
	req.Header.Set("Content-Type", "application/json")
	resp, _ := app.Test(req)

	if resp.StatusCode != http.StatusOK {
		t.Errorf("Expected status 200, got %d", resp.StatusCode)
	}

	var analysis map[string]interface{}
	body, _ := ioutil.ReadAll(resp.Body)
	json.Unmarshal(body, &analysis)

	// Verify analysis contains expected data
	if analysis["sbom_path"] != syftPath {
		t.Errorf("Expected SBOM path %s, got %v", syftPath, analysis["sbom_path"])
	}

	if analysis["dependencies"] == nil {
		t.Error("Expected dependencies in analysis")
	}

	deps := analysis["dependencies"].([]interface{})
	if len(deps) != 2 {
		t.Errorf("Expected 2 dependencies, got %d", len(deps))
	}

	// Test vulnerability correlation
	correlateReq := map[string]interface{}{
		"dependencies": deps,
	}
	correlateBody, _ := json.Marshal(correlateReq)

	req = httptest.NewRequest("POST", "/api/v1/arf/phase4/sbom/correlate", bytes.NewReader(correlateBody))
	req.Header.Set("Content-Type", "application/json")
	resp, _ = app.Test(req)

	if resp.StatusCode != http.StatusOK {
		t.Errorf("Expected status 200, got %d", resp.StatusCode)
	}

	// Clean up
	os.RemoveAll(tempDir)
}

// TestPhase4Integration_ProductionOptimizationWorkflow tests production optimization
func TestPhase4Integration_ProductionOptimizationWorkflow(t *testing.T) {
	app := fiber.New()
	handler := NewHandler(nil, nil)
	handler.RegisterRoutes(app)

	// Test 1: Configure circuit breaker
	cbConfig := map[string]interface{}{
		"service": "production-api",
		"config": map[string]interface{}{
			"threshold":          5,
			"timeout":            30,
			"reset_timeout":      60,
			"half_open_requests": 3,
		},
	}
	cbBody, _ := json.Marshal(cbConfig)

	req := httptest.NewRequest("POST", "/api/v1/arf/phase4/optimization/circuit-breaker", bytes.NewReader(cbBody))
	req.Header.Set("Content-Type", "application/json")
	resp, _ := app.Test(req)

	if resp.StatusCode != http.StatusOK {
		t.Errorf("Expected status 200, got %d", resp.StatusCode)
	}

	// Test 2: Monitor service performance
	perfData := map[string]interface{}{
		"service": "production-api",
		"metrics": map[string]interface{}{
			"response_time": 2500.0,
			"error_rate":    0.15,
			"throughput":    100,
			"cpu_usage":     85.0,
			"memory_usage":  90.0,
			"latency":       500.0,
		},
	}
	perfBody, _ := json.Marshal(perfData)

	req = httptest.NewRequest("POST", "/api/v1/arf/phase4/performance/monitor", bytes.NewReader(perfBody))
	req.Header.Set("Content-Type", "application/json")
	resp, _ = app.Test(req)

	if resp.StatusCode != http.StatusOK {
		t.Errorf("Expected status 200, got %d", resp.StatusCode)
	}

	// Test 3: Analyze bottlenecks
	req = httptest.NewRequest("GET", "/api/v1/arf/phase4/optimization/bottlenecks", nil)
	resp, _ = app.Test(req)

	if resp.StatusCode != http.StatusOK {
		t.Errorf("Expected status 200, got %d", resp.StatusCode)
	}

	var bottlenecks map[string]interface{}
	body, _ := ioutil.ReadAll(resp.Body)
	json.Unmarshal(body, &bottlenecks)

	if bottlenecks["bottlenecks"] == nil {
		t.Error("Expected bottlenecks in response")
	}

	// Test 4: Optimize deployment based on bottlenecks
	deployConfig := map[string]interface{}{
		"service":  "production-api",
		"version":  "2.0.0",
		"replicas": 3,
		"resources": map[string]interface{}{
			"cpu":    "1000m",
			"memory": "2Gi",
		},
		"strategy": "rolling",
	}
	deployBody, _ := json.Marshal(deployConfig)

	req = httptest.NewRequest("POST", "/api/v1/arf/phase4/optimization/deploy", bytes.NewReader(deployBody))
	req.Header.Set("Content-Type", "application/json")
	resp, _ = app.Test(req)

	if resp.StatusCode != http.StatusOK {
		t.Errorf("Expected status 200, got %d", resp.StatusCode)
	}

	var optimizationPlan map[string]interface{}
	body, _ = ioutil.ReadAll(resp.Body)
	json.Unmarshal(body, &optimizationPlan)

	if optimizationPlan["recommendations"] == nil {
		t.Error("Expected recommendations in optimization plan")
	}

	// Test 5: Set auto-scaling rules
	scalingRules := map[string]interface{}{
		"service": "production-api",
		"rules": map[string]interface{}{
			"min_replicas": 2,
			"max_replicas": 10,
			"metrics": []map[string]interface{}{
				{
					"type":       "cpu",
					"target":     70.0,
					"scale_up":   2,
					"scale_down": 1,
				},
			},
		},
	}
	scalingBody, _ := json.Marshal(scalingRules)

	req = httptest.NewRequest("POST", "/api/v1/arf/phase4/optimization/autoscaling", bytes.NewReader(scalingBody))
	req.Header.Set("Content-Type", "application/json")
	resp, _ = app.Test(req)

	if resp.StatusCode != http.StatusOK {
		t.Errorf("Expected status 200, got %d", resp.StatusCode)
	}
}

// TestPhase4Integration_HumanWorkflowWithEscalation tests workflow escalation
func TestPhase4Integration_HumanWorkflowWithEscalation(t *testing.T) {
	app := fiber.New()
	handler := NewHandler(nil, nil)
	handler.RegisterRoutes(app)

	// Set escalation rules
	escalationRules := map[string]interface{}{
		"rules": []map[string]interface{}{
			{
				"priority":        "critical",
				"time_threshold":  1, // 1 nanosecond for immediate escalation in test
				"escalation_path": []string{"security-team", "ciso"},
			},
		},
	}
	escalationBody, _ := json.Marshal(escalationRules)

	req := httptest.NewRequest("POST", "/api/v1/arf/phase4/workflow/escalation", bytes.NewReader(escalationBody))
	req.Header.Set("Content-Type", "application/json")
	resp, _ := app.Test(req)

	if resp.StatusCode != http.StatusOK {
		t.Errorf("Expected status 200, got %d", resp.StatusCode)
	}

	// Create critical approval request
	approvalReq := map[string]interface{}{
		"type":         "critical_remediation",
		"title":        "Critical Production Issue",
		"description":  "Immediate action required",
		"priority":     "critical",
		"requested_by": "monitoring-system",
	}
	approvalBody, _ := json.Marshal(approvalReq)

	req = httptest.NewRequest("POST", "/api/v1/arf/phase4/workflow/approval", bytes.NewReader(approvalBody))
	req.Header.Set("Content-Type", "application/json")
	resp, _ = app.Test(req)

	if resp.StatusCode != http.StatusOK {
		t.Errorf("Expected status 200, got %d", resp.StatusCode)
	}

	var workflow map[string]interface{}
	body, _ := ioutil.ReadAll(resp.Body)
	json.Unmarshal(body, &workflow)

	workflowID := workflow["id"].(string)

	// Wait briefly for escalation
	time.Sleep(10 * time.Millisecond)

	// Check for escalations
	req = httptest.NewRequest("POST", "/api/v1/arf/phase4/workflow/check-escalations", nil)
	resp, _ = app.Test(req)

	if resp.StatusCode != http.StatusOK {
		t.Errorf("Expected status 200, got %d", resp.StatusCode)
	}

	var escalations map[string]interface{}
	body, _ = ioutil.ReadAll(resp.Body)
	json.Unmarshal(body, &escalations)

	escalatedIDs := escalations["escalated"].([]interface{})
	found := false
	for _, id := range escalatedIDs {
		if id.(string) == workflowID {
			found = true
			break
		}
	}

	if !found {
		t.Error("Expected workflow to be escalated")
	}
}

// TestPhase4Integration_ComplianceAssessment tests compliance checking
func TestPhase4Integration_ComplianceAssessment(t *testing.T) {
	app := fiber.New()
	handler := NewHandler(nil, nil)
	handler.RegisterRoutes(app)

	// Create vulnerability list with OWASP and CWE mappings
	vulns := []map[string]interface{}{
		{
			"id":       "CVE-2024-0001",
			"severity": "CRITICAL",
			"cvss":     9.0,
			"owasp":    []string{"A01:2021"},
			"cwe":      []string{"CWE-89"},
		},
		{
			"id":       "CVE-2024-0002",
			"severity": "HIGH",
			"cvss":     7.0,
			"owasp":    []string{"A02:2021"},
			"cwe":      []string{"CWE-79"},
		},
	}

	// Test compliance assessment
	complianceReq := map[string]interface{}{
		"vulnerabilities": vulns,
		"codebase": map[string]interface{}{
			"path":     "/test",
			"language": "javascript",
		},
	}
	complianceBody, _ := json.Marshal(complianceReq)

	req := httptest.NewRequest("POST", "/api/v1/arf/phase4/security/compliance", bytes.NewReader(complianceBody))
	req.Header.Set("Content-Type", "application/json")
	resp, _ := app.Test(req)

	if resp.StatusCode != http.StatusOK {
		t.Errorf("Expected status 200, got %d", resp.StatusCode)
	}

	var compliance map[string]interface{}
	body, _ := ioutil.ReadAll(resp.Body)
	json.Unmarshal(body, &compliance)

	// Verify compliance assessment structure
	if compliance["overall"] == nil {
		t.Error("Expected overall compliance status")
	}

	if compliance["frameworks"] == nil {
		t.Error("Expected framework compliance details")
	}

	frameworks := compliance["frameworks"].(map[string]interface{})
	if frameworks["OWASP"] == nil {
		t.Error("Expected OWASP compliance assessment")
	}

	if frameworks["NIST"] == nil {
		t.Error("Expected NIST compliance assessment")
	}
}

// TestPhase4Integration_ErrorHandling tests error handling across Phase 4
func TestPhase4Integration_ErrorHandling(t *testing.T) {
	app := fiber.New()
	handler := NewHandler(nil, nil)
	handler.RegisterRoutes(app)

	tests := []struct {
		name     string
		endpoint string
		method   string
		body     map[string]interface{}
		wantCode int
	}{
		{
			name:     "Invalid scan type",
			endpoint: "/api/v1/arf/phase4/security/scan",
			method:   "POST",
			body: map[string]interface{}{
				"target":    "/tmp/test",
				"scan_type": "invalid-type",
			},
			wantCode: http.StatusBadRequest,
		},
		{
			name:     "Missing required field",
			endpoint: "/api/v1/arf/phase4/workflow/approval",
			method:   "POST",
			body: map[string]interface{}{
				"description": "Missing type field",
			},
			wantCode: http.StatusBadRequest,
		},
		{
			name:     "Invalid workflow ID",
			endpoint: "/api/v1/arf/phase4/workflow/approve/invalid-id",
			method:   "POST",
			body: map[string]interface{}{
				"approved":    true,
				"approved_by": "admin",
			},
			wantCode: http.StatusNotFound,
		},
		{
			name:     "Invalid circuit breaker config",
			endpoint: "/api/v1/arf/phase4/optimization/circuit-breaker",
			method:   "POST",
			body: map[string]interface{}{
				"service": "test-service",
				"config": map[string]interface{}{
					"threshold": 0, // Invalid: must be > 0
				},
			},
			wantCode: http.StatusBadRequest,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			body, _ := json.Marshal(tt.body)
			req := httptest.NewRequest(tt.method, tt.endpoint, bytes.NewReader(body))
			req.Header.Set("Content-Type", "application/json")
			resp, _ := app.Test(req)

			if resp.StatusCode != tt.wantCode {
				t.Errorf("Expected status %d, got %d", tt.wantCode, resp.StatusCode)
			}
		})
	}
}

// BenchmarkPhase4Integration_FullWorkflow benchmarks complete Phase 4 workflow
func BenchmarkPhase4Integration_FullWorkflow(b *testing.B) {
	app := fiber.New()
	handler := NewHandler(nil, nil)
	handler.RegisterRoutes(app)

	// Prepare test data
	scanReq := map[string]interface{}{
		"target":    "/tmp/test.sbom.json",
		"scan_type": "sbom",
	}
	scanBody, _ := json.Marshal(scanReq)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		// Scan
		req := httptest.NewRequest("POST", "/api/v1/arf/phase4/security/scan", bytes.NewReader(scanBody))
		req.Header.Set("Content-Type", "application/json")
		app.Test(req)

		// Monitor
		perfReq := map[string]interface{}{
			"service": "bench-service",
			"metrics": map[string]interface{}{
				"response_time": 100.0,
				"error_rate":    0.01,
			},
		}
		perfBody, _ := json.Marshal(perfReq)
		req = httptest.NewRequest("POST", "/api/v1/arf/phase4/performance/monitor", bytes.NewReader(perfBody))
		req.Header.Set("Content-Type", "application/json")
		app.Test(req)
	}
}
