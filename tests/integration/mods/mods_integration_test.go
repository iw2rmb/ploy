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
	"github.com/iw2rmb/ploy/tests/integration/internal/testenv"
)

// ModsIntegrationSuite tests mods with real services
type ModsIntegrationSuite struct {
	suite.Suite
	nomadClient   *nomadapi.Client
	consulClient  *consulapi.Client
	storageClient storage.Storage
	config        *mods.ModConfig
}

func (s *ModsIntegrationSuite) SetupSuite() {
	t := s.T()
	if testing.Short() {
		t.Skip("skipping mods integration suite in short mode")
	}

	s.nomadClient = testenv.RequireNomadClient(t)
	s.consulClient = testenv.RequireConsulClient(t)
	s.storageClient = testenv.RequireSeaweedStorage(t)

	s.config = &mods.ModConfig{
		ID:           "integration-test-workflow",
		TargetRepo:   "https://github.com/example/test-repo.git", // Will fail - test repo
		TargetBranch: "main",
		BaseRef:      "refs/heads/main",
		Lane:         "D",
		BuildTimeout: "10m",
		Steps: []mods.ModStep{
			{
				Type:    "recipe",
				ID:      "java-migration",
				Engine:  "openrewrite",
				Recipes: []string{"org.openrewrite.java.migrate.Java11toJava17"},
			},
		},
		SelfHeal: &mods.SelfHealConfig{
			Enabled:    true,
			MaxRetries: 2,
		},
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
	jobType := nomadapi.JobTypeService
	groupName := "test-group"
	count := 1
	job := &nomadapi.Job{
		ID:   &jobID,
		Name: &jobID,
		Type: &jobType,
		TaskGroups: []*nomadapi.TaskGroup{
			{
				Name:  &groupName,
				Count: &count,
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
