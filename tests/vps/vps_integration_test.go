//go:build vps
// +build vps

package vps

import (
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestVPSEnvironmentReadiness(t *testing.T) {
	// Should fail initially - VPS may not be configured

	if os.Getenv("TARGET_HOST") == "" {
		t.Skip("TARGET_HOST not set, skipping VPS tests")
	}

	vpsClient := NewVPSClient(os.Getenv("TARGET_HOST"))

	// Test VPS service health
	services := []string{"consul", "nomad", "seaweedfs-master", "seaweedfs-filer"}
	for _, service := range services {
		t.Run(fmt.Sprintf("service_%s", service), func(t *testing.T) {
			healthy, err := vpsClient.CheckServiceHealth(service)
			assert.NoError(t, err, "Should be able to check %s health", service)
			assert.True(t, healthy, "Service %s should be healthy on VPS", service)
		})
	}

	// Test transflow CLI availability
	output, err := vpsClient.RunCommand("su - ploy -c '/opt/ploy/bin/ploy --version'")
	assert.NoError(t, err, "Should be able to run ploy CLI on VPS")
	assert.Contains(t, output, "ploy version", "Ploy CLI should be installed")

	// Test transflow command availability
	output, err = vpsClient.RunCommand("su - ploy -c '/opt/ploy/bin/ploy mod --help'")
	assert.NoError(t, err, "Mods command should be available")
	assert.Contains(t, output, "transflow", "Mods subcommand should be available")
}

func TestVPSKBStorageSetup(t *testing.T) {
	// Should fail initially - KB namespace may not exist

	if os.Getenv("TARGET_HOST") == "" {
		t.Skip("TARGET_HOST not set, skipping VPS tests")
	}

	vpsClient := NewVPSClient(os.Getenv("TARGET_HOST"))

	// Test KB namespace creation
	cmd := `su - ploy -c "curl -X POST http://localhost:8888/kb/ -d 'mkdir kb namespace'"`
	_, err := vpsClient.RunCommand(cmd)
	assert.NoError(t, err, "Should be able to create KB namespace")

	// Test KB storage read/write
	testKey := fmt.Sprintf("test-case-%d", time.Now().Unix())
	testData := `{"test": "kb storage validation"}`

	// Write test data
	writeCmd := fmt.Sprintf(`su - ploy -c "echo '%s' | curl -X POST http://localhost:8888/kb/test/%s -d @-"`, testData, testKey)
	_, err = vpsClient.RunCommand(writeCmd)
	assert.NoError(t, err, "Should be able to write KB test data")

	// Read test data back
	readCmd := fmt.Sprintf(`su - ploy -c "curl -s http://localhost:8888/kb/test/%s"`, testKey)
	output, err := vpsClient.RunCommand(readCmd)
	assert.NoError(t, err, "Should be able to read KB test data")
	assert.JSONEq(t, testData, output, "Retrieved data should match written data")

	// Cleanup test data
	deleteCmd := fmt.Sprintf(`su - ploy -c "curl -X DELETE http://localhost:8888/kb/test/%s"`, testKey)
	vpsClient.RunCommand(deleteCmd) // Best effort cleanup
}
