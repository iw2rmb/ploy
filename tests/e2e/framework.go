package e2e

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"
)

type TestEnvironment struct {
	Config       Config
	TransflowCLI *TransflowCLI
	cleanup      []func()
}

type Config struct {
	UseRealServices bool
	CleanupAfter    bool
	InjectFailures  bool
	TimeoutMinutes  int
}

func SetupTestEnvironment(t *testing.T, config Config) *TestEnvironment {
	env := &TestEnvironment{Config: config}

	if config.UseRealServices {
		env.setupRealServices(t)
	} else {
		env.setupMockServices(t)
	}

	return env
}

func (env *TestEnvironment) setupRealServices(t *testing.T) {
	if os.Getenv("TARGET_HOST") != "" {
		env.setupVPSServices(t)
	} else {
		env.setupLocalServices(t)
	}
}

func (env *TestEnvironment) setupLocalServices(t *testing.T) {
	env.TransflowCLI = &TransflowCLI{
		binaryPath: "./bin/ploy",
		env: map[string]string{
			"CONSUL_HTTP_ADDR": "localhost:8500",
			"NOMAD_ADDR":       "http://localhost:4646",
			"SEAWEEDFS_MASTER": "http://localhost:9333",
			"SEAWEEDFS_FILER":  "http://localhost:8888",
		},
	}
}

func (env *TestEnvironment) setupVPSServices(t *testing.T) {
	targetHost := os.Getenv("TARGET_HOST")
	env.TransflowCLI = &TransflowCLI{
		binaryPath: "./bin/ploy-linux",
		env: map[string]string{
			"TARGET_HOST": targetHost,
		},
	}
}

func (env *TestEnvironment) setupMockServices(t *testing.T) {
	env.TransflowCLI = &TransflowCLI{
		binaryPath: "./bin/ploy",
		env: map[string]string{
			"TRANSFLOW_TEST_MODE": "true",
		},
	}
}

func (env *TestEnvironment) ExecuteWorkflow(ctx context.Context, workflow *TransflowWorkflow) (WorkflowResult, error) {
	yamlContent, err := workflow.ToYAML()
	if err != nil {
		return WorkflowResult{}, fmt.Errorf("failed to generate workflow YAML: %w", err)
	}

	tempFile := filepath.Join(os.TempDir(), fmt.Sprintf("transflow-%s.yaml", workflow.ID))
	err = os.WriteFile(tempFile, []byte(yamlContent), 0644)
	if err != nil {
		return WorkflowResult{}, fmt.Errorf("failed to write workflow file: %w", err)
	}
	defer os.Remove(tempFile)

	start := time.Now()
	output, err := env.TransflowCLI.Run(ctx, "transflow", "run", "-f", tempFile)
	duration := time.Since(start)

	result := WorkflowResult{
		ID:       workflow.ID,
		Duration: duration,
		Success:  err == nil,
		Output:   output,
	}

	if err != nil {
		result.Error = err.Error()
	}

	result.parseFromOutput(output)
	return result, nil
}

func (env *TestEnvironment) Cleanup() {
	for _, cleanup := range env.cleanup {
		cleanup()
	}
}

type TransflowCLI struct {
	binaryPath string
	env        map[string]string
}

func (cli *TransflowCLI) Run(ctx context.Context, args ...string) (string, error) {
	if os.Getenv("TARGET_HOST") != "" {
		return cli.runVPS(ctx, args...)
	}
	return cli.runLocal(ctx, args...)
}

func (cli *TransflowCLI) runLocal(ctx context.Context, args ...string) (string, error) {
	cmd := exec.CommandContext(ctx, cli.binaryPath, args...)
	cmd.Env = os.Environ()
	for k, v := range cli.env {
		cmd.Env = append(cmd.Env, fmt.Sprintf("%s=%s", k, v))
	}
	output, err := cmd.CombinedOutput()
	return string(output), err
}

func (cli *TransflowCLI) runVPS(ctx context.Context, args ...string) (string, error) {
	targetHost := os.Getenv("TARGET_HOST")
	cmdArgs := append([]string{
		fmt.Sprintf("root@%s", targetHost),
		"su - ploy -c",
		fmt.Sprintf("'/opt/ploy/bin/ploy %s'", args[0])}, args[1:]...)

	if len(args) > 1 {
		cmdArgs[2] = fmt.Sprintf("'/opt/ploy/bin/ploy %s'", fmt.Sprintf("%s %s", args[0], args[1]))
		if len(args) > 2 {
			for _, arg := range args[2:] {
				cmdArgs[2] += " " + arg
			}
		}
	}

	cmd := exec.CommandContext(ctx, "ssh", "-o", "ConnectTimeout=30", cmdArgs[0], cmdArgs[1], cmdArgs[2])
	output, err := cmd.CombinedOutput()
	return string(output), err
}
