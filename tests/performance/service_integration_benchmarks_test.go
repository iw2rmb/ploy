//go:build performance
// +build performance

package performance

import (
	"bytes"
	"context"
	"fmt"
	"math/rand"
	"testing"
	"time"

	consulapi "github.com/hashicorp/consul/api"
	nomadapi "github.com/hashicorp/nomad/api"

	"github.com/iw2rmb/ploy/internal/git/provider"
	"github.com/iw2rmb/ploy/internal/storage"
)

// setupServiceIntegrationBenchmarkEnvironment creates a service integration test environment
func setupServiceIntegrationBenchmarkEnvironment(b *testing.B) *ServiceIntegrationBenchmarkEnvironment {
	b.Helper()

	// Nomad client
	nomadConfig := nomadapi.DefaultConfig()
	nomadConfig.Address = "http://localhost:4646"
	nomadClient, err := nomadapi.NewClient(nomadConfig)
	if err != nil {
		b.Skip("Nomad not available for service integration benchmarks")
	}

	// Consul client
	consulConfig := consulapi.DefaultConfig()
	consulConfig.Address = "localhost:8500"
	consulClient, err := consulapi.NewClient(consulConfig)
	if err != nil {
		b.Skip("Consul not available for service integration benchmarks")
	}

	// Storage client (SeaweedFS)
	storageClient, err := storage.NewStorageClient(&storage.Config{
		Endpoint: "http://localhost:8888",
	})
	if err != nil {
		b.Skip("SeaweedFS not available for service integration benchmarks")
	}

	// GitLab provider (will use environment variables if available)
	gitProvider, err := provider.NewGitProvider(&provider.Config{
		Type:    "gitlab",
		BaseURL: "https://gitlab.com",
		Token:   "mock-token", // Will fail but that's expected for benchmarks
	})
	if err != nil {
		// Continue with nil provider - benchmarks will log failures
		gitProvider = nil
	}

	env := &ServiceIntegrationBenchmarkEnvironment{
		NomadClient:   nomadClient,
		ConsulClient:  consulClient,
		StorageClient: storageClient,
		GitProvider:   gitProvider,
	}

	return env
}

type ServiceIntegrationBenchmarkEnvironment struct {
	NomadClient   *nomadapi.Client
	ConsulClient  *consulapi.Client
	StorageClient *storage.StorageClient
	GitProvider   provider.GitProvider
}

func (e *ServiceIntegrationBenchmarkEnvironment) Cleanup() {
	// Cleanup any test jobs or resources
}

// generateTestJobSpec creates a realistic Nomad job spec for benchmarking
func generateTestJobSpec(jobID string) *nomadapi.Job {
	return &nomadapi.Job{
		ID:   &jobID,
		Name: &jobID,
		Type: nomadapi.JobTypeBatch.Ptr(),
		TaskGroups: []*nomadapi.TaskGroup{
			{
				Name:  nomadapi.StringToPtr("benchmark-group"),
				Count: nomadapi.IntToPtr(1),
				RestartPolicy: &nomadapi.RestartPolicy{
					Attempts: nomadapi.IntToPtr(1),
					Interval: nomadapi.TimeToPtr(5 * time.Minute),
					Delay:    nomadapi.TimeToPtr(25 * time.Second),
					Mode:     nomadapi.StringToPtr("fail"),
				},
				Tasks: []*nomadapi.Task{
					{
						Name:   "benchmark-task",
						Driver: "raw_exec",
						Config: map[string]interface{}{
							"command": "/bin/echo",
							"args":    []string{"benchmark", "test", jobID},
						},
						Resources: &nomadapi.Resources{
							CPU:      nomadapi.IntToPtr(100),
							MemoryMB: nomadapi.IntToPtr(128),
						},
					},
				},
			},
		},
	}
}

