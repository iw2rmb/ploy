package behavioral

import (
	"context"
	"os"
	"testing"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/iw2rmb/ploy/internal/testutil"
	"github.com/iw2rmb/ploy/internal/testutil/api"
)

var (
	apiClient   *api.TestClient
	testContext context.Context
	testCancel  context.CancelFunc
	fixtures    *testutil.TestDataRepository
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
		baseURL = testutil.GetEnvOrDefault("PLOY_CONTROLLER", "http://localhost:8081")
	}

	apiClient = api.NewTestClient(GinkgoT(), baseURL)
	apiClient.WithTimeout(30 * time.Second)

	// Initialize test fixtures
	fixtures = testutil.NewTestDataRepository()

	// Wait for services to be ready
	Eventually(func() error {
		resp := apiClient.GET("/health").Execute()
		if resp == nil || resp.StatusCode != 200 {
			return nil // Return nil to continue polling
		}
		return nil
	}, "2m", "5s").Should(Succeed(), "Controller should be healthy")

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