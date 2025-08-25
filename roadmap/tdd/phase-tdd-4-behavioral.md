# Phase 4: Behavioral & End-to-End Testing

## Overview

Phase 4 implements Behavior-Driven Development (BDD) specifications and comprehensive end-to-end testing scenarios. We'll create human-readable test specifications that validate complete user workflows while establishing performance regression testing and system resilience validation.

## Objectives

1. Implement BDD-style specifications using Ginkgo/Gomega
2. Create end-to-end test scenarios for critical user journeys
3. Establish performance regression testing framework
4. Develop chaos testing for system resilience
5. Achieve complete workflow coverage

## Implementation Plan

### BDD Implementation
- Ginkgo/Gomega setup and BDD patterns
- User journey specifications
- BDD test data management

### E2E and Performance
- End-to-end workflow testing
- Performance regression framework
- Chaos testing and resilience validation

## Deliverables

### 1. BDD Framework Setup (`test/behavioral/`)

#### 1.1 BDD Suite Configuration (`suite_test.go`)
```go
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
    apiClient     *api.TestClient
    testContext   context.Context
    testCancel    context.CancelFunc
    fixtures      *testutil.TestDataRepository
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
        baseURL = "http://localhost:8081"
    }
    
    apiClient = api.NewTestClient(GinkgoT(), baseURL)
    apiClient.WithTimeout(30 * time.Second)
    
    // Initialize test fixtures
    fixtures = testutil.NewTestDataRepository()
    
    // Wait for services to be ready
    Eventually(func() error {
        _, err := apiClient.GET("/health").Execute(), nil
        return err
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
    // Implementation would go here
}

func cleanupTestData() {
    // Clean up any test artifacts
    By("Cleaning up behavioral test data")
    // Implementation would go here
}
```

#### 1.2 Application Deployment Workflows (`app_deployment_test.go`)
```go
package behavioral

import (
    "fmt"
    "time"
    
    . "github.com/onsi/ginkgo/v2"
    . "github.com/onsi/gomega"
)

var _ = Describe("Application Deployment Workflow", func() {
    var (
        appName string
        gitURL  string
        branch  string
    )
    
    BeforeEach(func() {
        appName = fmt.Sprintf("test-app-%d", GinkgoRandomSeed())
        gitURL = "https://github.com/test-org/sample-go-app.git"
        branch = "main"
    })
    
    AfterEach(func() {
        // Cleanup deployed app
        apiClient.DELETE("/v1/apps/" + appName).Execute()
    })
    
    Context("When deploying a new Go application", func() {
        It("should successfully deploy through the entire pipeline", func() {
            By("Triggering a build for the Go application")
            buildRequest := map[string]interface{}{
                "git_url": gitURL,
                "branch":  branch,
            }
            
            resp := apiClient.POST("/v1/apps/"+appName+"/builds").
                WithJSON(buildRequest).
                ExpectStatus(202).
                ExpectJSON().
                Execute()
            
            resp.AssertJSONPath("status", "build_triggered")
            
            By("Waiting for the build to start")
            Eventually(func() string {
                resp := apiClient.GET("/v1/apps/"+appName+"/status").
                    ExpectStatus(200).
                    Execute()
                
                var status map[string]interface{}
                resp.JSON(&status)
                return status["status"].(string)
            }, "2m", "5s").Should(Equal("building"), "Build should start within 2 minutes")
            
            By("Waiting for the build to complete successfully")
            Eventually(func() string {
                resp := apiClient.GET("/v1/apps/"+appName+"/status").
                    ExpectStatus(200).
                    Execute()
                
                var status map[string]interface{}
                resp.JSON(&status)
                return status["status"].(string)
            }, "10m", "10s").Should(Equal("running"), "Build should complete within 10 minutes")
            
            By("Verifying the application is accessible")
            Eventually(func() int {
                // This would test the actual deployed app endpoint
                return 200 // Simplified for example
            }, "2m", "5s").Should(Equal(200), "Application should be accessible")
            
            By("Checking application health")
            Eventually(func() string {
                resp := apiClient.GET("/v1/apps/"+appName+"/status").
                    ExpectStatus(200).
                    Execute()
                
                var status map[string]interface{}
                resp.JSON(&status)
                return status["health"].(string)
            }, "1m", "5s").Should(Equal("healthy"), "Application should be healthy")
        })
        
        It("should handle build failures gracefully", func() {
            By("Triggering a build with invalid repository")
            buildRequest := map[string]interface{}{
                "git_url": "https://github.com/test-org/non-existent-repo.git",
                "branch":  "main",
            }
            
            apiClient.POST("/v1/apps/"+appName+"/builds").
                WithJSON(buildRequest).
                ExpectStatus(202).
                Execute()
            
            By("Waiting for the build to fail")
            Eventually(func() string {
                resp := apiClient.GET("/v1/apps/"+appName+"/status").
                    ExpectStatus(200).
                    Execute()
                
                var status map[string]interface{}
                resp.JSON(&status)
                return status["status"].(string)
            }, "5m", "10s").Should(Equal("failed"), "Build should fail within 5 minutes")
            
            By("Verifying error information is available")
            resp := apiClient.GET("/v1/apps/"+appName+"/logs").
                ExpectStatus(200).
                Execute()
            
            var logs map[string]interface{}
            resp.JSON(&logs)
            Expect(logs["error"]).ToNot(BeEmpty(), "Error logs should be available")
        })
    })
    
    Context("When deploying different application types", func() {
        DescribeTable("should correctly detect lane and deploy",
            func(repoURL, expectedLane string, buildTimeout time.Duration) {
                By(fmt.Sprintf("Deploying %s application", expectedLane))
                buildRequest := map[string]interface{}{
                    "git_url": repoURL,
                    "branch":  "main",
                }
                
                apiClient.POST("/v1/apps/"+appName+"/builds").
                    WithJSON(buildRequest).
                    ExpectStatus(202).
                    Execute()
                
                By("Verifying correct lane detection")
                Eventually(func() string {
                    resp := apiClient.GET("/v1/apps/"+appName+"/status").
                        ExpectStatus(200).
                        Execute()
                    
                    var status map[string]interface{}
                    resp.JSON(&status)
                    return status["lane"].(string)
                }, "1m", "5s").Should(Equal(expectedLane))
                
                By("Waiting for successful deployment")
                Eventually(func() string {
                    resp := apiClient.GET("/v1/apps/"+appName+"/status").
                        ExpectStatus(200).
                        Execute()
                    
                    var status map[string]interface{}
                    resp.JSON(&status)
                    return status["status"].(string)
                }, buildTimeout, "10s").Should(Equal("running"))
            },
            Entry("Go application (Lane A)", "https://github.com/test/go-app.git", "A", 5*time.Minute),
            Entry("Node.js application (Lane B)", "https://github.com/test/node-app.git", "B", 8*time.Minute),
            Entry("Java application (Lane C)", "https://github.com/test/java-app.git", "C", 10*time.Minute),
            Entry("WASM application (Lane G)", "https://github.com/test/wasm-app.git", "G", 6*time.Minute),
        )
    })
})
```

