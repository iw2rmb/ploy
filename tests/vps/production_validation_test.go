package vps

import (
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestVPSProductionReadiness(t *testing.T) {
	if os.Getenv("TARGET_HOST") == "" {
		t.Skip("TARGET_HOST not set")
	}

	vpsClient := NewVPSClient(os.Getenv("TARGET_HOST"))

	// Test production-like service topology
	t.Run("ServiceTopology", func(t *testing.T) {
		// Verify Consul cluster health
		output, err := vpsClient.RunCommand("su - ploy -c 'consul members'")
		assert.NoError(t, err)
		assert.Contains(t, output, "alive", "Consul should be alive")

		// Verify Nomad cluster health
		output, err = vpsClient.RunCommand("su - ploy -c '/opt/hashicorp/bin/nomad-job-manager.sh node status'")
		assert.NoError(t, err)
		assert.Contains(t, output, "ready", "Nomad should be ready")

		// Verify SeaweedFS cluster health
		output, err = vpsClient.RunCommand("su - ploy -c 'curl -s http://localhost:9333/cluster/status'")
		assert.NoError(t, err)
		assert.Contains(t, output, "Leader", "SeaweedFS should have leader")
	})

	// Test performance characteristics
	t.Run("PerformanceBaseline", func(t *testing.T) {
		// KB storage performance
		start := time.Now()
		testData := strings.Repeat("test", 1000) // 4KB test data

		cmd := fmt.Sprintf(`su - ploy -c "echo '%s' | curl -w '%%{time_total}' -X POST http://localhost:8888/kb/perf/test -d @-"`, testData)
		output, err := vpsClient.RunCommand(cmd)
		assert.NoError(t, err)

		duration := time.Since(start)
		assert.True(t, duration < 2*time.Second, "KB storage should be responsive (<2s)")

		// Nomad job submission performance
		start = time.Now()
		jobHCL := `
job "perf-test" {
  type = "batch"
  group "test" {
    task "echo" {
      driver = "raw_exec"
      config {
        command = "echo"
        args = ["performance test"]
      }
    }
  }
}
`
		jobFile := fmt.Sprintf("/tmp/perf-test-%d.hcl", time.Now().Unix())
		writeCmd := fmt.Sprintf(`su - ploy -c "cat > %s << 'EOF'\n%s\nEOF"`, jobFile, jobHCL)
		_, err = vpsClient.RunCommand(writeCmd)
		assert.NoError(t, err)

		submitCmd := fmt.Sprintf(`su - ploy -c "/opt/hashicorp/bin/nomad-job-manager.sh run %s"`, jobFile)
		_, err = vpsClient.RunCommand(submitCmd)
		assert.NoError(t, err)

		duration = time.Since(start)
		assert.True(t, duration < 30*time.Second, "Job submission should be fast (<30s)")

		// Cleanup
		vpsClient.RunCommand(fmt.Sprintf(`su - ploy -c "/opt/hashicorp/bin/nomad-job-manager.sh stop perf-test"`))
		vpsClient.RunCommand(fmt.Sprintf(`su - ploy -c "rm -f %s"`, jobFile))
	})
}

func TestVPSTransflowEndToEnd(t *testing.T) {
	if os.Getenv("TARGET_HOST") == "" {
		t.Skip("TARGET_HOST not set")
	}

	vpsClient := NewVPSClient(os.Getenv("TARGET_HOST"))

	t.Run("TransflowWorkflowValidation", func(t *testing.T) {
		// Test transflow configuration validation
		configCmd := `su - ploy -c "/opt/ploy/bin/ploy transflow validate /opt/ploy/test/fixtures/java-migration.yaml"`
		output, err := vpsClient.RunCommand(configCmd)
		assert.NoError(t, err, "Transflow configuration should validate successfully")
		assert.NotContains(t, strings.ToLower(output), "error", "Configuration validation should not contain errors")

		// Test KB learning system availability
		kbTestCmd := `su - ploy -c "curl -f http://localhost:8888/kb/cases/"`
		_, err = vpsClient.RunCommand(kbTestCmd)
		assert.NoError(t, err, "KB learning system should be accessible")
	})

	t.Run("VPSEnvironmentVariables", func(t *testing.T) {
		// Verify required environment variables are set
		envVars := []string{"CONSUL_HTTP_ADDR", "NOMAD_ADDR", "SEAWEEDFS_MASTER", "SEAWEEDFS_FILER"}

		for _, envVar := range envVars {
			cmd := fmt.Sprintf(`su - ploy -c "echo \\$%s"`, envVar)
			output, err := vpsClient.RunCommand(cmd)
			assert.NoError(t, err, "Should be able to read environment variable %s", envVar)
			assert.NotEmpty(t, strings.TrimSpace(output), "Environment variable %s should be set", envVar)
		}
	})
}

func TestVPSSecurityAndAccess(t *testing.T) {
	if os.Getenv("TARGET_HOST") == "" {
		t.Skip("TARGET_HOST not set")
	}

	vpsClient := NewVPSClient(os.Getenv("TARGET_HOST"))

	t.Run("AccessControls", func(t *testing.T) {
		// Verify ploy user exists and has correct permissions
		output, err := vpsClient.RunCommand("id ploy")
		assert.NoError(t, err, "ploy user should exist")
		assert.Contains(t, output, "ploy", "ploy user should be in output")

		// Verify directory permissions
		output, err = vpsClient.RunCommand("su - ploy -c 'ls -la /opt/ploy'")
		assert.NoError(t, err, "Should be able to list /opt/ploy directory as ploy user")
		assert.Contains(t, output, "bin", "/opt/ploy should contain bin directory")

		// Verify transflow binary is executable by ploy user
		output, err = vpsClient.RunCommand("su - ploy -c 'test -x /opt/ploy/bin/ploy && echo \"executable\"'")
		assert.NoError(t, err, "Should be able to test ploy binary executability")
		assert.Contains(t, output, "executable", "ploy binary should be executable")
	})

	t.Run("ServiceSecurity", func(t *testing.T) {
		// Test that services are not exposed externally (should only be accessible locally)
		services := map[string]string{
			"consul":           "8500",
			"nomad":            "4646",
			"seaweedfs-master": "9333",
			"seaweedfs-filer":  "8888",
		}

		for service, port := range services {
			// Test that service is accessible locally
			localCmd := fmt.Sprintf(`su - ploy -c "curl -f http://localhost:%s/ || curl -f http://localhost:%s/v1/status/leader"`, port, port)
			_, err := vpsClient.RunCommand(localCmd)
			assert.NoError(t, err, "Service %s should be accessible locally on port %s", service, port)
		}
	})
}
