package mods

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"strings"
	"time"

	gitapi "github.com/iw2rmb/ploy/api/git"
	"github.com/iw2rmb/ploy/internal/cli/common"
	"github.com/iw2rmb/ploy/internal/git/provider"
	"github.com/iw2rmb/ploy/internal/orchestration"
	"github.com/iw2rmb/ploy/internal/storage"
)

// Note: ARF-based Git/Recipe execution wrappers removed with ARF transforms HTTP removal.

// SharedPushBuildChecker implements build checking using SharedPush
type SharedPushBuildChecker struct {
	controllerURL string
}

// NewSharedPushBuildChecker creates a new build checker using SharedPush
func NewSharedPushBuildChecker(controllerURL string) *SharedPushBuildChecker {
	return &SharedPushBuildChecker{
		controllerURL: controllerURL,
	}
}

// CheckBuild performs a build check using the existing SharedPush infrastructure
func (b *SharedPushBuildChecker) CheckBuild(ctx context.Context, config common.DeployConfig) (*common.DeployResult, error) {
	if shouldSkipRemoteBuild(config.Lane) {
		log.Printf("[Mods Build] Skipping remote build for lane=%s due to MODS_SKIP_DEPLOY_LANES", config.Lane)
		if modID := os.Getenv("MOD_ID"); modID != "" {
			rep := NewControllerEventReporter(b.controllerURL, modID)
			_ = rep.Report(ctx, Event{Phase: "build", Step: "build-gate", Level: "info", Message: fmt.Sprintf("skipped remote build for lane=%s", config.Lane)})
		}
		return &common.DeployResult{Success: true, Message: "Remote build skipped"}, nil
	}
	// Set the controller URL in the config
	config.ControllerURL = b.controllerURL
	config.IsPlatform = false // Mods uses ploy mode, not ployman mode
	// Mods build gate must be ephemeral; ask API to tear down sandboxed app after gate
	config.BuildOnly = true

	// Propagate working directory hint from metadata if present
	if config.Metadata != nil {
		if wd, ok := config.Metadata["working_dir"]; ok && wd != "" {
			config.WorkingDir = wd
		}
	}
	// Optional: emit via controller reporter if exec ID present
	if modID := os.Getenv("MOD_ID"); modID != "" {
		rep := NewControllerEventReporter(b.controllerURL, modID)
		_ = rep.Report(ctx, Event{Phase: "build", Step: "build-gate", Level: "info", Message: fmt.Sprintf("start app=%s lane=%s env=%s wd=%s", config.App, config.Lane, config.Environment, config.WorkingDir)})
	}
	log.Printf("[Mods Build] Starting build check: controller=%s app=%s lane=%s env=%s wd=%s", b.controllerURL, config.App, config.Lane, config.Environment, config.WorkingDir)

	// Use SharedPush to perform the build check
	// SharedPush already supports build-only mode when used with specific endpoints
	result, err := common.SharedPush(config)
	if err != nil {
		// Emit additional context for debugging 500s
		if modID := os.Getenv("MOD_ID"); modID != "" {
			rep := NewControllerEventReporter(b.controllerURL, modID)
			_ = rep.Report(ctx, Event{Phase: "build", Step: "build-gate-error", Level: "error", Message: err.Error()})
		}
		return nil, fmt.Errorf("build check failed (controller=%s app=%s lane=%s env=%s): %w", b.controllerURL, config.App, config.Lane, config.Environment, err)
	}

	if result != nil && !result.Success {
		// Normalize the controller response into a readable message and capture builder logs (if present).
		result.Message = enrichBuildFailureMessage(result.Message)

		// Attempt to fetch build logs to enrich error for downstream healing
		logs := fetchBuildLogs(b.controllerURL, config.App, result.DeploymentID)
		if logs != "" {
			tail := logs
			if len(tail) > 2000 {
				tail = tail[len(tail)-2000:]
			}
			// Append logs tail to result.Message
			result.Message = appendUniqueLine(result.Message, tail)
		}
		if modID := os.Getenv("MOD_ID"); modID != "" {
			rep := NewControllerEventReporter(b.controllerURL, modID)
			// Include deployment id if available
			dep := result.DeploymentID
			msg := fmt.Sprintf("unsuccessful: %s", result.Message)
			if strings.TrimSpace(dep) != "" {
				msg = fmt.Sprintf("%s (deployment_id=%s)", msg, dep)
			}
			_ = rep.Report(ctx, Event{Phase: "build", Step: "build-gate", Level: "error", Message: msg})
		}
		log.Printf("[Mods Build] Unsuccessful build: controller=%s app=%s lane=%s env=%s msg=%s", b.controllerURL, config.App, config.Lane, config.Environment, result.Message)
		return result, fmt.Errorf("build check unsuccessful (controller=%s app=%s lane=%s env=%s): %s", b.controllerURL, config.App, config.Lane, config.Environment, result.Message)
	}
	if modID := os.Getenv("MOD_ID"); modID != "" {
		rep := NewControllerEventReporter(b.controllerURL, modID)
		msg := fmt.Sprintf("succeeded version=%s", result.Version)
		if strings.TrimSpace(result.DeploymentID) != "" {
			msg = fmt.Sprintf("%s (deployment_id=%s)", msg, result.DeploymentID)
		}
		_ = rep.Report(ctx, Event{Phase: "build", Step: "build-gate", Level: "info", Message: msg})
	}
	log.Printf("[Mods Build] Build check succeeded: controller=%s app=%s lane=%s env=%s version=%s", b.controllerURL, config.App, config.Lane, config.Environment, result.Version)
	return result, nil
}