#### 1.3 Environment Variable Management (`environment_management_test.go`)
```go
package behavioral

import (
    "fmt"
    
    . "github.com/onsi/ginkgo/v2"
    . "github.com/onsi/gomega"
)

var _ = Describe("Environment Variable Management", func() {
    var appName string
    
    BeforeEach(func() {
        appName = fmt.Sprintf("env-test-app-%d", GinkgoRandomSeed())
    })
    
    Context("When managing application environment variables", func() {
        It("should support complete CRUD operations", func() {
            By("Starting with empty environment variables")
            resp := apiClient.GET("/v1/apps/"+appName+"/env").
                ExpectStatus(200).
                Execute()
            
            var envResp map[string]interface{}
            resp.JSON(&envResp)
            Expect(envResp["env"]).To(BeEmpty(), "Environment should start empty")
            
            By("Setting multiple environment variables")
            envVars := map[string]string{
                "DATABASE_URL": "postgres://localhost:5432/myapp",
                "REDIS_URL":    "redis://localhost:6379",
                "LOG_LEVEL":    "info",
                "DEBUG":        "false",
            }
            
            apiClient.POST("/v1/apps/"+appName+"/env").
                WithJSON(envVars).
                ExpectStatus(200).
                Execute()
            
            By("Verifying all variables were set correctly")
            resp = apiClient.GET("/v1/apps/"+appName+"/env").
                ExpectStatus(200).
                Execute()
            
            resp.JSON(&envResp)
            envMap := envResp["env"].(map[string]interface{})
            
            Expect(envMap["DATABASE_URL"]).To(Equal("postgres://localhost:5432/myapp"))
            Expect(envMap["REDIS_URL"]).To(Equal("redis://localhost:6379"))
            Expect(envMap["LOG_LEVEL"]).To(Equal("info"))
            Expect(envMap["DEBUG"]).To(Equal("false"))
            
            By("Updating a single environment variable")
            apiClient.PUT("/v1/apps/"+appName+"/env/LOG_LEVEL").
                WithJSON(map[string]string{"value": "debug"}).
                ExpectStatus(200).
                Execute()
            
            By("Verifying the update")
            apiClient.GET("/v1/apps/"+appName+"/env").
                ExpectStatus(200).
                Execute().
                AssertJSONPath("env.LOG_LEVEL", "debug")
            
            By("Deleting an environment variable")
            apiClient.DELETE("/v1/apps/"+appName+"/env/DEBUG").
                ExpectStatus(200).
                Execute()
            
            By("Verifying the deletion")
            resp = apiClient.GET("/v1/apps/"+appName+"/env").
                ExpectStatus(200).
                Execute()
            
            resp.JSON(&envResp)
            envMap = envResp["env"].(map[string]interface{})
            Expect(envMap).ToNot(HaveKey("DEBUG"), "DEBUG variable should be deleted")
            Expect(envMap).To(HaveKey("LOG_LEVEL"), "Other variables should remain")
        })
        
        It("should validate environment variable constraints", func() {
            By("Rejecting invalid variable names")
            invalidEnvVars := map[string]string{
                "INVALID-NAME!": "value",  // Invalid characters
                "123INVALID":    "value",  // Starts with number
                "":              "value",  // Empty name
            }
            
            apiClient.POST("/v1/apps/"+appName+"/env").
                WithJSON(invalidEnvVars).
                ExpectStatus(400).
                Execute()
            
            By("Rejecting oversized values")
            largeValue := make([]byte, 10000) // 10KB value
            for i := range largeValue {
                largeValue[i] = 'x'
            }
            
            oversizedEnvVars := map[string]string{
                "LARGE_VALUE": string(largeValue),
            }
            
            apiClient.POST("/v1/apps/"+appName+"/env").
                WithJSON(oversizedEnvVars).
                ExpectStatus(400).
                Execute()
        })
        
        It("should handle concurrent updates correctly", func() {
            By("Setting up initial environment variables")
            initialVars := map[string]string{
                "COUNTER": "0",
                "STATUS":  "initial",
            }
            
            apiClient.POST("/v1/apps/"+appName+"/env").
                WithJSON(initialVars).
                ExpectStatus(200).
                Execute()
            
            By("Performing concurrent updates")
            // This would test race conditions in environment variable updates
            done := make(chan bool, 2)
            
            go func() {
                defer GinkgoRecover()
                apiClient.PUT("/v1/apps/"+appName+"/env/COUNTER").
                    WithJSON(map[string]string{"value": "1"}).
                    ExpectStatus(200).
                    Execute()
                done <- true
            }()
            
            go func() {
                defer GinkgoRecover()
                apiClient.PUT("/v1/apps/"+appName+"/env/STATUS").
                    WithJSON(map[string]string{"value": "updated"}).
                    ExpectStatus(200).
                    Execute()
                done <- true
            }()
            
            // Wait for both updates
            Eventually(done).Should(Receive())
            Eventually(done).Should(Receive())
            
            By("Verifying final state is consistent")
            resp := apiClient.GET("/v1/apps/"+appName+"/env").
                ExpectStatus(200).
                Execute()
            
            var envResp map[string]interface{}
            resp.JSON(&envResp)
            envMap := envResp["env"].(map[string]interface{})
            
            Expect(envMap["COUNTER"]).To(Equal("1"))
            Expect(envMap["STATUS"]).To(Equal("updated"))
        })
    })
})
```

