//go:build arf_legacy
// +build arf_legacy

package arf

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

// MockLLMGenerator for testing
type MockLLMGenerator struct {
	mock.Mock
}

func (m *MockLLMGenerator) GenerateRecipe(ctx context.Context, request RecipeGenerationRequest) (*GeneratedRecipe, error) {
	args := m.Called(ctx, request)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*GeneratedRecipe), args.Error(1)
}

func (m *MockLLMGenerator) GetCapabilities() LLMCapabilities {
	args := m.Called()
	return args.Get(0).(LLMCapabilities)
}

func (m *MockLLMGenerator) IsAvailable(ctx context.Context) bool {
	args := m.Called(ctx)
	return args.Bool(0)
}

func (m *MockLLMGenerator) ValidateGenerated(ctx context.Context, recipe GeneratedRecipe) (*EvolutionValidationResult, error) {
	args := m.Called(ctx, recipe)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*EvolutionValidationResult), args.Error(1)
}

func (m *MockLLMGenerator) OptimizeRecipe(ctx context.Context, recipe interface{}, feedback TransformationFeedback) (interface{}, error) {
	args := m.Called(ctx, recipe, feedback)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0), args.Error(1)
}

// HealingMockConsulStore for testing
type HealingMockConsulStore struct {
	mock.Mock
	data map[string]*TransformationStatus
}

func NewHealingMockConsulStore() *HealingMockConsulStore {
	return &HealingMockConsulStore{
		data: make(map[string]*TransformationStatus),
	}
}

func (m *HealingMockConsulStore) StoreTransformationStatus(ctx context.Context, id string, status *TransformationStatus) error {
	m.data[id] = status
	return m.Called(ctx, id, status).Error(0)
}

func (m *HealingMockConsulStore) GetTransformationStatus(ctx context.Context, id string) (*TransformationStatus, error) {
	args := m.Called(ctx, id)
	if status, ok := m.data[id]; ok {
		return status, args.Error(1)
	}
	return args.Get(0).(*TransformationStatus), args.Error(1)
}

func (m *HealingMockConsulStore) UpdateWorkflowStage(ctx context.Context, id string, stage string) error {
	if status, ok := m.data[id]; ok {
		status.WorkflowStage = stage
	}
	return m.Called(ctx, id, stage).Error(0)
}

func (m *HealingMockConsulStore) AddHealingAttempt(ctx context.Context, rootID, attemptPath string, attempt *HealingAttempt) error {
	args := m.Called(ctx, rootID, attemptPath, attempt)
	if status, ok := m.data[rootID]; ok {
		status.Children = append(status.Children, *attempt)
		status.TotalHealingAttempts++
	}
	return args.Error(0)
}

func (m *HealingMockConsulStore) UpdateHealingAttempt(ctx context.Context, rootID, attemptPath string, attempt *HealingAttempt) error {
	return m.Called(ctx, rootID, attemptPath, attempt).Error(0)
}

func (m *HealingMockConsulStore) GetHealingTree(ctx context.Context, rootID string) (*HealingTree, error) {
	args := m.Called(ctx, rootID)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*HealingTree), args.Error(1)
}

func (m *HealingMockConsulStore) GetActiveHealingAttempts(ctx context.Context, rootID string) ([]string, error) {
	args := m.Called(ctx, rootID)
	return args.Get(0).([]string), args.Error(1)
}

func (m *HealingMockConsulStore) CleanupCompletedTransformations(ctx context.Context, maxAge time.Duration) error {
	return m.Called(ctx, maxAge).Error(0)
}

func (m *HealingMockConsulStore) SetTransformationTTL(ctx context.Context, id string, ttl time.Duration) error {
	return m.Called(ctx, id, ttl).Error(0)
}

func (m *HealingMockConsulStore) GenerateNextAttemptPath(ctx context.Context, rootID string, parentPath string) (string, error) {
	args := m.Called(ctx, rootID, parentPath)
	return args.String(0), args.Error(1)
}