// BenchmarkNomadJobSubmission benchmarks Nomad job submission and monitoring
// This should initially show realistic latencies before optimization
func BenchmarkNomadJobSubmission(b *testing.B) {
	if testing.Short() {
		b.Skip("skipping service integration benchmarks in short mode")
	}

	env := setupServiceIntegrationBenchmarkEnvironment(b)
	defer env.Cleanup()

	// Test Nomad connectivity first
	_, err := env.NomadClient.Status().Leader()
	if err != nil {
		b.Skip("Nomad leader not available")
	}

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		jobID := fmt.Sprintf("benchmark-job-%d-%d", time.Now().Unix(), i)
		job := generateTestJobSpec(jobID)

		startTime := time.Now()

		// Submit job
		response, _, err := env.NomadClient.Jobs().Register(job, nil)
		if err != nil {
			b.Logf("Nomad job submission iteration %d failed: %v", i, err)
			continue
		}

		// Wait for job completion (with timeout)
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		success := false

		// Poll job status
	pollLoop:
		for {
			select {
			case <-ctx.Done():
				b.Logf("Job %s timed out", jobID)
				break pollLoop
			default:
				job, _, err := env.NomadClient.Jobs().Info(jobID, nil)
				if err != nil {
					break pollLoop
				}

				if job.Status != nil {
					switch *job.Status {
					case "dead":
						success = true
						break pollLoop
					case "running":
						// Continue polling
						time.Sleep(100 * time.Millisecond)
					default:
						time.Sleep(100 * time.Millisecond)
					}
				}
			}
		}

		duration := time.Since(startTime)
		cancel()

		// Cleanup
		env.NomadClient.Jobs().Deregister(jobID, true, nil)

		if success {
			b.Logf("Job %s completed in %v (eval: %s)", jobID, duration, response.EvalID)
		}

		// Track if this exceeds our performance target
		if duration > 5*time.Second {
			b.Logf("Job submission %d took %v (exceeds 5s target)", i, duration)
		}
	}
}

// BenchmarkSeaweedFSOperations benchmarks SeaweedFS storage operations
func BenchmarkSeaweedFSOperations(b *testing.B) {
	if testing.Short() {
		b.Skip("skipping service integration benchmarks in short mode")
	}

	env := setupServiceIntegrationBenchmarkEnvironment(b)
	defer env.Cleanup()

	// Test different data sizes
	dataSizes := []int{1024, 4096, 16384, 65536} // 1KB, 4KB, 16KB, 64KB

	for _, size := range dataSizes {
		b.Run(fmt.Sprintf("StoreRetrieve_%dKB", size/1024), func(b *testing.B) {
			testData := make([]byte, size)
			rand.Read(testData)

			b.ResetTimer()
			b.ReportAllocs()

			for i := 0; i < b.N; i++ {
				ctx := context.Background()
				key := fmt.Sprintf("benchmark-data-%d-%d", size, i)

				startTime := time.Now()

				// Store operation
				err := env.StorageClient.Put(ctx, key, bytes.NewReader(testData))
				if err != nil {
					b.Logf("SeaweedFS store iteration %d failed: %v", i, err)
					continue
				}

				storeTime := time.Since(startTime)

				// Retrieve operation
				retrieveStart := time.Now()
				reader, err := env.StorageClient.Get(ctx, key)
				if err != nil {
					b.Logf("SeaweedFS retrieve iteration %d failed: %v", i, err)
					continue
				}

				// Read all data to measure complete retrieval time
				retrievedData := make([]byte, size)
				_, err = reader.Read(retrievedData)
				reader.Close()
				if err != nil {
					b.Logf("SeaweedFS read iteration %d failed: %v", i, err)
					continue
				}

				retrieveTime := time.Since(retrieveStart)

				// Cleanup
				env.StorageClient.Delete(ctx, key)

				// Log performance metrics
				if storeTime > 100*time.Millisecond {
					b.Logf("Store %dKB took %v (exceeds 100ms target)", size/1024, storeTime)
				}
				if retrieveTime > 50*time.Millisecond {
					b.Logf("Retrieve %dKB took %v (exceeds 50ms target)", size/1024, retrieveTime)
				}
			}
		})
	}
}