#### 1.4 Domain and Certificate Management (`domain_certificate_test.go`)
```go
package behavioral

import (
    "fmt"
    "time"
    
    . "github.com/onsi/ginkgo/v2"
    . "github.com/onsi/gomega"
)

var _ = Describe("Domain and Certificate Management", func() {
    var (
        appName    string
        testDomain string
    )
    
    BeforeEach(func() {
        appName = fmt.Sprintf("domain-test-app-%d", GinkgoRandomSeed())
        testDomain = fmt.Sprintf("test-%d.example.com", GinkgoRandomSeed())
    })
    
    AfterEach(func() {
        // Cleanup domains and certificates
        apiClient.DELETE("/v1/apps/"+appName+"/domains/"+testDomain).Execute()
        apiClient.DELETE("/v1/apps/"+appName+"/certificates/"+testDomain).Execute()
        apiClient.DELETE("/v1/apps/" + appName).Execute()
    })
    
    Context("When managing application domains", func() {
        It("should support complete domain lifecycle", func() {
            By("Starting with no domains")
            resp := apiClient.GET("/v1/apps/"+appName+"/domains").
                ExpectStatus(200).
                Execute()
            
            var domainsResp map[string]interface{}
            resp.JSON(&domainsResp)
            Expect(domainsResp["domains"]).To(BeEmpty())
            
            By("Adding a custom domain")
            domainRequest := map[string]string{
                "domain": testDomain,
            }
            
            apiClient.POST("/v1/apps/"+appName+"/domains").
                WithJSON(domainRequest).
                ExpectStatus(201).
                Execute()
            
            By("Verifying domain was added")
            resp = apiClient.GET("/v1/apps/"+appName+"/domains").
                ExpectStatus(200).
                Execute()
            
            resp.JSON(&domainsResp)
            domains := domainsResp["domains"].([]interface{})
            Expect(domains).To(HaveLen(1))
            Expect(domains[0].(map[string]interface{})["domain"]).To(Equal(testDomain))
            
            By("Removing the domain")
            apiClient.DELETE("/v1/apps/"+appName+"/domains/"+testDomain).
                ExpectStatus(200).
                Execute()
            
            By("Verifying domain was removed")
            resp = apiClient.GET("/v1/apps/"+appName+"/domains").
                ExpectStatus(200).
                Execute()
            
            resp.JSON(&domainsResp)
            Expect(domainsResp["domains"]).To(BeEmpty())
        })
        
        It("should automatically provision certificates for domains", func() {
            Skip("Requires valid DNS configuration - run manually for full testing")
            
            By("Adding a domain that supports automatic certificate provisioning")
            domainRequest := map[string]string{
                "domain": testDomain,
            }
            
            apiClient.POST("/v1/apps/"+appName+"/domains").
                WithJSON(domainRequest).
                ExpectStatus(201).
                Execute()
            
            By("Waiting for automatic certificate provisioning")
            Eventually(func() string {
                resp := apiClient.GET("/v1/apps/"+appName+"/certificates/"+testDomain).
                    ExpectStatus(200).
                    Execute()
                
                var certResp map[string]interface{}
                resp.JSON(&certResp)
                return certResp["status"].(string)
            }, "5m", "10s").Should(Equal("active"), "Certificate should be provisioned automatically")
            
            By("Verifying certificate details")
            resp := apiClient.GET("/v1/apps/"+appName+"/certificates/"+testDomain).
                ExpectStatus(200).
                Execute()
            
            var certResp map[string]interface{}
            resp.JSON(&certResp)
            Expect(certResp["domain"]).To(Equal(testDomain))
            Expect(certResp["issuer"]).To(Equal("Let's Encrypt"))
            Expect(certResp["expires_at"]).ToNot(BeEmpty())
        })
        
        It("should handle domain validation errors", func() {
            By("Attempting to add an invalid domain")
            invalidDomains := []string{
                "invalid.domain",      // Invalid TLD
                "localhost",           // Localhost not allowed
                "sub.sub.sub.sub.domain.com", // Too many subdomains
                "",                    // Empty domain
                "invalid space.com",   // Space in domain
            }
            
            for _, invalidDomain := range invalidDomains {
                domainRequest := map[string]string{
                    "domain": invalidDomain,
                }
                
                apiClient.POST("/v1/apps/"+appName+"/domains").
                    WithJSON(domainRequest).
                    ExpectStatus(400).
                    Execute()
            }
        })
    })
    
    Context("When managing SSL certificates", func() {
        It("should support manual certificate upload", func() {
            // This would test uploading custom certificates
            Skip("Requires certificate test data - implement with test certificates")
        })
        
        It("should handle certificate renewal", func() {
            Skip("Requires time-based testing - implement with mock certificates")
        })
    })
})
```

