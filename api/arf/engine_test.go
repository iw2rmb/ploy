package arf

import (
	"encoding/json"
	"testing"
	"time"
)

func TestTransformationResultWithHealingFields(t *testing.T) {
	tests := []struct {
		name string
		test func(t *testing.T)
	}{
		{
			name: "should serialize and deserialize healing workflow fields",
			test: testHealingFieldsSerialization,
		},
		{
			name: "should maintain backward compatibility with existing fields",
			test: testBackwardCompatibility,
		},
		{
			name: "should handle nested children healing attempts",
			test: testNestedHealingAttempts,
		},
		{
			name: "should properly handle deployment status",
			test: testDeploymentStatus,
		},
		{
			name: "should track parent-child relationships",
			test: testParentChildRelationships,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.test(t)
		})
	}
}

func testHealingFieldsSerialization(t *testing.T) {
	result := &TransformationResult{
		TransformationID: "test-transform-123",
		RecipeID:         "upgrade-java-17",
		Success:          true,
		ChangesApplied:   5,
		ExecutionTime:    15 * time.Minute,

		// New healing workflow fields
		WorkflowStage:   "heal",
		ChildTransforms: []string{"child-1", "child-2"},
		ParentTransform: "parent-456",
		Children: []HealingAttempt{
			{
				TransformationID: "child-1",
				AttemptPath:      "1",
				TriggerReason:    "build_failure",
				Status:           "completed",
				Result:           "success",
			},
		},
		SandboxID: "sandbox-789",
		DeploymentStatus: &DeploymentMetrics{
			DeploymentID:     "deploy-abc",
			DeploymentURL:    "https://sandbox-789.ployd.app",
			DeploymentStatus: "healthy",
			DeploymentTime:   5 * time.Minute,
			SandboxID:        "sandbox-789",
		},
		ConsulKey:   "ploy/arf/transforms/test-transform-123",
		LastUpdated: time.Now(),
	}

	// Serialize to JSON
	data, err := json.Marshal(result)
	if err != nil {
		t.Fatalf("Failed to marshal TransformationResult: %v", err)
	}

	// Deserialize back
	var decoded TransformationResult
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("Failed to unmarshal TransformationResult: %v", err)
	}

	// Verify key fields
	if decoded.WorkflowStage != "heal" {
		t.Errorf("WorkflowStage mismatch: got %s, want heal", decoded.WorkflowStage)
	}

	if len(decoded.ChildTransforms) != 2 {
		t.Errorf("ChildTransforms count mismatch: got %d, want 2", len(decoded.ChildTransforms))
	}

	if decoded.ParentTransform != "parent-456" {
		t.Errorf("ParentTransform mismatch: got %s, want parent-456", decoded.ParentTransform)
	}

	if len(decoded.Children) != 1 {
		t.Errorf("Children count mismatch: got %d, want 1", len(decoded.Children))
	}

	if decoded.SandboxID != "sandbox-789" {
		t.Errorf("SandboxID mismatch: got %s, want sandbox-789", decoded.SandboxID)
	}

	if decoded.DeploymentStatus == nil {
		t.Error("DeploymentStatus should not be nil")
	} else if decoded.DeploymentStatus.DeploymentID != "deploy-abc" {
		t.Errorf("DeploymentID mismatch: got %s, want deploy-abc", decoded.DeploymentStatus.DeploymentID)
	}

	if decoded.ConsulKey != "ploy/arf/transforms/test-transform-123" {
		t.Errorf("ConsulKey mismatch: got %s, want ploy/arf/transforms/test-transform-123", decoded.ConsulKey)
	}

	if decoded.LastUpdated.IsZero() {
		t.Error("LastUpdated should not be zero")
	}
}

func testBackwardCompatibility(t *testing.T) {
	// Test that existing fields work without new healing fields
	result := &TransformationResult{
		RecipeID:        "legacy-recipe",
		Success:         false,
		ChangesApplied:  3,
		TotalFiles:      10,
		FilesModified:   []string{"file1.java", "file2.java"},
		Diff:            "unified diff content",
		ValidationScore: 0.85,
		ExecutionTime:   10 * time.Minute,
		Errors: []TransformationError{
			{
				Type:    "compilation",
				Message: "Failed to compile",
				File:    "Main.java",
				Line:    42,
			},
		},
	}

	// Serialize and deserialize
	data, err := json.Marshal(result)
	if err != nil {
		t.Fatalf("Failed to marshal legacy TransformationResult: %v", err)
	}

	var decoded TransformationResult
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("Failed to unmarshal legacy TransformationResult: %v", err)
	}

	// Verify legacy fields are preserved
	if decoded.RecipeID != "legacy-recipe" {
		t.Errorf("RecipeID mismatch: got %s, want legacy-recipe", decoded.RecipeID)
	}

	if decoded.Success != false {
		t.Error("Success should be false")
	}

	if decoded.ChangesApplied != 3 {
		t.Errorf("ChangesApplied mismatch: got %d, want 3", decoded.ChangesApplied)
	}

	if len(decoded.FilesModified) != 2 {
		t.Errorf("FilesModified count mismatch: got %d, want 2", len(decoded.FilesModified))
	}

	if len(decoded.Errors) != 1 {
		t.Errorf("Errors count mismatch: got %d, want 1", len(decoded.Errors))
	}

	// New fields should be empty/nil
	if decoded.WorkflowStage != "" {
		t.Errorf("WorkflowStage should be empty for legacy data, got %s", decoded.WorkflowStage)
	}

	if len(decoded.ChildTransforms) != 0 {
		t.Error("ChildTransforms should be empty for legacy data")
	}

	if decoded.DeploymentStatus != nil {
		t.Error("DeploymentStatus should be nil for legacy data")
	}
}

