package arf

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestValidateBuild(t *testing.T) {
	tests := []struct {
		name           string
		sandboxID      string
		buildTool      string
		mockOutput     string
		expectedResult BuildValidationResult
		expectError    bool
	}{
		{
			name:       "successful Maven build",
			sandboxID:  "sandbox-123",
			buildTool:  "maven",
			mockOutput: "[INFO] BUILD SUCCESS\n[INFO] Total time: 10.5s",
			expectedResult: BuildValidationResult{
				Success:      true,
				BuildTool:    "maven",
				BuildCommand: "mvn clean compile",
				Duration:     10 * time.Second,
				Output:       "[INFO] BUILD SUCCESS\n[INFO] Total time: 10.5s",
			},
			expectError: false,
		},
		{
			name:       "failed Maven build with compilation errors",
			sandboxID:  "sandbox-456",
			buildTool:  "maven",
			mockOutput: "[ERROR] /src/main/java/App.java:[10,5] cannot find symbol\n[INFO] BUILD FAILURE",
			expectedResult: BuildValidationResult{
				Success:      false,
				BuildTool:    "maven",
				BuildCommand: "mvn clean compile",
				Output:       "[ERROR] /src/main/java/App.java:[10,5] cannot find symbol\n[INFO] BUILD FAILURE",
				Errors: []ValidationBuildError{
					{
						Type:    "compilation",
						File:    "/src/main/java/App.java",
						Line:    10,
						Column:  5,
						Message: "cannot find symbol",
					},
				},
			},
			expectError: false,
		},
		{
			name:       "successful Gradle build",
			sandboxID:  "sandbox-789",
			buildTool:  "gradle",
			mockOutput: "BUILD SUCCESSFUL in 8s",
			expectedResult: BuildValidationResult{
				Success:      true,
				BuildTool:    "gradle",
				BuildCommand: "gradle build -x test",
				Duration:     8 * time.Second,
				Output:       "BUILD SUCCESSFUL in 8s",
			},
			expectError: false,
		},
		{
			name:       "successful npm build",
			sandboxID:  "sandbox-npm",
			buildTool:  "npm",
			mockOutput: "✓ Compiled successfully",
			expectedResult: BuildValidationResult{
				Success:      true,
				BuildTool:    "npm",
				BuildCommand: "npm run build",
				Output:       "✓ Compiled successfully",
			},
			expectError: false,
		},
		{
			name:       "successful Go build",
			sandboxID:  "sandbox-go",
			buildTool:  "go",
			mockOutput: "",
			expectedResult: BuildValidationResult{
				Success:      true,
				BuildTool:    "go",
				BuildCommand: "go build -v ./...",
				Output:       "",
			},
			expectError: false,
		},
		{
			name:        "build timeout",
			sandboxID:   "sandbox-timeout",
			buildTool:   "maven",
			mockOutput:  "",
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			validator := &SandboxValidator{
				sandboxMgr: &MockValidationSandboxManager{
					mockOutput:    tt.mockOutput,
					shouldTimeout: tt.name == "build timeout",
				},
			}

			config := BuildConfig{
				BuildTool: tt.buildTool,
				Timeout:   30 * time.Second,
			}

			result, err := validator.ValidateBuild(context.Background(), tt.sandboxID, config)

			if tt.expectError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.expectedResult.Success, result.Success)
				assert.Equal(t, tt.expectedResult.BuildTool, result.BuildTool)
				assert.Equal(t, tt.expectedResult.BuildCommand, result.BuildCommand)
				assert.Equal(t, len(tt.expectedResult.Errors), len(result.Errors))
			}
		})
	}
}