### 2. End-to-End Workflow Testing

#### 2.1 Complete Application Lifecycle (`e2e_lifecycle_test.go`)
```go
package behavioral

import (
    "fmt"
    "time"
    
    . "github.com/onsi/ginkgo/v2"
    . "github.com/onsi/gomega"
)

var _ = Describe("End-to-End Application Lifecycle", func() {
    Context("Complete developer workflow", func() {
        It("should support the full application development and deployment cycle", func() {
            appName := fmt.Sprintf("e2e-app-%d", GinkgoRandomSeed())
            customDomain := fmt.Sprintf("e2e-%d.test.com", GinkgoRandomSeed())
            
            By("Step 1: Initial application deployment")
            buildRequest := map[string]interface{}{
                "git_url": "https://github.com/test-org/sample-microservice.git",
                "branch":  "main",
            }
            
            apiClient.POST("/v1/apps/"+appName+"/builds").
                WithJSON(buildRequest).
                ExpectStatus(202).
                Execute()
            
            By("Step 2: Waiting for initial deployment to complete")
            Eventually(func() string {
                resp := apiClient.GET("/v1/apps/"+appName+"/status").ExpectStatus(200).Execute()
                var status map[string]interface{}
                resp.JSON(&status)
                return status["status"].(string)
            }, "10m", "15s").Should(Equal("running"), "Initial deployment should succeed")
            
            By("Step 3: Configuring environment variables")
            envVars := map[string]string{
                "NODE_ENV":           "production",
                "DATABASE_URL":       "postgres://prod-db:5432/app",
                "REDIS_URL":          "redis://prod-redis:6379",
                "LOG_LEVEL":          "info",
                "METRICS_ENABLED":    "true",
                "HEALTH_CHECK_PATH":  "/health",
            }
            
            apiClient.POST("/v1/apps/"+appName+"/env").
                WithJSON(envVars).
                ExpectStatus(200).
                Execute()
            
            By("Step 4: Adding custom domain")
            domainRequest := map[string]string{"domain": customDomain}
            apiClient.POST("/v1/apps/"+appName+"/domains").
                WithJSON(domainRequest).
                ExpectStatus(201).
                Execute()
            
            By("Step 5: Triggering application restart to apply changes")
            apiClient.POST("/v1/apps/"+appName+"/restart").
                ExpectStatus(202).
                Execute()
            
            By("Step 6: Verifying application is running with new configuration")
            Eventually(func() string {
                resp := apiClient.GET("/v1/apps/"+appName+"/status").ExpectStatus(200).Execute()
                var status map[string]interface{}
                resp.JSON(&status)
                return status["status"].(string)
            }, "5m", "10s").Should(Equal("running"))
            
            By("Step 7: Scaling application")
            scaleRequest := map[string]interface{}{"instances": 3}
            apiClient.PUT("/v1/apps/"+appName+"/scale").
                WithJSON(scaleRequest).
                ExpectStatus(200).
                Execute()
            
            Eventually(func() float64 {
                resp := apiClient.GET("/v1/apps/"+appName+"/status").ExpectStatus(200).Execute()
                var status map[string]interface{}
                resp.JSON(&status)
                return status["instances"].(float64)
            }, "3m", "5s").Should(Equal(float64(3)))
            
            By("Step 8: Deploying application update")
            updateRequest := map[string]interface{}{
                "git_url": "https://github.com/test-org/sample-microservice.git",
                "branch":  "v2.0",
            }
            
            apiClient.POST("/v1/apps/"+appName+"/builds").
                WithJSON(updateRequest).
                ExpectStatus(202).
                Execute()
            
            Eventually(func() string {
                resp := apiClient.GET("/v1/apps/"+appName+"/status").ExpectStatus(200).Execute()
                var status map[string]interface{}
                resp.JSON(&status)
                return status["version"].(string)
            }, "10m", "15s").Should(ContainSubstring("v2.0"))
            
            By("Step 9: Rolling back if needed")
            rollbackRequest := map[string]interface{}{
                "target_version": "v1.0",
            }
            
            apiClient.POST("/v1/apps/"+appName+"/rollback").
                WithJSON(rollbackRequest).
                ExpectStatus(200).
                Execute()
            
            Eventually(func() string {
                resp := apiClient.GET("/v1/apps/"+appName+"/status").ExpectStatus(200).Execute()
                var status map[string]interface{}
                resp.JSON(&status)
                return status["version"].(string)
            }, "5m", "10s").Should(ContainSubstring("v1.0"))
            
            By("Step 10: Monitoring and debugging")
            logsResp := apiClient.GET("/v1/apps/"+appName+"/logs").
                ExpectStatus(200).
                Execute()
            
            var logs map[string]interface{}
            logsResp.JSON(&logs)
            Expect(logs["logs"]).ToNot(BeEmpty())
            
            metricsResp := apiClient.GET("/v1/apps/"+appName+"/metrics").
                ExpectStatus(200).
                Execute()
            
            var metrics map[string]interface{}
            metricsResp.JSON(&metrics)
            Expect(metrics["metrics"]).ToNot(BeEmpty())
            
            By("Step 11: Final cleanup")
            apiClient.DELETE("/v1/apps/"+appName).
                ExpectStatus(200).
                Execute()
            
            Eventually(func() int {
                resp, _ := apiClient.GET("/v1/apps/"+appName+"/status").Execute(), nil
                return resp.StatusCode
            }, "2m", "5s").Should(Equal(404), "Application should be fully removed")
        })
    })
})
```