// BenchmarkConsulKeyValueOperations benchmarks Consul KV operations for KB locking
func BenchmarkConsulKeyValueOperations(b *testing.B) {
	if testing.Short() {
		b.Skip("skipping service integration benchmarks in short mode")
	}

	env := setupServiceIntegrationBenchmarkEnvironment(b)
	defer env.Cleanup()

	// Test Consul connectivity
	_, err := env.ConsulClient.Status().Leader()
	if err != nil {
		b.Skip("Consul leader not available")
	}

	kv := env.ConsulClient.KV()

	b.Run("PutGetDelete", func(b *testing.B) {
		b.ResetTimer()
		b.ReportAllocs()

		for i := 0; i < b.N; i++ {
			key := fmt.Sprintf("benchmark/kb-lock/%d", i)
			value := fmt.Sprintf("lock-value-%d-%d", i, time.Now().UnixNano())

			startTime := time.Now()

			// Put operation
			_, err := kv.Put(&consulapi.KVPair{
				Key:   key,
				Value: []byte(value),
			}, nil)
			if err != nil {
				b.Logf("Consul PUT iteration %d failed: %v", i, err)
				continue
			}

			putTime := time.Since(startTime)

			// Get operation
			getStart := time.Now()
			kvPair, _, err := kv.Get(key, nil)
			if err != nil {
				b.Logf("Consul GET iteration %d failed: %v", i, err)
				continue
			}
			getTime := time.Since(getStart)

			// Verify value
			if kvPair == nil || string(kvPair.Value) != value {
				b.Logf("Consul GET iteration %d returned unexpected value", i)
			}

			// Delete operation
			deleteStart := time.Now()
			_, err = kv.Delete(key, nil)
			if err != nil {
				b.Logf("Consul DELETE iteration %d failed: %v", i, err)
			}
			deleteTime := time.Since(deleteStart)

			// Log performance metrics
			totalTime := putTime + getTime + deleteTime
			if totalTime > 50*time.Millisecond {
				b.Logf("Consul KV operations took %v (put: %v, get: %v, delete: %v)",
					totalTime, putTime, getTime, deleteTime)
			}
		}
	})

	b.Run("LockingMechanism", func(b *testing.B) {
		b.ResetTimer()
		b.ReportAllocs()

		for i := 0; i < b.N; i++ {
			sessionID := fmt.Sprintf("benchmark-session-%d", i)
			key := fmt.Sprintf("benchmark/locks/%d", i)

			startTime := time.Now()

			// Create session for locking
			session := env.ConsulClient.Session()
			sessionEntry := &consulapi.SessionEntry{
				Behavior: "release",
				TTL:      "60s",
			}

			lockSessionID, _, err := session.Create(sessionEntry, nil)
			if err != nil {
				b.Logf("Consul session create iteration %d failed: %v", i, err)
				continue
			}

			// Acquire lock
			acquired, _, err := kv.Acquire(&consulapi.KVPair{
				Key:     key,
				Value:   []byte(sessionID),
				Session: lockSessionID,
			}, nil)
			if err != nil {
				b.Logf("Consul lock acquire iteration %d failed: %v", i, err)
				continue
			}

			lockTime := time.Since(startTime)

			if !acquired {
				b.Logf("Consul lock acquire iteration %d failed to acquire lock", i)
			}

			// Release lock
			releaseStart := time.Now()
			_, _, err = kv.Release(&consulapi.KVPair{
				Key:     key,
				Session: lockSessionID,
			}, nil)
			if err != nil {
				b.Logf("Consul lock release iteration %d failed: %v", i, err)
			}
			releaseTime := time.Since(releaseStart)

			// Destroy session
			_, err = session.Destroy(lockSessionID, nil)
			if err != nil {
				b.Logf("Consul session destroy iteration %d failed: %v", i, err)
			}

			totalTime := lockTime + releaseTime
			if totalTime > 100*time.Millisecond {
				b.Logf("Consul locking took %v (acquire: %v, release: %v)",
					totalTime, lockTime, releaseTime)
			}
		}
	})
}

// BenchmarkGitLabIntegration benchmarks GitLab API operations
func BenchmarkGitLabIntegration(b *testing.B) {
	if testing.Short() {
		b.Skip("skipping service integration benchmarks in short mode")
	}

	env := setupServiceIntegrationBenchmarkEnvironment(b)
	defer env.Cleanup()

	if env.GitProvider == nil {
		b.Skip("GitLab provider not available (expected for benchmarking)")
	}

	// Mock merge request data
	mrData := &provider.MergeRequestData{
		Title:        "Performance Benchmark MR",
		Description:  "This is a benchmark merge request created during performance testing",
		SourceBranch: "feature/benchmark-branch",
		TargetBranch: "main",
		RepoURL:      "https://gitlab.com/test/benchmark-repo.git",
	}

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		// Create unique branch name for each iteration
		mrData.SourceBranch = fmt.Sprintf("feature/benchmark-%d", i)

		startTime := time.Now()

		// Attempt to create MR (will likely fail due to authentication/repo access)
		mr, err := env.GitProvider.CreateMergeRequest(context.Background(), mrData)

		duration := time.Since(startTime)

		if err != nil {
			b.Logf("GitLab MR creation iteration %d failed (expected): %v", i, err)
		} else if mr != nil {
			b.Logf("GitLab MR creation iteration %d succeeded: %s", i, mr.URL)
		}

		// Track performance even for failed requests
		if duration > 2*time.Second {
			b.Logf("GitLab API call %d took %v (exceeds 2s target)", i, duration)
		}
	}
}