func testNestedHealingAttempts(t *testing.T) {
	result := &TransformationResult{
		TransformationID: "root-transform",
		RecipeID:         "complex-upgrade",
		WorkflowStage:    "heal",
		Children: []HealingAttempt{
			{
				TransformationID: "heal-1",
				AttemptPath:      "1",
				TriggerReason:    "build_failure",
				Status:           "completed",
				Result:           "partial_success",
				Children: []HealingAttempt{
					{
						TransformationID: "heal-1-1",
						AttemptPath:      "1.1",
						TriggerReason:    "test_failure",
						Status:           "in_progress",
						ParentAttempt:    "1",
					},
				},
			},
			{
				TransformationID: "heal-2",
				AttemptPath:      "2",
				TriggerReason:    "test_failure",
				Status:           "completed",
				Result:           "success",
			},
		},
	}

	// Verify nested structure
	if len(result.Children) != 2 {
		t.Errorf("Root children count mismatch: got %d, want 2", len(result.Children))
	}

	firstChild := result.Children[0]
	if len(firstChild.Children) != 1 {
		t.Errorf("First child's children count mismatch: got %d, want 1", len(firstChild.Children))
	}

	nestedChild := firstChild.Children[0]
	if nestedChild.AttemptPath != "1.1" {
		t.Errorf("Nested child path mismatch: got %s, want 1.1", nestedChild.AttemptPath)
	}

	if nestedChild.ParentAttempt != "1" {
		t.Errorf("Nested child parent mismatch: got %s, want 1", nestedChild.ParentAttempt)
	}
}

func testDeploymentStatus(t *testing.T) {
	result := &TransformationResult{
		TransformationID: "deploy-test",
		RecipeID:         "deploy-recipe",
		DeploymentStatus: &DeploymentMetrics{
			DeploymentID:     "deploy-123",
			DeploymentURL:    "https://test.ployd.app",
			DeploymentStatus: "healthy",
			DeploymentTime:   3 * time.Minute,
			SandboxID:        "sandbox-test",
		},
	}

	// Serialize and verify
	data, err := json.Marshal(result)
	if err != nil {
		t.Fatalf("Failed to marshal with DeploymentStatus: %v", err)
	}

	var decoded TransformationResult
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("Failed to unmarshal with DeploymentStatus: %v", err)
	}

	if decoded.DeploymentStatus == nil {
		t.Fatal("DeploymentStatus should not be nil")
	}

	if decoded.DeploymentStatus.DeploymentURL != "https://test.ployd.app" {
		t.Errorf("DeploymentURL mismatch: got %s, want https://test.ployd.app",
			decoded.DeploymentStatus.DeploymentURL)
	}

	if decoded.DeploymentStatus.DeploymentStatus != "healthy" {
		t.Errorf("DeploymentStatus status mismatch: got %s, want healthy",
			decoded.DeploymentStatus.DeploymentStatus)
	}
}

func testParentChildRelationships(t *testing.T) {
	// Create parent transformation
	parent := &TransformationResult{
		TransformationID: "parent-123",
		RecipeID:         "parent-recipe",
		ChildTransforms:  []string{"child-456", "child-789"},
		WorkflowStage:    "heal",
	}

	// Create child transformation
	child := &TransformationResult{
		TransformationID: "child-456",
		RecipeID:         "healing-recipe",
		ParentTransform:  "parent-123",
		WorkflowStage:    "openrewrite",
	}

	// Verify relationships
	if len(parent.ChildTransforms) != 2 {
		t.Errorf("Parent should have 2 children, got %d", len(parent.ChildTransforms))
	}

	if parent.ChildTransforms[0] != "child-456" {
		t.Errorf("First child ID mismatch: got %s, want child-456", parent.ChildTransforms[0])
	}

	if child.ParentTransform != "parent-123" {
		t.Errorf("Child parent ID mismatch: got %s, want parent-123", child.ParentTransform)
	}

	// Verify both can be serialized
	parentData, err := json.Marshal(parent)
	if err != nil {
		t.Fatalf("Failed to marshal parent: %v", err)
	}

	childData, err := json.Marshal(child)
	if err != nil {
		t.Fatalf("Failed to marshal child: %v", err)
	}

	// Verify they deserialize correctly
	var decodedParent, decodedChild TransformationResult

	if err := json.Unmarshal(parentData, &decodedParent); err != nil {
		t.Fatalf("Failed to unmarshal parent: %v", err)
	}

	if err := json.Unmarshal(childData, &decodedChild); err != nil {
		t.Fatalf("Failed to unmarshal child: %v", err)
	}

	if decodedParent.TransformationID != parent.TransformationID {
		t.Error("Parent transformation ID not preserved")
	}

	if decodedChild.ParentTransform != child.ParentTransform {
		t.Error("Child parent reference not preserved")
	}
}