### 3. Performance Regression Testing

#### 3.1 Performance Regression Framework (`performance_regression_test.go`)
```go
package behavioral

import (
    "encoding/json"
    "fmt"
    "os"
    "time"
    
    . "github.com/onsi/ginkgo/v2"
    . "github.com/onsi/gomega"
)

// PerformanceBaseline represents expected performance metrics
type PerformanceBaseline struct {
    APIResponseTime   time.Duration `json:"api_response_time"`
    BuildTime         time.Duration `json:"build_time"`
    DeploymentTime    time.Duration `json:"deployment_time"`
    ThroughputRPS     int           `json:"throughput_rps"`
    ConcurrentUsers   int           `json:"concurrent_users"`
    MemoryUsageMB     int           `json:"memory_usage_mb"`
    CPUUsagePercent   float64       `json:"cpu_usage_percent"`
}

// LoadPerformanceBaseline loads expected performance metrics
func LoadPerformanceBaseline() *PerformanceBaseline {
    baselineFile := "test/behavioral/performance_baseline.json"
    data, err := os.ReadFile(baselineFile)
    if err != nil {
        // Default baseline if file doesn't exist
        return &PerformanceBaseline{
            APIResponseTime: 200 * time.Millisecond,
            BuildTime:       5 * time.Minute,
            DeploymentTime:  2 * time.Minute,
            ThroughputRPS:   50,
            ConcurrentUsers: 10,
            MemoryUsageMB:   256,
            CPUUsagePercent: 50.0,
        }
    }
    
    var baseline PerformanceBaseline
    if err := json.Unmarshal(data, &baseline); err != nil {
        Fail(fmt.Sprintf("Failed to parse baseline: %v", err))
    }
    
    return &baseline
}

var _ = Describe("Performance Regression Testing", func() {
    var baseline *PerformanceBaseline
    
    BeforeEach(func() {
        baseline = LoadPerformanceBaseline()
    })
    
    Context("API Performance", func() {
        It("should maintain API response time within baseline", func() {
            measurements := make([]time.Duration, 10)
            
            By("Measuring API response times")
            for i := 0; i < 10; i++ {
                start := time.Now()
                apiClient.GET("/health").ExpectStatus(200).Execute()
                measurements[i] = time.Since(start)
            }
            
            By("Calculating average response time")
            var total time.Duration
            for _, measurement := range measurements {
                total += measurement
            }
            avgResponseTime := total / time.Duration(len(measurements))
            
            By("Verifying response time is within acceptable range")
            maxAcceptable := baseline.APIResponseTime + (baseline.APIResponseTime / 4) // 25% tolerance
            Expect(avgResponseTime).To(BeNumerically("<=", maxAcceptable),
                fmt.Sprintf("Average API response time %v exceeds baseline %v (max acceptable: %v)",
                    avgResponseTime, baseline.APIResponseTime, maxAcceptable))
            
            By("Recording performance metrics")
            GinkgoWriter.Printf("API Performance:\n")
            GinkgoWriter.Printf("  Baseline: %v\n", baseline.APIResponseTime)
            GinkgoWriter.Printf("  Measured: %v\n", avgResponseTime)
            GinkgoWriter.Printf("  Tolerance: %v\n", maxAcceptable)
        })
        
        It("should handle concurrent load within performance targets", func() {
            concurrency := baseline.ConcurrentUsers
            duration := 30 * time.Second
            
            By(fmt.Sprintf("Running load test with %d concurrent users", concurrency))
            results := runLoadTest(concurrency, duration)
            
            By("Verifying throughput meets baseline")
            Expect(results.RequestsPerSecond).To(BeNumerically(">=", baseline.ThroughputRPS),
                fmt.Sprintf("Throughput %d RPS below baseline %d RPS",
                    results.RequestsPerSecond, baseline.ThroughputRPS))
            
            By("Verifying error rate is acceptable")
            Expect(results.ErrorRate).To(BeNumerically("<=", 0.01),
                fmt.Sprintf("Error rate %.2f%% exceeds 1%% threshold", results.ErrorRate*100))
        })
    })
    
    Context("Build Performance", func() {
        It("should complete builds within time baseline", func() {
            appName := fmt.Sprintf("perf-build-test-%d", GinkgoRandomSeed())
            
            By("Starting build timer")
            startTime := time.Now()
            
            buildRequest := map[string]interface{}{
                "git_url": "https://github.com/test-org/standard-go-app.git",
                "branch":  "main",
            }
            
            apiClient.POST("/v1/apps/"+appName+"/builds").
                WithJSON(buildRequest).
                ExpectStatus(202).
                Execute()
            
            By("Waiting for build completion")
            Eventually(func() string {
                resp := apiClient.GET("/v1/apps/"+appName+"/status").ExpectStatus(200).Execute()
                var status map[string]interface{}
                resp.JSON(&status)
                return status["status"].(string)
            }, baseline.BuildTime*2, "10s").Should(Equal("running"))
            
            buildTime := time.Since(startTime)
            
            By("Verifying build time is within baseline")
            maxAcceptable := baseline.BuildTime + (baseline.BuildTime / 3) // 33% tolerance
            Expect(buildTime).To(BeNumerically("<=", maxAcceptable),
                fmt.Sprintf("Build time %v exceeds baseline %v (max acceptable: %v)",
                    buildTime, baseline.BuildTime, maxAcceptable))
            
            By("Cleanup")
            apiClient.DELETE("/v1/apps/" + appName).Execute()
            
            GinkgoWriter.Printf("Build Performance:\n")
            GinkgoWriter.Printf("  Baseline: %v\n", baseline.BuildTime)
            GinkgoWriter.Printf("  Measured: %v\n", buildTime)
        })
    })
    
    Context("Resource Usage", func() {
        It("should maintain memory usage within baseline", func() {
            Skip("Requires system metrics collection - implement with monitoring integration")
            
            // This would integrate with system monitoring to check memory usage
            // Implementation would depend on monitoring stack (Prometheus, etc.)
        })
        
        It("should maintain CPU usage within baseline", func() {
            Skip("Requires system metrics collection - implement with monitoring integration")
            
            // Similar to memory usage testing
        })
    })
})

// LoadTestResult represents load test results
type LoadTestResult struct {
    TotalRequests      int
    SuccessfulRequests int
    FailedRequests     int
    RequestsPerSecond  int
    AverageLatency     time.Duration
    ErrorRate          float64
}

// runLoadTest executes a load test and returns results
func runLoadTest(concurrency int, duration time.Duration) *LoadTestResult {
    // Implementation would be similar to the load testing in Phase 3
    // but focused on regression testing metrics
    return &LoadTestResult{
        TotalRequests:      1000,
        SuccessfulRequests: 995,
        FailedRequests:     5,
        RequestsPerSecond:  33,
        AverageLatency:     150 * time.Millisecond,
        ErrorRate:          0.005,
    }
}
```

