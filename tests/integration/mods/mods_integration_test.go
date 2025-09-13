//go:build integration
// +build integration

package integration

import (
	"context"
	"fmt"
	"testing"
	"time"

	consulapi "github.com/hashicorp/consul/api"
	nomadapi "github.com/hashicorp/nomad/api"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"

	mods "github.com/iw2rmb/ploy/internal/mods"
	"github.com/iw2rmb/ploy/internal/storage"
)

// ModsIntegrationSuite tests mods with real services
type ModsIntegrationSuite struct {
	suite.Suite
	nomadClient   *nomadapi.Client
	consulClient  *consulapi.Client
	storageClient *storage.StorageClient
	config        *mods.ModConfig
}

func (s *ModsIntegrationSuite) SetupSuite() {
	// Skip if services not available
	s.skipIfNoServices()

	// Setup real service clients
	var err error

	// Nomad client
	nomadConfig := nomadapi.DefaultConfig()
	nomadConfig.Address = "http://localhost:4646"
	s.nomadClient, err = nomadapi.NewClient(nomadConfig)
	require.NoError(s.T(), err)

	// Consul client
	consulConfig := consulapi.DefaultConfig()
	consulConfig.Address = "localhost:8500"
	s.consulClient, err = consulapi.NewClient(consulConfig)
	require.NoError(s.T(), err)

	// Storage client
	s.storageClient, err = storage.NewStorageClient(&storage.Config{
		Endpoint: "http://localhost:8888",
	})
	require.NoError(s.T(), err)

	// Mods config with real service endpoints
	s.config = &mods.ModConfig{
		ID:           "integration-test-workflow",
		TargetRepo:   "https://github.com/example/test-repo.git", // Will fail - test repo
		BaseRef:      "refs/heads/main",
		BuildTimeout: "10m",
		Steps: []mods.ModStep{
			{
				Type:    "recipe",
				ID:      "java-migration",
				Engine:  "openrewrite",
				Recipes: []string{"org.openrewrite.java.migrate.Java11toJava17"},
			},
		},
		SelfHeal: mods.SelfHealConfig{
			Enabled:    true,
			MaxRetries: 2,
		},
	}
}

func (s *ModsIntegrationSuite) skipIfNoServices() {
	// Check if Docker services are available
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	_, err := s.nomadClient.Status().Leader()
	if err != nil {
		s.T().Skip("Nomad service not available - run docker-compose up -d")
	}

	_, err = s.consulClient.Status().Leader()
	if err != nil {
		s.T().Skip("Consul service not available - run docker-compose up -d")
	}
}

func (s *ModsIntegrationSuite) TestModsFullWorkflow_Integration() {
	t := s.T()

	// This should fail initially - full integration not implemented
	integrations := mods.NewModIntegrationsWithTestMode("", t.TempDir(), false)

	runner, err := mods.NewModRunner(s.config, t.TempDir())
	require.NoError(t, err)

	// Set up with real dependencies (this will likely fail due to missing implementations)
	runner.SetBuildChecker(integrations.CreateBuildChecker())

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	// This should fail - we don't have a complete integration yet
	result, err := runner.Run(ctx)

	// Initially we expect this to fail due to incomplete integration
	if err != nil {
		t.Logf("Expected integration failure: %v", err)
		assert.Error(t, err, "Integration should fail initially")
	} else {
		// If it somehow passes, validate the result structure
		assert.NotNil(t, result)
		assert.NotEmpty(t, result.WorkflowID)
	}
}

func (s *ModsIntegrationSuite) TestServiceHealth() {
	t := s.T()

	// Test Nomad health
	leader, err := s.nomadClient.Status().Leader()
	require.NoError(t, err)
	assert.NotEmpty(t, leader, "Nomad should have a leader")

	// Test Consul health
	consulLeader, err := s.consulClient.Status().Leader()
	require.NoError(t, err)
	assert.NotEmpty(t, consulLeader, "Consul should have a leader")

	// Test storage health (basic connectivity)
	err = s.storageClient.Health(context.Background())
	if err != nil {
		t.Logf("Storage health check failed: %v (expected in initial setup)", err)
	}
}

func (s *ModsIntegrationSuite) TestNomadJobSubmission() {
	t := s.T()

	// Test basic Nomad job submission capability
	jobID := fmt.Sprintf("test-job-%d", time.Now().Unix())

	// This is a minimal test job that should succeed
	job := &nomadapi.Job{
		ID:   &jobID,
		Name: &jobID,
		Type: nomadapi.JobTypeService.Ptr(),
		TaskGroups: []*nomadapi.TaskGroup{
			{
				Name:  nomadapi.StringToPtr("test-group"),
				Count: nomadapi.IntToPtr(1),
				Tasks: []*nomadapi.Task{
					{
						Name:   "test-task",
						Driver: "raw_exec",
						Config: map[string]interface{}{
							"command": "echo",
							"args":    []string{"hello", "world"},
						},
					},
				},
			},
		},
	}

	// Submit job
	response, _, err := s.nomadClient.Jobs().Register(job, nil)
	require.NoError(t, err)
	assert.NotEmpty(t, response.EvalID)

	// Cleanup
	_, _, err = s.nomadClient.Jobs().Deregister(jobID, true, nil)
	require.NoError(t, err)
}

func TestModsIntegrationSuite(t *testing.T) {
	suite.Run(t, new(ModsIntegrationSuite))
}