func TestExecuteHealingWorkflow(t *testing.T) {
	tests := []struct {
		name           string
		errors         []string
		parentPath     string
		expectHealing  bool
		maxDepth       int
		simulateErrors int // Number of recursive errors to simulate
	}{
		{
			name:          "single healing attempt - success",
			errors:        []string{"compilation error: undefined variable"},
			parentPath:    "",
			expectHealing: true,
			maxDepth:      5,
		},
		{
			name:           "recursive healing - 2 levels",
			errors:         []string{"compilation error: undefined variable"},
			parentPath:     "",
			expectHealing:  true,
			maxDepth:       5,
			simulateErrors: 1, // Will cause one additional healing level
		},
		{
			name:           "max depth reached",
			errors:         []string{"compilation error: undefined variable"},
			parentPath:     "1.1.1.1.1", // Already at depth 5
			expectHealing:  false,
			maxDepth:       5,
			simulateErrors: 10, // Won't matter due to depth limit
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Setup
			mockLLM := new(MockLLMGenerator)
			mockConsul := NewHealingMockConsulStore()

			h := &Handler{
				llmGenerator: mockLLM,
				consulStore:  mockConsul,
			}

			// Create config
			config := &HealingConfig{
				MaxHealingDepth:     tt.maxDepth,
				MaxParallelAttempts: 3,
				HealingTimeout:      30 * time.Minute,
				AttemptTimeout:      5 * time.Minute,
			}

			transformID := uuid.New().String()
			ctx := context.Background()

			// Setup mock expectations
			mockConsul.On("GetTransformationStatus", ctx, transformID).Return(&TransformationStatus{
				TransformationID: transformID,
				WorkflowStage:    "heal",
				Status:           "in_progress",
				Children:         []HealingAttempt{},
			}, nil)

			if tt.expectHealing {
				mockConsul.On("GenerateNextAttemptPath", ctx, transformID, tt.parentPath).Return("1", nil)
				mockConsul.On("AddHealingAttempt", ctx, transformID, mock.Anything, mock.Anything).Return(nil)
				mockConsul.On("UpdateHealingAttempt", ctx, transformID, mock.Anything, mock.Anything).Return(nil)

				// Mock LLM response
				mockLLM.On("GenerateRecipe", ctx, mock.Anything).Return(&GeneratedRecipe{
					ID:          "healing-recipe-1",
					Name:        "Healing Recipe",
					Description: "Fix compilation error",
					Language:    "java",
					Recipe: map[string]interface{}{
						"fixes": []map[string]interface{}{
							{
								"type":   "add_import",
								"file":   "Main.java",
								"import": "java.util.List",
							},
						},
					},
					Confidence:  0.85,
					Explanation: "Added missing variable declaration",
				}, nil)
			}

			// Execute
			err := h.executeHealingWorkflow(ctx, transformID, tt.errors, tt.parentPath, config)

			// Assert
			if tt.expectHealing {
				assert.NoError(t, err)
				mockConsul.AssertExpectations(t)
				mockLLM.AssertExpectations(t)
			} else {
				// Should skip healing due to max depth
				assert.Error(t, err)
				assert.Contains(t, err.Error(), "max healing depth")
			}
		})
	}
}

func TestAnalyzeBuildErrors(t *testing.T) {
	tests := []struct {
		name           string
		errors         []string
		expectedType   string
		expectedPrompt string
	}{
		{
			name:           "compilation error",
			errors:         []string{"error: cannot find symbol: class Foo"},
			expectedType:   "compilation",
			expectedPrompt: "Fix the following compilation errors",
		},
		{
			name:           "test failure",
			errors:         []string{"Test failed: expected <5> but was <3>"},
			expectedType:   "test",
			expectedPrompt: "Fix the following test failures",
		},
		{
			name:           "mixed errors",
			errors:         []string{"error: cannot find symbol", "Test failed: assertion error"},
			expectedType:   "mixed",
			expectedPrompt: "Fix the following errors",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := &Handler{}

			analysis := h.analyzeBuildErrors(tt.errors)

			assert.NotNil(t, analysis)
			assert.Equal(t, tt.expectedType, analysis.ErrorType)
			assert.Contains(t, analysis.SuggestedFix, tt.expectedPrompt)
		})
	}
}