func TestRunTests(t *testing.T) {
	tests := []struct {
		name           string
		sandboxID      string
		testFramework  string
		mockOutput     string
		expectedResult TestValidationResult
		expectError    bool
	}{
		{
			name:          "successful Maven tests",
			sandboxID:     "sandbox-test-123",
			testFramework: "maven",
			mockOutput:    "Tests run: 10, Failures: 0, Errors: 0, Skipped: 0\n[INFO] BUILD SUCCESS",
			expectedResult: TestValidationResult{
				Success:       true,
				TestFramework: "maven",
				TestCommand:   "mvn test",
				TotalTests:    10,
				PassedTests:   10,
				FailedTests:   0,
				SkippedTests:  0,
				Output:        "Tests run: 10, Failures: 0, Errors: 0, Skipped: 0\n[INFO] BUILD SUCCESS",
			},
			expectError: false,
		},
		{
			name:          "failed Maven tests",
			sandboxID:     "sandbox-test-456",
			testFramework: "maven",
			mockOutput:    "Tests run: 10, Failures: 2, Errors: 1, Skipped: 0\n[INFO] BUILD FAILURE",
			expectedResult: TestValidationResult{
				Success:       false,
				TestFramework: "maven",
				TestCommand:   "mvn test",
				TotalTests:    10,
				PassedTests:   7,
				FailedTests:   2,
				SkippedTests:  0,
				Output:        "Tests run: 10, Failures: 2, Errors: 1, Skipped: 0\n[INFO] BUILD FAILURE",
				Failures: []TestFailure{
					{
						TestName: "Unknown",
						Message:  "2 test failures, 1 error",
					},
				},
			},
			expectError: false,
		},
		{
			name:          "successful Gradle tests",
			sandboxID:     "sandbox-gradle-test",
			testFramework: "gradle",
			mockOutput:    "15 tests completed, 15 passed\nBUILD SUCCESSFUL",
			expectedResult: TestValidationResult{
				Success:       true,
				TestFramework: "gradle",
				TestCommand:   "gradle test",
				TotalTests:    15,
				PassedTests:   15,
				FailedTests:   0,
				Output:        "15 tests completed, 15 passed\nBUILD SUCCESSFUL",
			},
			expectError: false,
		},
		{
			name:          "successful npm tests",
			sandboxID:     "sandbox-npm-test",
			testFramework: "npm",
			mockOutput:    "PASS  src/App.test.js\n✓ renders without crashing (23ms)",
			expectedResult: TestValidationResult{
				Success:       true,
				TestFramework: "npm",
				TestCommand:   "npm test",
				Output:        "PASS  src/App.test.js\n✓ renders without crashing (23ms)",
			},
			expectError: false,
		},
		{
			name:          "successful Go tests",
			sandboxID:     "sandbox-go-test",
			testFramework: "go",
			mockOutput:    "PASS\nok  \tgithub.com/example/project\t0.123s",
			expectedResult: TestValidationResult{
				Success:       true,
				TestFramework: "go",
				TestCommand:   "go test -v ./...",
				Output:        "PASS\nok  \tgithub.com/example/project\t0.123s",
			},
			expectError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			validator := &SandboxValidator{
				sandboxMgr: &MockValidationSandboxManager{
					mockOutput: tt.mockOutput,
				},
			}

			config := TestConfig{
				TestFramework: tt.testFramework,
				Timeout:       60 * time.Second,
			}

			result, err := validator.RunTests(context.Background(), tt.sandboxID, config)

			if tt.expectError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.expectedResult.Success, result.Success)
				assert.Equal(t, tt.expectedResult.TestFramework, result.TestFramework)
				assert.Equal(t, tt.expectedResult.TotalTests, result.TotalTests)
				assert.Equal(t, tt.expectedResult.PassedTests, result.PassedTests)
			}
		})
	}
}

func TestExecuteInSandbox(t *testing.T) {
	tests := []struct {
		name        string
		sandboxID   string
		command     string
		args        []string
		mockOutput  string
		mockError   error
		expectError bool
	}{
		{
			name:        "successful command execution",
			sandboxID:   "sandbox-exec-1",
			command:     "echo",
			args:        []string{"hello", "world"},
			mockOutput:  "hello world\n",
			expectError: false,
		},
		{
			name:        "command with error",
			sandboxID:   "sandbox-exec-2",
			command:     "false",
			args:        []string{},
			mockOutput:  "",
			expectError: true,
		},
		{
			name:        "command timeout",
			sandboxID:   "sandbox-exec-3",
			command:     "sleep",
			args:        []string{"60"},
			mockOutput:  "",
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			validator := &SandboxValidator{
				sandboxMgr: &MockValidationSandboxManager{
					mockOutput:    tt.mockOutput,
					shouldTimeout: tt.name == "command timeout",
				},
			}

			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()

			output, err := validator.ExecuteInSandbox(ctx, tt.sandboxID, tt.command, tt.args...)

			if tt.expectError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.mockOutput, output)
			}
		})
	}
}

