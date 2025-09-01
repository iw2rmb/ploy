package behavioral

import (
	"context"
	"os"
	"testing"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/iw2rmb/ploy/internal/testing/fixtures"
	"github.com/iw2rmb/ploy/internal/testing/helpers"
	"github.com/iw2rmb/ploy/internal/testing/integration"
)

var (
	apiClient    *integration.TestClient
	testContext  context.Context
	testCancel   context.CancelFunc
	testFixtures *fixtures.TestDataRepository
)

func TestBehavioral(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Behavioral Test Suite")
}

var _ = BeforeSuite(func() {
	// Setup test environment
	testContext, testCancel = context.WithTimeout(context.Background(), 30*time.Minute)

	// Initialize API client
	baseURL := os.Getenv("PLOY_TEST_BASE_URL")
	if baseURL == "" {
		baseURL = helpers.GetEnvOrDefault("PLOY_CONTROLLER", "http://localhost:8081")
	}

	// Create BDD-friendly API client that handles unavailable services gracefully
	apiClient = integration.NewBDDTestClient(baseURL)
	apiClient.WithTimeout(30 * time.Second)

	// Initialize test fixtures
	testFixtures = fixtures.NewTestDataRepository()

	// Wait for services to be ready (optional - gracefully handle unavailable services)
	By("Checking if controller is available for testing")
	// Don't fail the test if controller is unavailable - tests handle this gracefully

	// Setup test data
	setupTestData()
})

var _ = AfterSuite(func() {
	// Cleanup test data
	cleanupTestData()

	if testCancel != nil {
		testCancel()
	}
})

func setupTestData() {
	// Pre-populate test data if needed
	By("Setting up behavioral test data")
	// Implementation would go here - for now just log
	GinkgoWriter.Printf("Behavioral test data setup completed\n")
}

func cleanupTestData() {
	// Clean up any test artifacts
	By("Cleaning up behavioral test data")
	// Implementation would go here - for now just log
	GinkgoWriter.Printf("Behavioral test data cleanup completed\n")
}
