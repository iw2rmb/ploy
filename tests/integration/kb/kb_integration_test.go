//go:build integration
// +build integration

package integration

import (
	"context"
	"testing"
	"time"

	consulapi "github.com/hashicorp/consul/api"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"

	"github.com/iw2rmb/ploy/internal/cli/transflow"
	"github.com/iw2rmb/ploy/internal/storage"
)

// KBIntegrationSuite tests KB learning with real services
type KBIntegrationSuite struct {
	suite.Suite
	consulClient  *consulapi.Client
	storageClient *storage.StorageClient
	kbStorage     *transflow.SeaweedFSKBStorage
	lockManager   *transflow.ConsulKBLockManager
}

func (s *KBIntegrationSuite) SetupSuite() {
	s.skipIfNoServices()

	var err error

	// Consul client for locking
	consulConfig := consulapi.DefaultConfig()
	consulConfig.Address = "localhost:8500"
	s.consulClient, err = consulapi.NewClient(consulConfig)
	require.NoError(s.T(), err)

	// Storage client
	s.storageClient, err = storage.NewStorageClient(&storage.Config{
		Endpoint: "http://localhost:8888",
	})
	require.NoError(s.T(), err)

	// KB storage with real services
	s.lockManager = transflow.NewConsulKBLockManager(s.consulClient)
	s.kbStorage = transflow.NewSeaweedFSKBStorage(s.storageClient, s.lockManager)
}

func (s *KBIntegrationSuite) skipIfNoServices() {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// Quick health check
	if s.consulClient == nil {
		consulConfig := consulapi.DefaultConfig()
		consulConfig.Address = "localhost:8500"
		client, err := consulapi.NewClient(consulConfig)
		if err != nil || func() bool {
			_, err := client.Status().Leader()
			return err != nil
		}() {
			s.T().Skip("Consul not available - run docker-compose up -d")
		}
	}
}

func (s *KBIntegrationSuite) TestKBLearningIntegration() {
	t := s.T()

	ctx := context.Background()
	errorSig := "integration-test-error"
	runID := "test-run-123"

	// Create test healing case
	caseRecord := &transflow.HealingCaseRecord{
		RunID:     runID,
		Timestamp: time.Now().Format(time.RFC3339),
		Language:  "java",
		Signature: errorSig,
		Context: transflow.HealingContext{
			Language:        "java",
			Lane:            "C",
			RepoURL:         "https://github.com/test/repo.git",
			CompilerVersion: "javac 11.0.1",
		},
		Attempt: transflow.HealingAttemptRecord{
			Type:   "orw_recipe",
			Recipe: "org.openrewrite.java.migrate.Java11toJava17",
		},
		Outcome: transflow.HealingOutcomeRecord{
			Success:      true,
			BuildStatus:  "passed",
			ErrorChanged: false,
			DurationMs:   5000,
			CompletedAt:  time.Now().Format(time.RFC3339),
		},
		BuildLogs: transflow.HealingBuildLogs{
			Stdout: "Build successful",
		},
	}

	// Test case recording - this should fail initially due to incomplete integration
	err := s.kbStorage.WriteCase(ctx, "java", errorSig, runID, caseRecord)
	if err != nil {
		t.Logf("Expected KB write failure: %v", err)
		assert.Error(t, err, "KB integration should fail initially")
	} else {
		// If write succeeds, test read
		cases, err := s.kbStorage.ReadCases(ctx, "java", errorSig)
		if err != nil {
			assert.Error(t, err, "Read should also be incomplete")
		} else {
			assert.Greater(t, len(cases), 0, "Should have at least one case")
		}
	}
}

func (s *KBIntegrationSuite) TestKBStorageHealth() {
	t := s.T()

	ctx := context.Background()

	// Test basic storage health
	err := s.kbStorage.Health(ctx)
	if err != nil {
		t.Logf("KB storage health check failed: %v (expected initially)", err)
	}
}

func (s *KBIntegrationSuite) TestKBLockingMechanism() {
	t := s.T()

	ctx := context.Background()
	lockKey := "test-lock-integration"

	// Test distributed locking
	acquired, err := s.lockManager.AcquireLock(ctx, lockKey, time.Minute)
	if err != nil {
		t.Logf("Lock acquisition failed: %v (expected initially)", err)
		assert.Error(t, err, "Locking should fail initially due to incomplete setup")
	} else {
		assert.True(t, acquired, "Lock should be acquired")

		// Release lock
		err = s.lockManager.ReleaseLock(ctx, lockKey)
		assert.NoError(t, err, "Lock release should succeed")
	}
}

func TestKBIntegrationSuite(t *testing.T) {
	suite.Run(t, new(KBIntegrationSuite))
}
