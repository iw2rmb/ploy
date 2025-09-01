//go:build integration
// +build integration

package integration

import (
	"context"
	"io"
	"os"
	"strings"
	"testing"
	"time"

	consulapi "github.com/hashicorp/consul/api"
	nomadapi "github.com/hashicorp/nomad/api"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"

	"github.com/iw2rmb/ploy/internal/storage"
	"github.com/iw2rmb/ploy/internal/testing/helpers"
)

// BuildIntegrationSuite tests build pipeline with real services
type BuildIntegrationSuite struct {
	suite.Suite

	nomadClient   *nomadapi.Client
	consulClient  *consulapi.Client
	storageClient *storage.StorageClient

	tempDir string
}

func (suite *BuildIntegrationSuite) SetupSuite() {
	// Initialize Nomad client
	nomadConfig := nomadapi.DefaultConfig()
	nomadConfig.Address = helpers.GetEnvOrDefault("NOMAD_ADDR", "http://localhost:4646")

	var err error
	suite.nomadClient, err = nomadapi.NewClient(nomadConfig)
	require.NoError(suite.T(), err)

	// Wait for Nomad to be ready
	suite.waitForNomad()

	// Initialize Consul client
	consulConfig := consulapi.DefaultConfig()
	consulConfig.Address = helpers.GetEnvOrDefault("CONSUL_HTTP_ADDR", "localhost:8500")

	suite.consulClient, err = consulapi.NewClient(consulConfig)
	require.NoError(suite.T(), err)

	// Wait for Consul to be ready
	suite.waitForConsul()

	// Initialize storage client
	seaweedfsConfig := storage.SeaweedFSConfig{
		Master: helpers.GetEnvOrDefault("SEAWEEDFS_MASTER", "localhost:9333"),
		Filer:  helpers.GetEnvOrDefault("SEAWEEDFS_FILER", "localhost:8888"),
	}

	seaweedfsProvider, err := storage.NewSeaweedFSClient(seaweedfsConfig)
	require.NoError(suite.T(), err)
	suite.storageClient = storage.NewStorageClient(seaweedfsProvider, nil)

	// Wait for SeaweedFS to be ready
	suite.waitForSeaweedFS()

	// Create temporary directory for test apps
	suite.tempDir, err = os.MkdirTemp("", "ploy-build-test-*")
	require.NoError(suite.T(), err)
}

func (suite *BuildIntegrationSuite) TearDownSuite() {
	// Cleanup test resources
	if suite.tempDir != "" {
		os.RemoveAll(suite.tempDir)
	}

	// Cleanup test jobs from Nomad
	suite.cleanupTestJobs()
}

func (suite *BuildIntegrationSuite) waitForNomad() {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	for {
		select {
		case <-ctx.Done():
			suite.T().Fatal("Nomad not ready within timeout")
		default:
			leader, err := suite.nomadClient.Status().Leader()
			if err == nil && leader != "" {
				return
			}
			time.Sleep(1 * time.Second)
		}
	}
}

func (suite *BuildIntegrationSuite) waitForConsul() {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	for {
		select {
		case <-ctx.Done():
			suite.T().Fatal("Consul not ready within timeout")
		default:
			_, err := suite.consulClient.Status().Leader()
			if err == nil {
				return
			}
			time.Sleep(1 * time.Second)
		}
	}
}

func (suite *BuildIntegrationSuite) waitForSeaweedFS() {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	for {
		select {
		case <-ctx.Done():
			suite.T().Fatal("SeaweedFS not ready within timeout")
		default:
			// Simple health check to verify SeaweedFS is responsive
			health := suite.storageClient.GetHealthStatus()
			if health != nil && health.Status == "healthy" {
				return
			}
			time.Sleep(1 * time.Second)
		}
	}
}

func (suite *BuildIntegrationSuite) cleanupTestJobs() {
	jobs, _, err := suite.nomadClient.Jobs().List(nil)
	if err != nil {
		return
	}

	for _, job := range jobs {
		if strings.HasPrefix(job.Name, "test-") {
			suite.nomadClient.Jobs().Deregister(job.ID, true, nil)
		}
	}
}

func (suite *BuildIntegrationSuite) TestServiceConnectivity() {
	suite.T().Run("Nomad connectivity", func(t *testing.T) {
		leader, err := suite.nomadClient.Status().Leader()
		require.NoError(t, err)
		assert.NotEmpty(t, leader, "Nomad should have a leader")
	})

	suite.T().Run("Consul connectivity", func(t *testing.T) {
		leader, err := suite.consulClient.Status().Leader()
		require.NoError(t, err)
		assert.NotEmpty(t, leader, "Consul should have a leader")
	})

	suite.T().Run("SeaweedFS connectivity", func(t *testing.T) {
		health := suite.storageClient.GetHealthStatus()
		assert.NotNil(t, health, "Should get health status")
		assert.Equal(t, "healthy", health.Status, "SeaweedFS should be healthy")
	})
}

func (suite *BuildIntegrationSuite) TestStorageOperations() {
	testBucket := suite.storageClient.GetArtifactsBucket()
	testKey := "integration-test/sample-file.txt"
	testData := []byte("integration test data")

	suite.T().Run("Upload file to storage", func(t *testing.T) {
		result, err := suite.storageClient.PutObject(testBucket, testKey, strings.NewReader(string(testData)), "text/plain")
		require.NoError(t, err, "Should be able to upload file")
		assert.NotEmpty(t, result.ETag, "Should have ETag")
	})

	suite.T().Run("Download file from storage", func(t *testing.T) {
		reader, err := suite.storageClient.GetObject(testBucket, testKey)
		require.NoError(t, err)
		defer reader.Close()

		data, err := io.ReadAll(reader)
		require.NoError(t, err)
		assert.Equal(t, testData, data, "Downloaded data should match uploaded data")
	})

	suite.T().Run("List objects in storage", func(t *testing.T) {
		objects, err := suite.storageClient.ListObjects(testBucket, "integration-test/")
		require.NoError(t, err)
		assert.Len(t, objects, 1, "Should find uploaded file")
		assert.Equal(t, testKey, objects[0].Key)
	})
}

func TestBuildIntegrationSuite(t *testing.T) {
	suite.Run(t, new(BuildIntegrationSuite))
}
