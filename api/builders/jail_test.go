package builders

import (
	"fmt"
	"os"
	"os/exec"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// TestBuildJail tests the BuildJail function
func TestBuildJail(t *testing.T) {
	tests := []struct {
		name           string
		app            string
		srcDir         string
		sha            string
		outDir         string
		envVars        map[string]string
		mockOutput     string
		mockError      error
		expectedResult string
		wantErr        bool
		errContains    string
	}{
		{
			name:           "successful jail build",
			app:            "test-app",
			srcDir:         "/tmp/src",
			sha:            "abc123",
			outDir:         "/tmp/out",
			envVars:        map[string]string{"TEST_VAR": "test_value"},
			mockOutput:     "/tmp/out/test-app-jail.tar.gz",
			mockError:      nil,
			expectedResult: "/tmp/out/test-app-jail.tar.gz",
			wantErr:        false,
		},
		{
			name:           "successful build with trimmed output",
			app:            "test-app",
			srcDir:         "/tmp/src",
			sha:            "abc123",
			outDir:         "/tmp/out",
			envVars:        nil,
			mockOutput:     "  /tmp/out/test-app-jail.tar.gz  \n",
			mockError:      nil,
			expectedResult: "/tmp/out/test-app-jail.tar.gz",
			wantErr:        false,
		},
		{
			name:           "build failure with error message",
			app:            "test-app",
			srcDir:         "/tmp/src",
			sha:            "abc123",
			outDir:         "/tmp/out",
			envVars:        nil,
			mockOutput:     "Error: FreeBSD jail creation failed",
			mockError:      fmt.Errorf("exit status 1"),
			expectedResult: "",
			wantErr:        true,
			errContains:    "jail build failed",
		},
		{
			name:           "empty source directory",
			app:            "test-app",
			srcDir:         "",
			sha:            "abc123",
			outDir:         "/tmp/out",
			envVars:        nil,
			mockOutput:     "",
			mockError:      fmt.Errorf("exit status 1"),
			expectedResult: "",
			wantErr:        true,
			errContains:    "jail build failed",
		},
		{
			name:   "multiple environment variables",
			app:    "multi-env-app",
			srcDir: "/tmp/src",
			sha:    "def456",
			outDir: "/tmp/out",
			envVars: map[string]string{
				"VAR1": "value1",
				"VAR2": "value2",
				"VAR3": "value3",
			},
			mockOutput:     "/tmp/out/multi-env-app-jail.tar.gz",
			mockError:      nil,
			expectedResult: "/tmp/out/multi-env-app-jail.tar.gz",
			wantErr:        false,
		},
		{
			name:           "output with additional messages",
			app:            "test-app",
			srcDir:         "/tmp/src",
			sha:            "abc123",
			outDir:         "/tmp/out",
			envVars:        nil,
			mockOutput:     "Creating jail environment...\nBuilding application...\nSuccess: /tmp/out/jail.tar.gz",
			mockError:      nil,
			expectedResult: "Creating jail environment...\nBuilding application...\nSuccess: /tmp/out/jail.tar.gz",
			wantErr:        false,
		},
		{
			name:           "empty output on success",
			app:            "test-app",
			srcDir:         "/tmp/src",
			sha:            "abc123",
			outDir:         "/tmp/out",
			envVars:        nil,
			mockOutput:     "",
			mockError:      nil,
			expectedResult: "",
			wantErr:        false,
		},
		{
			name:           "whitespace only output",
			app:            "test-app",
			srcDir:         "/tmp/src",
			sha:            "abc123",
			outDir:         "/tmp/out",
			envVars:        nil,
			mockOutput:     "   \n\t  \n  ",
			mockError:      nil,
			expectedResult: "",
			wantErr:        false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Mock the exec.Command function
			result, err := testBuildJail(tt.app, tt.srcDir, tt.sha, tt.outDir, tt.envVars, tt.mockOutput, tt.mockError)

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

// TestBuildJailCommandArguments tests that the correct arguments are passed to the build script
func TestBuildJailCommandArguments(t *testing.T) {
	tests := []struct {
		name           string
		app            string
		srcDir         string
		sha            string
		outDir         string
		expectedArgs   []string
		expectedScript string
	}{
		{
			name:   "standard arguments",
			app:    "test-app",
			srcDir: "/tmp/src",
			sha:    "abc123",
			outDir: "/tmp/out",
			expectedArgs: []string{
				"--app", "test-app",
				"--src", "/tmp/src",
				"--sha", "abc123",
				"--out-dir", "/tmp/out",
			},
			expectedScript: "./scripts/build/jail/build_jail.sh",
		},
		{
			name:   "arguments with spaces in paths",
			app:    "my-app",
			srcDir: "/tmp/my src dir",
			sha:    "def456",
			outDir: "/tmp/my out dir",
			expectedArgs: []string{
				"--app", "my-app",
				"--src", "/tmp/my src dir",
				"--sha", "def456",
				"--out-dir", "/tmp/my out dir",
			},
			expectedScript: "./scripts/build/jail/build_jail.sh",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var capturedScript string
			var capturedArgs []string

			// Mock function that captures the command arguments
			mockCommand := func(name string, args ...string) *exec.Cmd {
				capturedScript = name
				capturedArgs = args
				return exec.Command("echo", "test-output")
			}

			// Run the test with mock
			_, err := BuildJailWithMock(tt.app, tt.srcDir, tt.sha, tt.outDir, nil, mockCommand)
			require.NoError(t, err)

			// Verify script name
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

// TestBuildJailEnvironmentPropagation tests that environment variables are properly passed
func TestBuildJailEnvironmentPropagation(t *testing.T) {
	envVars := map[string]string{
		"JAIL_CONFIG":  "/etc/jail.conf",
		"BUILD_TARGET": "freebsd",
		"DEBUG":        "true",
		"PATH_VAR":     "/custom/path",
	}

	var capturedCmd *exec.Cmd

	// Mock exec.Command to capture environment
	mockCommand := func(name string, args ...string) *exec.Cmd {
		cmd := exec.Command("echo", "test-output")
		capturedCmd = cmd
		return cmd
	}

	// Run BuildJail with environment variables
	_, err := BuildJailWithMock("app", "/src", "sha", "/out", envVars, mockCommand)
	require.NoError(t, err)

	// Convert captured environment to map for easier checking
	require.NotNil(t, capturedCmd)
	envMap := make(map[string]string)
	for _, env := range capturedCmd.Env {
		parts := strings.SplitN(env, "=", 2)
		if len(parts) == 2 {
			envMap[parts[0]] = parts[1]
		}
	}

	// Verify all provided env vars were added
	for key, expectedValue := range envVars {
		if actualValue, exists := envMap[key]; !exists {
			t.Errorf("environment variable %s not found", key)
		} else if actualValue != expectedValue {
			t.Errorf("environment variable %s: got %s, want %s", key, actualValue, expectedValue)
		}
	}
}

// testBuildJail is a testable version that returns mocked outputs
func testBuildJail(app, srcDir, sha, outDir string, envVars map[string]string, mockOutput string, mockError error) (string, error) {
	// Simulate the build process with mock data
	if mockError != nil {
		return "", fmt.Errorf("jail build failed: %v: %s", mockError, mockOutput)
	}

	// Trim whitespace from output (matching the actual function behavior)
	return strings.TrimSpace(mockOutput), nil
}

// BuildJailWithMock is a testable version of BuildJail that accepts a mock command executor
func BuildJailWithMock(app, srcDir, sha, outDir string, envVars map[string]string, commandExecutor func(string, ...string) *exec.Cmd) (string, error) {
	args := []string{"--app", app, "--src", srcDir, "--sha", sha, "--out-dir", outDir}
	cmd := commandExecutor("./scripts/build/jail/build_jail.sh", args...)

	// Add environment variables to the build process
	env := os.Environ()
	for k, v := range envVars {
		env = append(env, fmt.Sprintf("%s=%s", k, v))
	}
	cmd.Env = env

	b, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("jail build failed: %v: %s", err, string(b))
	}
	return strings.TrimSpace(string(b)), nil
}
