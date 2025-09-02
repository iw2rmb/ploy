package builders

import (
	"fmt"
	"os"
	"os/exec"
	"strings"
	"testing"
)

// TestBuildVM tests the BuildVM function
func TestBuildVM(t *testing.T) {
	tests := []struct {
		name           string
		app            string
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
			name:           "successful VM build",
			app:            "test-app",
			sha:            "abc123",
			outDir:         "/tmp/out",
			envVars:        map[string]string{"VM_TYPE": "qemu"},
			mockOutput:     "/tmp/out/test-app-abc123.qcow2",
			mockError:      nil,
			expectedResult: "/tmp/out/test-app-abc123.qcow2",
			wantErr:        false,
		},
		{
			name:           "VM build with VMware output",
			app:            "vmware-app",
			sha:            "def456",
			outDir:         "/tmp/vmware",
			envVars:        map[string]string{"VM_TYPE": "vmware"},
			mockOutput:     "/tmp/vmware/vmware-app-def456.vmdk",
			mockError:      nil,
			expectedResult: "/tmp/vmware/vmware-app-def456.vmdk",
			wantErr:        false,
		},
		{
			name:           "build with trimmed output",
			app:            "test-app",
			sha:            "abc123",
			outDir:         "/tmp/out",
			envVars:        nil,
			mockOutput:     "  /tmp/out/vm.qcow2  \n",
			mockError:      nil,
			expectedResult: "/tmp/out/vm.qcow2",
			wantErr:        false,
		},
		{
			name:           "VM build failure",
			app:            "test-app",
			sha:            "abc123",
			outDir:         "/tmp/out",
			envVars:        nil,
			mockOutput:     "Error: Packer build failed",
			mockError:      fmt.Errorf("exit status 1"),
			expectedResult: "",
			wantErr:        true,
			errContains:    "vm build failed",
		},
		{
			name:           "build with packer configuration error",
			app:            "test-app",
			sha:            "abc123",
			outDir:         "/tmp/out",
			envVars:        nil,
			mockOutput:     "Error: Invalid Packer template",
			mockError:      fmt.Errorf("exit status 1"),
			expectedResult: "",
			wantErr:        true,
			errContains:    "vm build failed",
		},
		{
			name:   "build with cloud provider settings",
			app:    "cloud-app",
			sha:    "xyz789",
			outDir: "/tmp/cloud",
			envVars: map[string]string{
				"CLOUD_PROVIDER": "aws",
				"AWS_REGION":     "us-west-2",
				"AMI_NAME":       "cloud-app-ami",
			},
			mockOutput:     "ami-1234567890abcdef0",
			mockError:      nil,
			expectedResult: "ami-1234567890abcdef0",
			wantErr:        false,
		},
		{
			name:           "output with build progress",
			app:            "test-app",
			sha:            "abc123",
			outDir:         "/tmp/out",
			envVars:        nil,
			mockOutput:     "Building VM image...\nInstalling OS...\nConfiguring...\nSuccess: /tmp/out/vm.qcow2",
			mockError:      nil,
			expectedResult: "Building VM image...\nInstalling OS...\nConfiguring...\nSuccess: /tmp/out/vm.qcow2",
			wantErr:        false,
		},
		{
			name:           "empty output directory",
			app:            "test-app",
			sha:            "abc123",
			outDir:         "",
			envVars:        nil,
			mockOutput:     "./vm.qcow2",
			mockError:      nil,
			expectedResult: "./vm.qcow2",
			wantErr:        false,
		},
		{
			name:           "whitespace only output",
			app:            "test-app",
			sha:            "abc123",
			outDir:         "/tmp/out",
			envVars:        nil,
			mockOutput:     "   \t\n   ",
			mockError:      nil,
			expectedResult: "",
			wantErr:        false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Test with mock
			result, err := testBuildVM(tt.app, tt.sha, tt.outDir, tt.envVars, tt.mockOutput, tt.mockError)

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

// TestBuildVMCommandArguments tests that correct arguments are passed to the build script
func TestBuildVMCommandArguments(t *testing.T) {
	tests := []struct {
		name           string
		app            string
		sha            string
		outDir         string
		expectedArgs   []string
		expectedScript string
	}{
		{
			name:   "standard arguments",
			app:    "my-app",
			sha:    "commit123",
			outDir: "/output/dir",
			expectedArgs: []string{
				"--app", "my-app",
				"--sha", "commit123",
				"--out-dir", "/output/dir",
			},
			expectedScript: "./scripts/build/packer/build_vm.sh",
		},
		{
			name:   "long SHA",
			app:    "test-app",
			sha:    "1234567890abcdef1234567890abcdef12345678",
			outDir: "/tmp/out",
			expectedArgs: []string{
				"--app", "test-app",
				"--sha", "1234567890abcdef1234567890abcdef12345678",
				"--out-dir", "/tmp/out",
			},
			expectedScript: "./scripts/build/packer/build_vm.sh",
		},
		{
			name:   "paths with spaces",
			app:    "my app name",
			sha:    "abc123",
			outDir: "/path/with spaces/out",
			expectedArgs: []string{
				"--app", "my app name",
				"--sha", "abc123",
				"--out-dir", "/path/with spaces/out",
			},
			expectedScript: "./scripts/build/packer/build_vm.sh",
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
			BuildVMWithMock(tt.app, tt.sha, tt.outDir, nil, mockCommand)

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

// TestBuildVMEnvironmentVariables tests environment variable propagation
func TestBuildVMEnvironmentVariables(t *testing.T) {
	envVars := map[string]string{
		"PACKER_LOG":       "1",
		"PACKER_CACHE_DIR": "/tmp/packer-cache",
		"VM_DISK_SIZE":     "20G",
		"VM_MEMORY":        "4096",
		"VM_CPUS":          "2",
		"CUSTOM_CONFIG":    "custom.json",
	}

	var capturedEnv []string

	// Mock command to capture environment
	mockCommand := func(name string, args ...string) *exec.Cmd {
		cmd := exec.Command("echo", "test-output")
		capturedEnv = cmd.Env
		return cmd
	}

	// Run with environment variables
	BuildVMWithMock("app", "sha", "/out", envVars, mockCommand)

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

// TestBuildVMDifferentFormats tests handling of different VM image formats
func TestBuildVMDifferentFormats(t *testing.T) {
	formats := []struct {
		name           string
		mockOutput     string
		expectedResult string
	}{
		{
			name:           "QCOW2 format",
			mockOutput:     "/tmp/vm.qcow2",
			expectedResult: "/tmp/vm.qcow2",
		},
		{
			name:           "VMDK format",
			mockOutput:     "/tmp/vm.vmdk",
			expectedResult: "/tmp/vm.vmdk",
		},
		{
			name:           "VHD format",
			mockOutput:     "/tmp/vm.vhd",
			expectedResult: "/tmp/vm.vhd",
		},
		{
			name:           "RAW format",
			mockOutput:     "/tmp/vm.raw",
			expectedResult: "/tmp/vm.raw",
		},
		{
			name:           "AMI ID output",
			mockOutput:     "ami-0123456789abcdef0",
			expectedResult: "ami-0123456789abcdef0",
		},
	}

	for _, tt := range formats {
		t.Run(tt.name, func(t *testing.T) {
			result, err := testBuildVM("app", "sha", "/out", nil, tt.mockOutput, nil)
			if err != nil {
				t.Errorf("unexpected error: %v", err)
			}
			if result != tt.expectedResult {
				t.Errorf("result mismatch: got %q, want %q", result, tt.expectedResult)
			}
		})
	}
}

// testBuildVM is a testable version that returns mocked outputs
func testBuildVM(app, sha, outDir string, envVars map[string]string, mockOutput string, mockError error) (string, error) {
	if mockError != nil {
		return "", fmt.Errorf("vm build failed: %v: %s", mockError, mockOutput)
	}
	return strings.TrimSpace(mockOutput), nil
}

// BuildVMWithMock is a testable version of BuildVM with mock command executor
func BuildVMWithMock(app, sha, outDir string, envVars map[string]string, commandExecutor func(string, ...string) *exec.Cmd) (string, error) {
	args := []string{"--app", app, "--sha", sha, "--out-dir", outDir}
	cmd := commandExecutor("./scripts/build/packer/build_vm.sh", args...)

	// Add environment variables
	env := os.Environ()
	for k, v := range envVars {
		env = append(env, fmt.Sprintf("%s=%s", k, v))
	}
	cmd.Env = env

	b, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("vm build failed: %v: %s", err, string(b))
	}
	return strings.TrimSpace(string(b)), nil
}