// enrichBuildFailureMessage attempts to convert the raw controller error payload into a readable message.
// It extracts `error.message`, `error.details`, and any builder logs (`builder.logs` or top-level `logs` fields),
// ensuring the returned string surfaces actionable diagnostics instead of raw JSON blobs.
func enrichBuildFailureMessage(raw string) string {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return ""
	}

	type builderPayload struct {
		Error struct {
			Code    string      `json:"code"`
			Message string      `json:"message"`
			Details interface{} `json:"details"`
		} `json:"error"`
		Builder struct {
			Logs string `json:"logs"`
		} `json:"builder"`
		Logs string `json:"logs"`
	}

	var messageLines []string
	var builderLogs string

	appendIfNew := func(line string) {
		line = strings.TrimSpace(line)
		if line == "" {
			return
		}
		for _, existing := range messageLines {
			if existing == line {
				return
			}
		}
		messageLines = append(messageLines, line)
	}

	segments := strings.Split(trimmed, "\n")
	parsed := false
	for _, segment := range segments {
		seg := strings.TrimSpace(segment)
		if seg == "" {
			continue
		}
		var payload builderPayload
		if json.Unmarshal([]byte(seg), &payload) == nil {
			parsed = true
			if msg := strings.TrimSpace(payload.Error.Message); msg != "" {
				appendIfNew(msg)
			}
			if payload.Error.Details != nil {
				if detail := strings.TrimSpace(fmt.Sprint(payload.Error.Details)); detail != "" {
					appendIfNew(detail)
				}
			}
			if logs := strings.TrimSpace(payload.Builder.Logs); logs != "" {
				builderLogs = logs
			}
			if logs := strings.TrimSpace(payload.Logs); logs != "" {
				builderLogs = logs
			}
			continue
		}
		appendIfNew(seg)
	}

	if builderLogs != "" {
		appendIfNew(builderLogs)
	}

	if len(messageLines) == 0 {
		// Fallback to original payload if JSON parsing failed entirely.
		return trimmed
	}

	sanitized := strings.Join(messageLines, "\n")
	// If the sanitized output is empty (shouldn't happen), use the original raw payload.
	if strings.TrimSpace(sanitized) == "" && !parsed {
		return trimmed
	}
	return sanitized
}

// appendUniqueLine appends a new log fragment to an existing message, avoiding duplicate blocks.
func appendUniqueLine(message, addition string) string {
	addition = strings.TrimSpace(addition)
	if addition == "" {
		return strings.TrimSpace(message)
	}
	if strings.Contains(message, addition) {
		return strings.TrimSpace(message)
	}
	if strings.TrimSpace(message) == "" {
		return addition
	}
	return strings.TrimSpace(message + "\n" + addition)
}

