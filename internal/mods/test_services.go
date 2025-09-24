package mods

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"strings"
	"testing"
	"time"

	consulapi "github.com/hashicorp/consul/api"
	nomadapi "github.com/hashicorp/nomad/api"
	"github.com/iw2rmb/ploy/internal/storage"
	"github.com/iw2rmb/ploy/internal/storage/factory"
)

// ServicesConfig holds configuration for service integration tests
type ServicesConfig struct {
	ConsulAddr      string
	NomadAddr       string
	SeaweedFSFiler  string
	SeaweedFSMaster string
	GitLabURL       string
	GitLabToken     string
}

// RequireServices enforces that services are running - no fallback to mocks
func RequireServices(t *testing.T, services ...string) *ServicesConfig {
	t.Helper()
	harness := ResolveHarnessFromEnv()
	config := &ServicesConfig{}
	var failures []string
	for _, service := range services {
		switch service {
		case "consul":
			if !isConsulHealthy() {
				failures = append(failures, "Consul not available at localhost:8500")
			} else {
				config.ConsulAddr = "localhost:8500"
			}
		case "nomad":
			if !isNomadHealthy() {
				failures = append(failures, "Nomad not available at http://localhost:4646")
			} else {
				config.NomadAddr = "http://localhost:4646"
			}
		case "seaweedfs":
			filerURL := harness.Infra.SeaweedURL
			if filerURL == "" {
				filerURL = "http://localhost:8888"
			}
			masterHost := harness.SeaweedMasterHost()
			if masterHost == "" {
				masterHost = "localhost:9333"
			}
			if !isSeaweedFSHealthy(harness) {
				failures = append(failures, fmt.Sprintf("SeaweedFS not available at %s (master %s)", filerURL, masterHost))
			} else {
				config.SeaweedFSFiler = filerURL
				config.SeaweedFSMaster = fmt.Sprintf("http://%s", masterHost)
			}
		case "gitlab":
			token := os.Getenv("GITLAB_TOKEN")
			if token == "" {
				failures = append(failures, "GITLAB_TOKEN environment variable required for real GitLab testing")
			} else {
				config.GitLabURL = "https://gitlab.com"
				config.GitLabToken = token
			}
		default:
			failures = append(failures, fmt.Sprintf("Unknown service: %s", service))
		}
	}
	if len(failures) > 0 {
		t.Fatalf("Required services not available:\n%s\n\nSetup:\n1. Run: docker-compose -f docker-compose.integration.yml up -d\n2. Set GITLAB_TOKEN environment variable for GitLab tests\n3. Wait for services to be healthy", strings.Join(failures, "\n"))
	}
	return config
}

// Health checks
func isConsulHealthy() bool {
	client, err := consulapi.NewClient(consulapi.DefaultConfig())
	if err != nil {
		return false
	}
	_, err = client.Status().Leader()
	return err == nil
}

func isNomadHealthy() bool {
	client, err := nomadapi.NewClient(nomadapi.DefaultConfig())
	if err != nil {
		return false
	}
	_, err = client.Status().Leader()
	return err == nil
}

func isSeaweedFSHealthy(h HarnessConfig) bool {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	filer := h.Infra.SeaweedURL
	if filer == "" {
		filer = "http://localhost:8888"
	}
	master := h.SeaweedMasterHost()
	if master == "" {
		master = "localhost:9333"
	}
	if !strings.HasSuffix(filer, "/") {
		filer += "/"
	}
	masterURL := fmt.Sprintf("http://%s/cluster/status", master)
	return isServiceHealthyHTTP(ctx, filer) && isServiceHealthyHTTP(ctx, masterURL)
}

func isServiceHealthyHTTP(ctx context.Context, url string) bool {
	client := &http.Client{Timeout: 2 * time.Second}
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return false
	}
	resp, err := client.Do(req)
	if err != nil {
		return false
	}
	defer func() { _ = resp.Body.Close() }()
	return resp.StatusCode >= 200 && resp.StatusCode < 300
}