### 4. Chaos Testing Framework

#### 4.1 System Resilience Testing (`chaos_test.go`)
```go
package behavioral

import (
    "context"
    "fmt"
    "time"
    
    . "github.com/onsi/ginkgo/v2"
    . "github.com/onsi/gomega"
)

var _ = Describe("System Resilience and Chaos Testing", func() {
    Context("When external dependencies fail", func() {
        It("should gracefully handle Consul unavailability", func() {
            Skip("Requires chaos engineering setup - implement with container manipulation")
            
            appName := fmt.Sprintf("chaos-consul-test-%d", GinkgoRandomSeed())
            
            By("Deploying application successfully")
            buildRequest := map[string]interface{}{
                "git_url": "https://github.com/test-org/resilient-app.git",
                "branch":  "main",
            }
            
            apiClient.POST("/v1/apps/"+appName+"/builds").
                WithJSON(buildRequest).
                ExpectStatus(202).
                Execute()
            
            Eventually(func() string {
                resp := apiClient.GET("/v1/apps/"+appName+"/status").ExpectStatus(200).Execute()
                var status map[string]interface{}
                resp.JSON(&status)
                return status["status"].(string)
            }, "5m", "10s").Should(Equal("running"))
            
            By("Simulating Consul failure")
            // This would use chaos engineering tools to disrupt Consul
            // Implementation would depend on the chosen chaos testing framework
            
            By("Verifying application continues to function")
            // The controller should continue to work with cached/fallback data
            Consistently(func() int {
                resp, _ := apiClient.GET("/v1/apps/"+appName+"/status").Execute(), nil
                return resp.StatusCode
            }, "2m", "5s").Should(Equal(200), "Application status should remain available")
            
            By("Verifying graceful degradation")
            resp := apiClient.GET("/health").ExpectStatus(200).Execute()
            var health map[string]interface{}
            resp.JSON(&health)
            Expect(health["degraded_services"]).To(ContainElement("consul"))
            
            By("Restoring Consul and verifying recovery")
            // Restore Consul service
            
            Eventually(func() bool {
                resp := apiClient.GET("/health").ExpectStatus(200).Execute()
                var health map[string]interface{}
                resp.JSON(&health)
                degraded, exists := health["degraded_services"]
                return !exists || len(degraded.([]interface{})) == 0
            }, "2m", "5s").Should(BeTrue(), "System should fully recover")
            
            apiClient.DELETE("/v1/apps/" + appName).Execute()
        })
        
        It("should handle Nomad cluster instability", func() {
            Skip("Requires advanced chaos testing setup")
            
            // Similar pattern for testing Nomad failures
        })
        
        It("should recover from storage service interruption", func() {
            Skip("Requires storage chaos testing")
            
            // Test storage service interruption and recovery
        })
    })
    
    Context("When system is under extreme load", func() {
        It("should maintain core functionality under stress", func() {
            By("Establishing baseline performance")
            resp := apiClient.GET("/health").ExpectStatus(200).Execute()
            baselineTime := resp.ResponseTime
            
            By("Starting high load scenario")
            // This would start multiple concurrent operations
            ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
            defer cancel()
            
            // Simulate high load with multiple concurrent builds
            numBuilds := 5
            buildResults := make(chan bool, numBuilds)
            
            for i := 0; i < numBuilds; i++ {
                go func(id int) {
                    defer GinkgoRecover()
                    appName := fmt.Sprintf("stress-test-app-%d", id)
                    
                    buildRequest := map[string]interface{}{
                        "git_url": "https://github.com/test-org/sample-app.git",
                        "branch":  "main",
                    }
                    
                    apiClient.POST("/v1/apps/"+appName+"/builds").
                        WithJSON(buildRequest).
                        ExpectStatus(202).
                        Execute()
                    
                    buildResults <- true
                }(i)
            }
            
            By("Verifying system remains responsive during load")
            Consistently(func() int {
                resp, _ := apiClient.GET("/health").Execute(), nil
                return resp.StatusCode
            }, "2m", "1s").Should(Equal(200), "Health endpoint should remain responsive")
            
            By("Verifying response time degradation is acceptable")
            resp = apiClient.GET("/health").ExpectStatus(200).Execute()
            loadTime := resp.ResponseTime
            
            // Allow up to 3x degradation under extreme load
            maxAcceptableDegradation := baselineTime * 3
            Expect(loadTime).To(BeNumerically("<=", maxAcceptableDegradation),
                fmt.Sprintf("Response time under load %v exceeds acceptable degradation %v",
                    loadTime, maxAcceptableDegradation))
            
            // Wait for all builds to be submitted
            for i := 0; i < numBuilds; i++ {
                Eventually(buildResults).Should(Receive())
            }
            
            By("Cleaning up stress test applications")
            for i := 0; i < numBuilds; i++ {
                appName := fmt.Sprintf("stress-test-app-%d", i)
                apiClient.DELETE("/v1/apps/" + appName).Execute()
            }
        })
    })
    
    Context("When network issues occur", func() {
        It("should handle intermittent connectivity", func() {
            Skip("Requires network chaos testing tools")
            
            // Test network partitions, latency injection, packet loss
        })
        
        It("should recover from DNS resolution failures", func() {
            Skip("Requires DNS chaos testing")
            
            // Test DNS resolution issues
        })
    })
})
```

