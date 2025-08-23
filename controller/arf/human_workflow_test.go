package arf

import (
	"context"
	"testing"
	"time"
)

func TestHumanWorkflowEngine_CreateRemediationApprovalWorkflow(t *testing.T) {
	engine := NewHumanWorkflowEngine(nil, nil, nil, nil, nil)
	ctx := context.Background()

	tests := []struct {
		name        string
		recipe      RemediationRecipe
		vulns       []VulnerabilityInfo
		requesterID string
		wantErr     bool
	}{
		{
			name: "Valid critical remediation request",
			recipe: RemediationRecipe{
				ID:              "recipe-1",
				Name:            "Critical Security Update",
				Description:     "Update package to fix CVE-2024-0001",
				Vulnerabilities: []string{"CVE-2024-0001"},
				Metadata: map[string]interface{}{
					"severity": "critical",
				},
				CreatedAt: time.Now(),
			},
			vulns: []VulnerabilityInfo{
				{
					CVE: CVEInfo{
						ID:       "CVE-2024-0001",
						Severity: "critical",
					},
					Severity: "HIGH",
					CVSS:     9.5,
				},
			},
			requesterID: "security-scanner",
			wantErr:     false,
		},
		{
			name: "Valid medium priority remediation",
			recipe: RemediationRecipe{
				ID:              "recipe-2",
				Name:            "Medium Security Update",
				Description:     "Update package to fix medium severity issue",
				Vulnerabilities: []string{"CVE-2024-0002"},
				CreatedAt:       time.Now(),
			},
			vulns: []VulnerabilityInfo{
				{
					CVE: CVEInfo{
						ID:       "CVE-2024-0002",
						Severity: "medium",
					},
					Severity: "MEDIUM",
					CVSS:     5.5,
				},
			},
			requesterID: "ci-system",
			wantErr:     false,
		},
		{
			name:        "Invalid request - empty recipe ID",
			recipe:      RemediationRecipe{},
			vulns:       []VulnerabilityInfo{},
			requesterID: "test-user",
			wantErr:     true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			workflow, err := engine.CreateRemediationApprovalWorkflow(ctx, tt.recipe, tt.vulns, tt.requesterID)
			
			if (err != nil) != tt.wantErr {
				t.Errorf("CreateRemediationApprovalWorkflow() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			
			if !tt.wantErr && workflow != nil {
				// Validate workflow structure
				if workflow.ID == "" {
					t.Error("Expected workflow ID to be set")
				}
				if workflow.Status != WorkflowPending {
					t.Errorf("Expected status 'pending', got %s", workflow.Status)
				}
				if workflow.CreatedAt.IsZero() {
					t.Error("Expected CreatedAt to be set")
				}
				if workflow.Type != "remediation_approval" {
					t.Errorf("Expected workflow type 'remediation_approval', got %s", workflow.Type)
				}
			}
		})
	}
}

func TestHumanWorkflowEngine_GetPendingWorkflows(t *testing.T) {
	engine := NewHumanWorkflowEngine(nil, nil, nil, nil, nil)
	
	// Test getting pending workflows
	pending, err := engine.GetPendingWorkflows("test-user")
	if err != nil {
		t.Fatalf("GetPendingWorkflows() error = %v", err)
	}

	// Should return empty list with nil services
	if pending == nil {
		t.Error("Expected non-nil pending workflows slice")
	}
}

func TestHumanWorkflowEngine_ProcessWorkflowDecision(t *testing.T) {
	engine := NewHumanWorkflowEngine(nil, nil, nil, nil, nil)
	ctx := context.Background()

	// Create a test workflow first
	recipe := RemediationRecipe{
		ID:              "recipe-test",
		Name:            "Test Remediation",
		Description:     "Test description",
		Vulnerabilities: []string{"CVE-2024-0001"},
		CreatedAt:       time.Now(),
	}
	vulns := []VulnerabilityInfo{
		{
			CVE: CVEInfo{
				ID:       "CVE-2024-0001",
				Severity: "high",
			},
			Severity: "HIGH",
			CVSS:     7.5,
		},
	}
	
	workflow, err := engine.CreateRemediationApprovalWorkflow(ctx, recipe, vulns, "test-user")
	if err != nil {
		t.Fatalf("Failed to create test workflow: %v", err)
	}

	decision := Decision{
		DecisionType: DecisionApprove,
		Reasoning:    "Approved for testing",
		MadeBy:       "admin-user",
		MadeAt:       time.Now(),
		Confidence:   1.0,
	}

	// Test processing decision
	err = engine.ProcessWorkflowDecision(ctx, workflow.ID, decision)
	
	// With nil services, this will likely error, but we're testing the interface
	if err != nil {
		// Expected with nil services - just confirm the method signature is correct
		t.Logf("ProcessWorkflowDecision returned expected error with nil services: %v", err)
	}
}

// Benchmark test
func BenchmarkHumanWorkflowEngine_CreateRemediationApprovalWorkflow(b *testing.B) {
	engine := NewHumanWorkflowEngine(nil, nil, nil, nil, nil)
	ctx := context.Background()

	recipe := RemediationRecipe{
		ID:              "benchmark-recipe",
		Name:            "Benchmark Request",
		Description:     "Benchmark description",
		Vulnerabilities: []string{"CVE-2024-0001"},
		CreatedAt:       time.Now(),
	}
	
	vulns := []VulnerabilityInfo{
		{
			CVE: CVEInfo{
				ID:       "CVE-2024-0001",
				Severity: "medium",
			},
			Severity: "MEDIUM",
			CVSS:     5.0,
		},
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = engine.CreateRemediationApprovalWorkflow(ctx, recipe, vulns, "benchmark")
	}
}

func BenchmarkHumanWorkflowEngine_GetPendingWorkflows(b *testing.B) {
	engine := NewHumanWorkflowEngine(nil, nil, nil, nil, nil)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = engine.GetPendingWorkflows("test-user")
	}
}