func TestParseBuildOutput(t *testing.T) {
	tests := []struct {
		name           string
		buildTool      string
		output         string
		expectedErrors []ValidationBuildError
	}{
		{
			name:      "Maven compilation errors",
			buildTool: "maven",
			output: `[ERROR] /src/main/java/App.java:[10,5] cannot find symbol
[ERROR]   symbol:   class NonExistent
[ERROR]   location: class com.example.App`,
			expectedErrors: []ValidationBuildError{
				{
					Type:    "compilation",
					File:    "/src/main/java/App.java",
					Line:    10,
					Column:  5,
					Message: "cannot find symbol",
				},
			},
		},
		{
			name:      "Gradle compilation errors",
			buildTool: "gradle",
			output: `App.java:15: error: cannot find symbol
        NonExistent obj = new NonExistent();
        ^
  symbol:   class NonExistent`,
			expectedErrors: []ValidationBuildError{
				{
					Type:    "compilation",
					File:    "App.java",
					Line:    15,
					Message: "error: cannot find symbol",
				},
			},
		},
		{
			name:      "Go compilation errors",
			buildTool: "go",
			output: `./main.go:10:2: undefined: fmt.Printlnx
./main.go:15:5: cannot use x (type int) as type string`,
			expectedErrors: []ValidationBuildError{
				{
					Type:    "compilation",
					File:    "./main.go",
					Line:    10,
					Column:  2,
					Message: "undefined: fmt.Printlnx",
				},
				{
					Type:    "compilation",
					File:    "./main.go",
					Line:    15,
					Column:  5,
					Message: "cannot use x (type int) as type string",
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			validator := &SandboxValidator{}
			errors := validator.ParseBuildOutput(tt.buildTool, tt.output)

			assert.Equal(t, len(tt.expectedErrors), len(errors))
			for i, expectedErr := range tt.expectedErrors {
				if i < len(errors) {
					assert.Equal(t, expectedErr.Type, errors[i].Type)
					assert.Equal(t, expectedErr.File, errors[i].File)
					assert.Equal(t, expectedErr.Line, errors[i].Line)
					assert.Contains(t, errors[i].Message, strings.Split(expectedErr.Message, ":")[0])
				}
			}
		})
	}
}

// MockValidationSandboxManager is a mock for testing
type MockValidationSandboxManager struct {
	mockOutput    string
	shouldTimeout bool
	shouldFail    bool
}

func (m *MockValidationSandboxManager) CreateSandbox(ctx context.Context, config SandboxConfig) (*Sandbox, error) {
	return &Sandbox{ID: "mock-sandbox"}, nil
}

func (m *MockValidationSandboxManager) DestroySandbox(ctx context.Context, sandboxID string) error {
	return nil
}

func (m *MockValidationSandboxManager) ListSandboxes(ctx context.Context) ([]SandboxInfo, error) {
	return []SandboxInfo{}, nil
}

func (m *MockValidationSandboxManager) CleanupExpiredSandboxes(ctx context.Context) error {
	return nil
}

func (m *MockValidationSandboxManager) ExecuteCommand(ctx context.Context, sandboxID string, command string, args ...string) (string, error) {
	if m.shouldTimeout {
		select {
		case <-ctx.Done():
			return "", ctx.Err()
		case <-time.After(10 * time.Second):
			return "", context.DeadlineExceeded
		}
	}

	if m.shouldFail {
		return m.mockOutput, assert.AnError
	}

	return m.mockOutput, nil
}