func shouldSkipRemoteBuild(lane string) bool {
	list := strings.TrimSpace(os.Getenv("MODS_SKIP_DEPLOY_LANES"))
	if list == "" {
		return false
	}
	for _, part := range strings.Split(list, ",") {
		if strings.EqualFold(strings.TrimSpace(part), lane) {
			return true
		}
	}
	return false
}

// fetchBuildLogs best-effort fetches recent build logs from the controller for a given deployment ID
func fetchBuildLogs(controller, app, id string) string {
	if controller == "" || app == "" || id == "" {
		return ""
	}
	// Normalize base and construct URL: /v1/apps/:app/builds/:id/logs?lines=400
	base := strings.TrimRight(controller, "/")
	url := fmt.Sprintf("%s/apps/%s/builds/%s/logs?lines=1200", base, app, id)
	client := &http.Client{Timeout: 20 * time.Second}
	req, _ := http.NewRequest(http.MethodGet, url, nil)
	resp, err := client.Do(req)
	if err != nil || resp == nil || resp.Body == nil {
		return ""
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return ""
	}
	b, _ := io.ReadAll(resp.Body)
	// Prefer extracting the 'logs' field if JSON; else fall back to raw body
	type respJSON struct {
		Logs string `json:"logs"`
	}
	var rj respJSON
	s := string(b)
	if json.Unmarshal(b, &rj) == nil && rj.Logs != "" {
		logs := rj.Logs
		if len(logs) > 12000 { // ~12KB cap for message enrichment
			logs = logs[len(logs)-12000:]
		}
		return logs
	}
	if len(s) > 8000 {
		return s[len(s)-8000:]
	}
	return s
}

// TestModeBuildChecker implements build checking for testing without external dependencies
type TestModeBuildChecker struct {
	shouldFail bool
}

// NewTestModeBuildChecker creates a new test mode build checker
func NewTestModeBuildChecker(shouldFail bool) *TestModeBuildChecker {
	return &TestModeBuildChecker{
		shouldFail: shouldFail,
	}
}

// CheckBuild performs a mock build check
func (m *TestModeBuildChecker) CheckBuild(ctx context.Context, config common.DeployConfig) (*common.DeployResult, error) {
	if m.shouldFail {
		return &common.DeployResult{
			Success: false,
			Message: "Mock build failed for testing",
		}, nil
	}

	return &common.DeployResult{
		Success:      true,
		Message:      "Mock build succeeded",
		Version:      "mock-v1.0.0",
		DeploymentID: "mock-deployment-123",
		URL:          "mock://test-image:latest",
	}, nil
}

// ModsIntegrations provides factory methods for creating concrete implementations
type ModIntegrations struct {
	ControllerURL string
	WorkDir       string
	TestMode      bool // Use mock implementations when true
}

// NewModIntegrations creates a new integrations factory
func NewModIntegrations(controllerURL, workDir string) *ModIntegrations {
	return &ModIntegrations{
		ControllerURL: controllerURL,
		WorkDir:       workDir,
		TestMode:      false,
	}
}

// NewModIntegrationsWithTestMode creates a new integrations factory with test mode option
func NewModIntegrationsWithTestMode(controllerURL, workDir string, testMode bool) *ModIntegrations {
	return &ModIntegrations{
		ControllerURL: controllerURL,
		WorkDir:       workDir,
		TestMode:      testMode,
	}
}

// APIGitOperations wraps the API git service to satisfy the Mods interface
type APIGitOperations struct{ *gitapi.Service }

func NewAPIGitOperations(workDir string) *APIGitOperations {
	return &APIGitOperations{Service: gitapi.NewGitOperations(workDir)}
}

func (g *APIGitOperations) PushBranchAsync(ctx context.Context, repoPath, remoteURL, branchName string) *gitapi.Operation {
	return g.Service.PushBranchAsync(ctx, gitapi.PushRequest{RepoPath: repoPath, RemoteURL: remoteURL, Branch: branchName})
}

// CreateGitOperations creates a Git operations implementation
func (i *ModIntegrations) CreateGitOperations() GitOperationsInterface {
	if i.TestMode {
		return NewMockGitOperations() // Use mock implementation for testing
	}
	return NewAPIGitOperations(i.WorkDir)
}