// Service operation tests
func testSeaweedFSOperations(t *testing.T, filerURL, master string) {
	t.Helper()
	storageClient, err := factory.New(factory.FactoryConfig{Provider: "seaweedfs", Endpoint: filerURL, Extra: map[string]interface{}{"master": master, "filer": filerURL}})
	if err != nil {
		t.Fatalf("failed to create SeaweedFS client: %v", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	testKey := fmt.Sprintf("mods/integration-test/%d", time.Now().Unix())
	testData := []byte("SeaweedFS integration test data")
	if err := storageClient.Put(ctx, testKey, strings.NewReader(string(testData))); err != nil {
		t.Fatalf("Failed to store data in SeaweedFS: %v", err)
	}
	rd, err := storageClient.Get(ctx, testKey)
	if err != nil {
		t.Fatalf("Failed to retrieve data from SeaweedFS: %v", err)
	}
	buf := make([]byte, len(testData))
	n, err := rd.Read(buf)
	if err != nil {
		t.Fatalf("Failed to read retrieved data: %v", err)
	}
	_ = rd.Close()
	if n != len(testData) || string(buf[:n]) != string(testData) {
		t.Fatalf("Retrieved data doesn't match stored data")
	}
	objs, err := storageClient.List(ctx, storage.ListOptions{Prefix: "mods/integration-test/"})
	if err != nil {
		t.Fatalf("Failed to list objects: %v", err)
	}
	found := false
	for _, o := range objs {
		if o.Key == testKey {
			found = true
		}
	}
	if !found {
		t.Fatalf("Stored key not found in list operation")
	}
	if err := storageClient.Delete(ctx, testKey); err != nil {
		t.Fatalf("Failed to delete data from SeaweedFS: %v", err)
	}
	if _, err := storageClient.Get(ctx, testKey); err == nil {
		t.Fatal("Expected error when retrieving deleted key, but got none")
	}
}

func testNomadOperations(t *testing.T, nomadAddr string) {
	t.Helper()
	config := nomadapi.DefaultConfig()
	config.Address = nomadAddr
	client, err := nomadapi.NewClient(config)
	if err != nil {
		t.Fatalf("failed to create Nomad client: %v", err)
	}
	if _, err = client.Status().Leader(); err != nil {
		t.Fatalf("Failed to connect to Nomad: %v", err)
	}
	if _, _, err = client.Jobs().List(&nomadapi.QueryOptions{}); err != nil {
		t.Fatalf("Failed to list Nomad jobs: %v", err)
	}
}

func testConsulOperations(t *testing.T, consulAddr string) {
	t.Helper()
	config := consulapi.DefaultConfig()
	config.Address = consulAddr
	client, err := consulapi.NewClient(config)
	if err != nil {
		t.Fatalf("failed to create Consul client: %v", err)
	}
	testKey := fmt.Sprintf("mods/integration-test/%d", time.Now().Unix())
	testValue := []byte("Consul integration test data")
	kv := client.KV()
	if _, err = kv.Put(&consulapi.KVPair{Key: testKey, Value: testValue}, &consulapi.WriteOptions{}); err != nil {
		t.Fatalf("Failed to store data in Consul: %v", err)
	}
	pair, _, err := kv.Get(testKey, &consulapi.QueryOptions{})
	if err != nil || pair == nil {
		t.Fatalf("Failed to retrieve data from Consul: %v", err)
	}
	if string(pair.Value) != string(testValue) {
		t.Fatalf("Retrieved data doesn't match")
	}
	pairs, _, err := kv.List("mods/integration-test/", &consulapi.QueryOptions{})
	if err != nil {
		t.Fatalf("Failed to list keys from Consul: %v", err)
	}
	found := false
	for _, p := range pairs {
		if p.Key == testKey {
			found = true
		}
	}
	if !found {
		t.Fatalf("Stored key not found in list operation")
	}
	if _, err = kv.Delete(testKey, &consulapi.WriteOptions{}); err != nil {
		t.Fatalf("Failed to delete data from Consul: %v", err)
	}
	pair, _, err = kv.Get(testKey, &consulapi.QueryOptions{})
	if err != nil {
		t.Fatalf("Unexpected error checking deleted key: %v", err)
	}
	if pair != nil {
		t.Fatal("Expected key to be deleted but still exists")
	}
}

func testGitLabOperations(t *testing.T, gitlabURL, token string) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	client := &http.Client{Timeout: 10 * time.Second}
	req, err := http.NewRequestWithContext(ctx, "GET", gitlabURL+"/api/v4/user", nil)
	if err != nil {
		t.Fatalf("Failed to create GitLab request: %v", err)
	}
	req.Header.Set("Authorization", "Bearer "+token)
	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("Failed to connect to GitLab API: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("GitLab API returned status %d", resp.StatusCode)
	}
	projReq, err := http.NewRequestWithContext(ctx, "GET", gitlabURL+"/api/v4/projects?simple=true&per_page=1", nil)
	if err != nil {
		t.Fatalf("Failed to create GitLab projects request: %v", err)
	}
	projReq.Header.Set("Authorization", "Bearer "+token)
	projResp, err := client.Do(projReq)
	if err != nil {
		t.Fatalf("Failed to fetch GitLab projects: %v", err)
	}
	defer func() { _ = projResp.Body.Close() }()
	if projResp.StatusCode != http.StatusOK {
		t.Fatalf("GitLab projects API returned status %d", projResp.StatusCode)
	}
}

// Validation helpers
func validateServiceUsage(t *testing.T, result *ModResult, serviceConfig *ServicesConfig) {
	t.Helper()
	if result.BranchName == "" {
		t.Error("Expected branch name but got empty string")
	}
	if len(result.StepResults) == 0 {
		t.Error("Expected step results but got none")
	}
	storageClient, err := factory.New(factory.FactoryConfig{Provider: "seaweedfs", Endpoint: serviceConfig.SeaweedFSFiler, Extra: map[string]interface{}{"master": serviceConfig.SeaweedFSMaster, "filer": serviceConfig.SeaweedFSFiler}})
	if err == nil {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if objs, err := storageClient.List(ctx, storage.ListOptions{Prefix: "mods/"}); err == nil && len(objs) > 0 {
			t.Logf("Found %d Mods-related artifacts in SeaweedFS storage", len(objs))
		}
	}
	for _, sr := range result.StepResults {
		if !sr.Success {
			t.Errorf("Step %s failed: %s", sr.StepID, sr.Message)
		}
	}
}

func validateNomadUsage(t *testing.T, result *ModResult, serviceConfig *ServicesConfig) {
	t.Helper()
	if result.BranchName == "" {
		t.Error("Expected branch name but got empty string")
	}
	config := nomadapi.DefaultConfig()
	config.Address = serviceConfig.NomadAddr
	client, err := nomadapi.NewClient(config)
	if err == nil {
		if jobs, _, err := client.Jobs().List(&nomadapi.QueryOptions{}); err == nil {
			modsJobs := 0
			for _, j := range jobs {
				if strings.Contains(j.Name, "mods") {
					modsJobs++
				}
			}
			if modsJobs > 0 {
				t.Logf("Found %d Mods-related jobs in Nomad", modsJobs)
			}
		}
	}
	for _, sr := range result.StepResults {
		if !sr.Success {
			t.Errorf("Step %s failed: %s", sr.StepID, sr.Message)
		}
	}
}

func validateConsulUsage(t *testing.T, result *ModResult, serviceConfig *ServicesConfig) {
	t.Helper()
	if result.BranchName == "" {
		t.Error("Expected branch name but got empty string")
	}
	config := consulapi.DefaultConfig()
	config.Address = serviceConfig.ConsulAddr
	client, err := consulapi.NewClient(config)
	if err == nil {
		kv := client.KV()
		if pairs, _, err := kv.List("mods/", &consulapi.QueryOptions{}); err == nil && len(pairs) > 0 {
			t.Logf("Found %d Mods-related keys in Consul KV", len(pairs))
		}
	}
	for _, sr := range result.StepResults {
		if !sr.Success {
			t.Errorf("Step %s failed: %s", sr.StepID, sr.Message)
		}
	}
}

func validateGitLabUsage(t *testing.T, result *ModResult, serviceConfig *ServicesConfig) {
	t.Helper()
	if result.BranchName == "" {
		t.Error("Expected branch name but got empty string")
	}
	for _, sr := range result.StepResults {
		if !sr.Success {
			t.Errorf("Step %s failed: %s", sr.StepID, sr.Message)
		}
	}
}

func validateAllServicesUsage(t *testing.T, result *ModResult, serviceConfig *ServicesConfig) {
	t.Helper()
	validateServiceUsage(t, result, serviceConfig)
	validateNomadUsage(t, result, serviceConfig)
	validateConsulUsage(t, result, serviceConfig)
	validateGitLabUsage(t, result, serviceConfig)
	if len(result.StepResults) < 5 {
		t.Errorf("Expected at least 5 workflow steps, got %d", len(result.StepResults))
	}
}
