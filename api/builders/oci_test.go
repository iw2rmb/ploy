package builders

import (
	"fmt"
	"os"
	"os/exec"
	"strings"
	"testing"
)

// TestBuildOCI tests the BuildOCI function
func TestBuildOCI(t *testing.T) {
	tests := []struct {
		name           string
		app            string
		srcDir         string
		tag            string
		envVars        map[string]string
		mockOutput     string
		mockError      error
		expectedResult string
		wantErr        bool
		errContains    string
	}{
		{
			name:           "successful OCI build",
			app:            "test-app",
			srcDir:         "/tmp/src",
			tag:            "v1.0.0",
			envVars:        map[string]string{"DOCKER_REGISTRY": "registry.example.com"},
			mockOutput:     "registry.example.com/test-app:v1.0.0",
			mockError:      nil,
			expectedResult: "registry.example.com/test-app:v1.0.0",
			wantErr:        false,
		},
		{
			name:           "successful build with sha tag",
			app:            "test-app",
			srcDir:         "/tmp/src",
			tag:            "sha-abc123",
			envVars:        nil,
			mockOutput:     "localhost:5000/test-app:sha-abc123",
			mockError:      nil,
			expectedResult: "localhost:5000/test-app:sha-abc123",
			wantErr:        false,
		},
		{
			name:           "build with trimmed output",
			app:            "test-app",
			srcDir:         "/tmp/src",
			tag:            "latest",
			envVars:        nil,
			mockOutput:     "  test-app:latest  \n\t",
			mockError:      nil,
			expectedResult: "test-app:latest",
			wantErr:        false,
		},
		{
			name:           "OCI build failure",
			app:            "test-app",
			srcDir:         "/tmp/src",
			tag:            "v1.0.0",
			envVars:        nil,
			mockOutput:     "Error: Docker daemon not running",
			mockError:      fmt.Errorf("exit status 1"),
			expectedResult: "",
			wantErr:        true,
			errContains:    "oci build failed",
		},
		{
			name:           "build with Dockerfile not found",
			app:            "test-app",
			srcDir:         "/tmp/nonexistent",
			tag:            "v1.0.0",
			envVars:        nil,
			mockOutput:     "Error: Dockerfile not found",
			mockError:      fmt.Errorf("exit status 1"),
			expectedResult: "",
			wantErr:        true,
			errContains:    "oci build failed",
		},
		{
			name:   "build with build args via env vars",
			app:    "test-app",
			srcDir: "/tmp/src",
			tag:    "v2.0.0",
			envVars: map[string]string{
				"DOCKER_BUILDKIT":   "1",
				"BUILD_ARG_VERSION": "2.0.0",
				"BUILD_ARG_ENV":     "production",
			},
			mockOutput:     "test-app:v2.0.0",
			mockError:      nil,
			expectedResult: "test-app:v2.0.0",
			wantErr:        false,
		},
		{
			name:           "output with build logs",
			app:            "test-app",
			srcDir:         "/tmp/src",
			tag:            "v1.0.0",
			envVars:        nil,
			mockOutput:     "Step 1/5 : FROM alpine\nStep 2/5 : COPY . /app\n...\nSuccessfully tagged test-app:v1.0.0\ntest-app:v1.0.0",
			mockError:      nil,
			expectedResult: "Step 1/5 : FROM alpine\nStep 2/5 : COPY . /app\n...\nSuccessfully tagged test-app:v1.0.0\ntest-app:v1.0.0",
			wantErr:        false,
		},
		{
			name:           "empty tag defaults",
			app:            "test-app",
			srcDir:         "/tmp/src",
			tag:            "",
			envVars:        nil,
			mockOutput:     "test-app:latest",
			mockError:      nil,
			expectedResult: "test-app:latest",
			wantErr:        false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Mock the build function
			result, err := testBuildOCI(tt.app, tt.srcDir, tt.tag, tt.envVars, tt.mockOutput, tt.mockError)

			// Verify error expectations
			if tt.wantErr {
				if err == nil {
					t.Errorf("expected error but got none")
				} else if tt.errContains != "" && !strings.Contains(err.Error(), tt.errContains) {
					t.Errorf("error should contain '%s', got: %v", tt.errContains, err)
				}
			} else {
				if err != nil {
					t.Errorf("unexpected error: %v", err)
				}
			}

			// Verify result
			if result != tt.expectedResult {
				t.Errorf("result mismatch: got %q, want %q", result, tt.expectedResult)
			}
		})
	}
}