// CreateRecipeExecutor creates a recipe executor implementation
func (i *ModIntegrations) CreateRecipeExecutor() RecipeExecutorInterface {
	if i.TestMode {
		return NewMockRecipeExecutor() // Use mock implementation for testing
	}
	// No-op in production for now; Mods does not rely on ARF recipe executor
	return NewMockRecipeExecutor()
}

// CreateBuildChecker creates a build checker implementation
func (i *ModIntegrations) CreateBuildChecker() BuildCheckerInterface {
	if i.TestMode {
		return NewMockBuildChecker() // Use mock implementation for testing
	}
	return NewSharedPushBuildChecker(i.ControllerURL)
}

// CreateGitProvider creates a Git provider implementation for MR operations
func (i *ModIntegrations) CreateGitProvider() provider.GitProvider {
	if i.TestMode {
		return NewMockGitProvider() // Use mock implementation for testing
	}
	return provider.NewGitLabProvider()
}

// CreateKBIntegration creates a KB integration for learning from healing attempts
func (i *ModIntegrations) CreateKBIntegration() KBIntegrator {
	if i.TestMode {
		// Return mock KB integration for testing
		return NewMockKBIntegration()
	}

	// Create production KB integration
	storageConfig := storage.Config{
		Master:      "localhost:9333", // Default SeaweedFS master
		Filer:       "localhost:8888", // Default SeaweedFS filer
		Collection:  "kb",
		Replication: "000", // No replication for development
		Timeout:     30,
	}

	storageClient, err := storage.New(storageConfig)
	if err != nil {
		// If storage creation fails, use a mock KB integration to prevent breaking the workflow
		return NewMockKBIntegration()
	}

	// Adapt StorageProvider to Storage interface
	storageBackend := storage.NewStorageAdapter(storageClient)

	kvStore := orchestration.NewKV()

	kbConfig := DefaultKBConfig()
	return NewKBIntegration(storageBackend, kvStore, kbConfig)
}

// CreateConfiguredRunner creates a fully configured ModRunner with KB learning integration
func (i *ModIntegrations) CreateConfiguredRunner(config *ModConfig) (*ModRunner, error) {
	// Create KB integration
	kbIntegration := i.CreateKBIntegration()

	// Create KB-enhanced runner
	kbRunner, err := NewKBModRunner(config, i.WorkDir, kbIntegration)
	if err != nil {
		return nil, err
	}

	// Get the embedded ModRunner for dependency injection
	runner := kbRunner.ModRunner

	// Inject the concrete implementations
	runner.SetGitOperations(i.CreateGitOperations())
	runner.SetRecipeExecutor(i.CreateRecipeExecutor())
	runner.SetBuildChecker(i.CreateBuildChecker())
	runner.SetGitProvider(i.CreateGitProvider())
	// Wire default modular adapters
	runner.SetTransformationExecutor(NewTransformationExecutorAdapter(runner))
	// BuildGate is already provided via SetBuildChecker; Repo/MR modules via SetGitOperations/SetGitProvider

	// Prefer production healing via Nomad; do not set a test submitter by default.
	// Tests can inject a mock submitter explicitly when needed.
	runner.SetJobSubmitter(nil)
	// Wire default production healing orchestrator (wraps existing fanout)
	runner.SetHealingOrchestrator(NewProdHealingOrchestrator(runner.jobSubmitter, runner))

	// Return the embedded ModRunner; KB integration remains wired inside kbRunner
	return runner, nil
}

// GetDefaultControllerURL returns the default controller URL for transflow operations
func GetDefaultControllerURL() string {
	// First check if PLOY_CONTROLLER is explicitly set
	if url := os.Getenv("PLOY_CONTROLLER"); url != "" {
		return url
	}

	// Check if PLOY_APPS_DOMAIN is set for SSL endpoint
	if domain := os.Getenv("PLOY_APPS_DOMAIN"); domain != "" {
		// Check for environment-specific subdomain
		if env := os.Getenv("PLOY_ENVIRONMENT"); env == "dev" {
			return fmt.Sprintf("https://api.dev.%s/v1", domain)
		}
		return fmt.Sprintf("https://api.%s/v1", domain)
	}

	// Default fallback for development
	return "https://api.dev.ployman.app/v1"
}