// BenchmarkIntegratedServiceWorkflow benchmarks a complete service integration workflow
func BenchmarkIntegratedServiceWorkflow(b *testing.B) {
	if testing.Short() {
		b.Skip("skipping service integration benchmarks in short mode")
	}

	env := setupServiceIntegrationBenchmarkEnvironment(b)
	defer env.Cleanup()

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		workflowID := fmt.Sprintf("benchmark-workflow-%d", i)

		startTime := time.Now()

		// Step 1: Store workflow data in SeaweedFS
		workflowData := fmt.Sprintf(`{"workflow_id": "%s", "step": "integration_test", "timestamp": "%s"}`,
			workflowID, time.Now().Format(time.RFC3339))

		ctx := context.Background()
		storageKey := fmt.Sprintf("workflows/%s/config.json", workflowID)

		err := env.StorageClient.Put(ctx, storageKey, bytes.NewReader([]byte(workflowData)))
		if err != nil {
			b.Logf("Workflow storage iteration %d failed: %v", i, err)
			continue
		}

		// Step 2: Create distributed lock in Consul
		kv := env.ConsulClient.KV()
		lockKey := fmt.Sprintf("workflows/%s/lock", workflowID)

		session := env.ConsulClient.Session()
		sessionEntry := &consulapi.SessionEntry{
			Behavior: "release",
			TTL:      "30s",
		}

		sessionID, _, err := session.Create(sessionEntry, nil)
		if err != nil {
			b.Logf("Workflow session create iteration %d failed: %v", i, err)
			continue
		}

		// Acquire lock
		acquired, _, err := kv.Acquire(&consulapi.KVPair{
			Key:     lockKey,
			Value:   []byte(workflowID),
			Session: sessionID,
		}, nil)
		if err != nil || !acquired {
			b.Logf("Workflow lock acquire iteration %d failed: %v", i, err)
			continue
		}

		// Step 3: Submit Nomad job for workflow execution
		jobID := fmt.Sprintf("workflow-job-%s", workflowID)
		job := generateTestJobSpec(jobID)

		response, _, err := env.NomadClient.Jobs().Register(job, nil)
		if err != nil {
			b.Logf("Workflow job submission iteration %d failed: %v", i, err)
		} else {
			b.Logf("Workflow %s job submitted (eval: %s)", workflowID, response.EvalID)
		}

		// Step 4: Cleanup
		// Release lock
		kv.Release(&consulapi.KVPair{
			Key:     lockKey,
			Session: sessionID,
		}, nil)
		session.Destroy(sessionID, nil)

		// Cleanup job
		if response != nil {
			env.NomadClient.Jobs().Deregister(jobID, true, nil)
		}

		// Delete storage
		env.StorageClient.Delete(ctx, storageKey)

		workflowTime := time.Since(startTime)

		if workflowTime > 10*time.Second {
			b.Logf("Integrated workflow %d took %v (exceeds 10s target)", i, workflowTime)
		}
	}
}

// Performance target constants for service integrations
const (
	MaxNomadJobSubmissionTime = 5 * time.Second        // Target for Nomad job submission
	MaxSeaweedFSStoreTime     = 100 * time.Millisecond // Target for storing 4KB file
	MaxSeaweedFSRetrieveTime  = 50 * time.Millisecond  // Target for retrieving 4KB file
	MaxConsulKVOperationTime  = 50 * time.Millisecond  // Target for Consul KV operations
	MaxGitLabMRCreationTime   = 2 * time.Second        // Target for GitLab MR creation
	MaxWorkflowExecutionTime  = 10 * time.Second       // Target for complete workflow
)

// BenchmarkServiceConnectivityLatency measures baseline connectivity to services
func BenchmarkServiceConnectivityLatency(b *testing.B) {
	env := setupServiceIntegrationBenchmarkEnvironment(b)
	defer env.Cleanup()

	b.Run("NomadStatus", func(b *testing.B) {
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			startTime := time.Now()
			_, err := env.NomadClient.Status().Leader()
			duration := time.Since(startTime)

			if err != nil {
				b.Logf("Nomad status check %d failed: %v", i, err)
			}
			if duration > 100*time.Millisecond {
				b.Logf("Nomad status check %d took %v", i, duration)
			}
		}
	})

	b.Run("ConsulStatus", func(b *testing.B) {
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			startTime := time.Now()
			_, err := env.ConsulClient.Status().Leader()
			duration := time.Since(startTime)

			if err != nil {
				b.Logf("Consul status check %d failed: %v", i, err)
			}
			if duration > 100*time.Millisecond {
				b.Logf("Consul status check %d took %v", i, duration)
			}
		}
	})

	b.Run("SeaweedFSHealth", func(b *testing.B) {
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			startTime := time.Now()
			err := env.StorageClient.Health(context.Background())
			duration := time.Since(startTime)

			if err != nil {
				b.Logf("SeaweedFS health check %d failed: %v", i, err)
			}
			if duration > 100*time.Millisecond {
				b.Logf("SeaweedFS health check %d took %v", i, duration)
			}
		}
	})
}