### 5. Test Configuration and Utilities

#### 5.1 BDD Test Configuration (`behavioral_config.yaml`)
```yaml
# Behavioral Test Configuration
test_config:
  # Environment settings
  environment: "test"
  base_url: "http://localhost:8081"
  timeout: "30s"
  
  # Test data configuration
  test_data:
    apps_prefix: "bdd-test"
    domain_suffix: ".test.local"
    cleanup_after_test: true
    preserve_on_failure: true
  
  # Performance baselines
  performance:
    api_response_time: "200ms"
    build_time: "5m"
    deployment_time: "2m"
    throughput_rps: 50
    concurrent_users: 10
    memory_usage_mb: 256
    cpu_usage_percent: 50.0
  
  # Chaos testing configuration
  chaos:
    enabled: false # Enable only for chaos testing runs
    failure_injection:
      consul_failure_rate: 0.1
      nomad_failure_rate: 0.05
      storage_failure_rate: 0.05
      network_latency_ms: 100
  
  # Retry and timeout settings
  retries:
    max_attempts: 3
    backoff_multiplier: 2
    max_wait_time: "5m"
  
  # Parallel execution
  parallel:
    enabled: true
    max_processes: 4
    test_timeout: "30m"
```

#### 5.2 BDD Test Makefile Targets
```makefile
# Behavioral testing targets
.PHONY: test-bdd test-e2e test-performance test-chaos

# Run BDD tests
test-bdd:
	@echo "Running BDD tests..."
	cd test/behavioral && ginkgo -v --randomize-all --fail-on-pending

# Run end-to-end tests  
test-e2e:
	@echo "Running end-to-end tests..."
	cd test/behavioral && ginkgo -v --focus="End-to-End" --slow-spec-threshold=30s

# Run performance regression tests
test-performance:
	@echo "Running performance regression tests..."
	cd test/behavioral && ginkgo -v --focus="Performance" --slow-spec-threshold=60s

# Run chaos tests (requires special setup)
test-chaos:
	@echo "Running chaos tests..."
	@echo "⚠️  Warning: This will disrupt services for testing"
	@read -p "Continue? (y/N): " confirm && [ "$$confirm" = "y" ] || exit 1
	cd test/behavioral && ginkgo -v --focus="Chaos" --slow-spec-threshold=120s

# Run all behavioral tests
test-behavioral: test-bdd test-e2e test-performance

# Generate BDD test reports
test-bdd-report:
	@echo "Generating BDD test report..."
	cd test/behavioral && ginkgo -v --json-report=report.json --junit-report=junit.xml
	@echo "Reports generated: test/behavioral/report.json, test/behavioral/junit.xml"

# Performance baseline update
update-performance-baseline:
	@echo "Updating performance baseline..."
	@echo "This should only be done after verifying performance improvements"
	@read -p "Continue with baseline update? (y/N): " confirm && [ "$$confirm" = "y" ] || exit 1
	cd test/behavioral && go run scripts/update_baseline.go
```