func TestDetermineTriggerReason(t *testing.T) {
	tests := []struct {
		name           string
		errors         []string
		expectedReason string
	}{
		{
			name:           "build failure",
			errors:         []string{"BUILD FAILED", "compilation error"},
			expectedReason: "build_failure",
		},
		{
			name:           "test failure",
			errors:         []string{"Tests run: 5, Failures: 2"},
			expectedReason: "test_failure",
		},
		{
			name:           "validation failure",
			errors:         []string{"Validation failed: missing dependency"},
			expectedReason: "validation_failure",
		},
		{
			name:           "unknown failure",
			errors:         []string{"Something went wrong"},
			expectedReason: "unknown_failure",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := &Handler{}

			reason := h.determineTriggerReason(tt.errors)

			assert.Equal(t, tt.expectedReason, reason)
		})
	}
}

func TestValidateAfterHealing(t *testing.T) {
	tests := []struct {
		name          string
		sandboxID     string
		buildSuccess  bool
		testSuccess   bool
		expectedCount int
	}{
		{
			name:          "all validations pass",
			sandboxID:     "sandbox-123",
			buildSuccess:  true,
			testSuccess:   true,
			expectedCount: 0,
		},
		{
			name:          "build fails",
			sandboxID:     "sandbox-456",
			buildSuccess:  false,
			testSuccess:   true,
			expectedCount: 1,
		},
		{
			name:          "tests fail",
			sandboxID:     "sandbox-789",
			buildSuccess:  true,
			testSuccess:   false,
			expectedCount: 1,
		},
		{
			name:          "both fail",
			sandboxID:     "sandbox-000",
			buildSuccess:  false,
			testSuccess:   false,
			expectedCount: 2,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := &Handler{
				sandboxMgr: &TestSandboxManager{
					buildSuccess: tt.buildSuccess,
					testSuccess:  tt.testSuccess,
				},
			}

			errors := h.validateAfterHealing(tt.sandboxID)

			assert.Len(t, errors, tt.expectedCount)
		})
	}
}

func TestHealingDepthControl(t *testing.T) {
	h := &Handler{}
	config := &HealingConfig{
		MaxHealingDepth: 3,
	}

	tests := []struct {
		name        string
		currentPath string
		canHeal     bool
	}{
		{
			name:        "root level",
			currentPath: "",
			canHeal:     true,
		},
		{
			name:        "depth 1",
			currentPath: "1",
			canHeal:     true,
		},
		{
			name:        "depth 2",
			currentPath: "1.1",
			canHeal:     true,
		},
		{
			name:        "depth 3 - at limit",
			currentPath: "1.1.1",
			canHeal:     false,
		},
		{
			name:        "depth 4 - exceeded",
			currentPath: "1.1.1.1",
			canHeal:     false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			canHeal := h.canPerformHealing(tt.currentPath, config)
			assert.Equal(t, tt.canHeal, canHeal)
		})
	}
}

// TestSandboxManager for testing
type TestSandboxManager struct {
	buildSuccess bool
	testSuccess  bool
}

func (m *TestSandboxManager) CreateSandbox(ctx context.Context, config SandboxConfig) (*Sandbox, error) {
	return &Sandbox{ID: "mock-sandbox"}, nil
}

func (m *TestSandboxManager) DestroySandbox(ctx context.Context, sandboxID string) error {
	return nil
}

func (m *TestSandboxManager) ListSandboxes(ctx context.Context) ([]SandboxInfo, error) {
	return []SandboxInfo{}, nil
}

func (m *TestSandboxManager) CleanupExpiredSandboxes(ctx context.Context) error {
	return nil
}

func (m *TestSandboxManager) ExecuteCommand(ctx context.Context, sandboxID string, command string, args ...string) (string, error) {
	// Mock implementation for testing
	if command == "mvn" && args[0] == "clean" && args[1] == "compile" {
		if m.buildSuccess {
			return "[INFO] BUILD SUCCESS", nil
		}
		return "[ERROR] Build failed: compilation error", nil
	}
	if command == "mvn" && args[0] == "test" {
		if m.testSuccess {
			return "Tests run: 10, Failures: 0, Errors: 0, Skipped: 0", nil
		}
		return "Tests run: 10, Failures: 2, Errors: 0, Skipped: 0", nil
	}
	return "", nil
}