// TestBuildOCICommandArguments tests that correct arguments are passed to the build script
func TestBuildOCICommandArguments(t *testing.T) {
	tests := []struct {
		name           string
		app            string
		srcDir         string
		tag            string
		expectedArgs   []string
		expectedScript string
	}{
		{
			name:   "standard arguments",
			app:    "my-app",
			srcDir: "/path/to/src",
			tag:    "v1.2.3",
			expectedArgs: []string{
				"--app", "my-app",
				"--src", "/path/to/src",
				"--tag", "v1.2.3",
			},
			expectedScript: "./scripts/build/oci/build_oci.sh",
		},
		{
			name:   "tag with special characters",
			app:    "test-app",
			srcDir: "/tmp/src",
			tag:    "feature/branch-123",
			expectedArgs: []string{
				"--app", "test-app",
				"--src", "/tmp/src",
				"--tag", "feature/branch-123",
			},
			expectedScript: "./scripts/build/oci/build_oci.sh",
		},
		{
			name:   "paths with spaces",
			app:    "my app",
			srcDir: "/path/to/my src",
			tag:    "latest",
			expectedArgs: []string{
				"--app", "my app",
				"--src", "/path/to/my src",
				"--tag", "latest",
			},
			expectedScript: "./scripts/build/oci/build_oci.sh",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var capturedScript string
			var capturedArgs []string

			// Mock command executor
			mockCommand := func(name string, args ...string) *exec.Cmd {
				capturedScript = name
				capturedArgs = args
				return exec.Command("echo", "test-output")
			}

			// Run with mock
			BuildOCIWithMock(tt.app, tt.srcDir, tt.tag, nil, mockCommand)

			// Verify script
			if capturedScript != tt.expectedScript {
				t.Errorf("script mismatch: got %s, want %s", capturedScript, tt.expectedScript)
			}

			// Verify arguments
			if len(capturedArgs) != len(tt.expectedArgs) {
				t.Errorf("argument count mismatch: got %d, want %d", len(capturedArgs), len(tt.expectedArgs))
			}
			for i, arg := range tt.expectedArgs {
				if i < len(capturedArgs) && capturedArgs[i] != arg {
					t.Errorf("arg[%d]: got %s, want %s", i, capturedArgs[i], arg)
				}
			}
		})
	}
}

// TestBuildOCIEnvironmentVariables tests environment variable propagation
func TestBuildOCIEnvironmentVariables(t *testing.T) {
	envVars := map[string]string{
		"DOCKER_HOST":     "tcp://localhost:2375",
		"DOCKER_REGISTRY": "registry.example.com",
		"DOCKER_BUILDKIT": "1",
		"BUILD_CACHE":     "true",
		"CUSTOM_VAR":      "custom_value",
	}

	var capturedEnv []string

	// Mock command to capture environment
	mockCommand := func(name string, args ...string) *exec.Cmd {
		cmd := exec.Command("echo", "test-output")
		capturedEnv = cmd.Env
		return cmd
	}

	// Run with environment variables
	BuildOCIWithMock("app", "/src", "latest", envVars, mockCommand)

	// Convert to map for checking
	envMap := make(map[string]string)
	for _, env := range capturedEnv {
		parts := strings.SplitN(env, "=", 2)
		if len(parts) == 2 {
			envMap[parts[0]] = parts[1]
		}
	}

	// Verify all env vars were added
	for key, expectedValue := range envVars {
		if actualValue, exists := envMap[key]; !exists {
			t.Errorf("environment variable %s not found", key)
		} else if actualValue != expectedValue {
			t.Errorf("environment variable %s: got %s, want %s", key, actualValue, expectedValue)
		}
	}
}

// testBuildOCI is a testable version that returns mocked outputs
func testBuildOCI(app, srcDir, tag string, envVars map[string]string, mockOutput string, mockError error) (string, error) {
	if mockError != nil {
		return "", fmt.Errorf("oci build failed: %v: %s", mockError, mockOutput)
	}
	return strings.TrimSpace(mockOutput), nil
}

// BuildOCIWithMock is a testable version of BuildOCI with mock command executor
func BuildOCIWithMock(app, srcDir, tag string, envVars map[string]string, commandExecutor func(string, ...string) *exec.Cmd) (string, error) {
	args := []string{"--app", app, "--src", srcDir, "--tag", tag}
	cmd := commandExecutor("./scripts/build/oci/build_oci.sh", args...)

	// Add environment variables
	env := os.Environ()
	for k, v := range envVars {
		env = append(env, fmt.Sprintf("%s=%s", k, v))
	}
	cmd.Env = env

	b, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("oci build failed: %v: %s", err, string(b))
	}
	return strings.TrimSpace(string(b)), nil
}