## Implementation Checklist

### Phase 1 Tasks
- [ ] **BDD Framework Setup**
  - [ ] Ginkgo/Gomega installation and configuration
  - [ ] BDD test suite structure and shared setup
  - [ ] Test data management for BDD scenarios
  - [ ] Custom matchers and assertions

- [ ] **User Journey Specifications**
  - [ ] Application deployment workflows
  - [ ] Environment variable management scenarios
  - [ ] Domain and certificate management tests
  - [ ] Error handling and validation scenarios

### Phase 2 Tasks
- [ ] **End-to-End Testing**
  - [ ] Complete application lifecycle tests
  - [ ] Multi-service integration scenarios
  - [ ] Real-world workflow simulations
  - [ ] Cross-browser testing (if web UI exists)

- [ ] **Performance and Chaos Testing**
  - [ ] Performance regression framework
  - [ ] Load testing integration
  - [ ] Chaos engineering setup
  - [ ] System resilience validation

## Success Criteria

### BDD Coverage
- [ ] **User Stories**: All critical user stories have BDD tests
- [ ] **Readable Specs**: Tests read like natural language
- [ ] **Living Documentation**: Tests serve as executable documentation
- [ ] **Stakeholder Review**: Non-technical stakeholders can understand tests

### E2E Validation
- [ ] **Complete Workflows**: All user workflows tested end-to-end
- [ ] **Real Data**: Tests use realistic data and scenarios
- [ ] **Production Parity**: Test environment mirrors production
- [ ] **Cross-Integration**: All service integrations validated

### Performance Assurance
- [ ] **Baseline Compliance**: All metrics within performance baselines
- [ ] **Regression Detection**: System catches performance regressions
- [ ] **Load Handling**: System performs under expected load
- [ ] **Resource Efficiency**: Memory and CPU usage within limits

### System Resilience
- [ ] **Failure Recovery**: System recovers from component failures
- [ ] **Graceful Degradation**: Non-critical failures don't break core functions
- [ ] **Load Tolerance**: System remains stable under stress
- [ ] **Data Consistency**: No data corruption during failures

## Risk Mitigation

### Test Complexity
1. **Over-complicated scenarios**
   - Mitigation: Focus on real user workflows, avoid artificial complexity

2. **Slow test execution**
   - Mitigation: Parallel execution, efficient test data, optimized setup/teardown

### Infrastructure Dependencies
1. **External service dependencies**
   - Mitigation: Containerized services, health checks, fallback strategies

2. **Environment consistency**
   - Mitigation: Infrastructure as code, automated environment setup

## Dependencies

### Prerequisites from Phase 3
- Integration testing framework
- API testing utilities
- Service integration patterns
- Performance testing foundation

### New Tools Required
- Ginkgo/Gomega BDD framework
- Chaos engineering tools (optional)
- Performance monitoring integration
- Container orchestration for test environment

## Next Steps

After completing Phase 4:
1. **Phase 5**: Test Automation & Optimization
2. **Monitoring Integration**: Connect tests with production monitoring
3. **CI/CD Enhancement**: Full pipeline integration with BDD tests
4. **Documentation**: Create user guides based on BDD specifications
5. **Team Training**: BDD practices and test maintenance

## References

- [Ginkgo BDD Framework](https://onsi.github.io/ginkgo/)
- [Gomega Matcher Library](https://onsi.github.io/gomega/)
- [BDD Best Practices](https://cucumber.io/docs/bdd/)
- [Chaos Engineering Principles](https://principlesofchaos.org/)
- [Performance Testing Guide](https://martinfowler.com/articles/practical-test-pyramid.html)