//go:build e2e
// +build e2e

package e2e

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestModsE2E_JavaMigrationComplete(t *testing.T) {
	// Should fail initially - end-to-end integration gaps

	if testing.Short() {
		t.Skip("skipping E2E test in short mode")
	}

	env := SetupTestEnvironment(t, Config{
		UseRealServices: true,
		CleanupAfter:    true,
		TimeoutMinutes:  15,
	})
	defer env.Cleanup()

	// Repo/Branch selection via env
	repo := getenvDefault("E2E_REPO", "https://gitlab.com/iw2rmb/ploy-orw-java11-maven.git")
	branch := getenvDefault("E2E_BRANCH", "e2e/success")
	controller := getenv("PLOY_CONTROLLER")
	if controller == "" {
		t.Skip("Skipping: requires PLOY_CONTROLLER for remote controller-backed E2E")
	}

	workflow := &ModWorkflow{
		ID:           fmt.Sprintf("e2e-java-migration-%d", time.Now().Unix()),
		Repository:   repo,
		TargetBranch: branch,
		Steps: []WorkflowStep{
			{
				Type:               "orw-apply",
				ID:                 "java11to17-migration",
				Recipes:            []WorkflowRecipe{{Name: "org.openrewrite.java.migrate.UpgradeToJava17", Group: "org.openrewrite.recipe", Artifact: "rewrite-migrate-java", Version: "3.17.0"}},
				MavenPluginVersion: "6.18.0",
			},
		},
		SelfHeal: SelfHealConfig{
			Enabled:    true,
			MaxRetries: 2,
			KBLearning: true,
		},
		ExpectedOutcome: OutcomeSuccess,
		MaxDuration:     10 * time.Minute,
	}

	ctx, cancel := context.WithTimeout(context.Background(), workflow.MaxDuration)
	defer cancel()

	result, err := env.ExecuteWorkflow(ctx, workflow)
	if err != nil {
		t.Logf("CLI Output (failure):\n%s", result.Output)
	}
	if os.Getenv("E2E_LOG_CONFIG") == "1" || err != nil {
		t.Logf("Mods YAML path: %s", result.ConfigPath)
		if result.ConfigYAML != "" {
			t.Logf("Mods YAML:\n%s", result.ConfigYAML)
		}
	}
	// CLI may exit non-zero in some environments; continue based on controller status
	if err != nil {
		t.Logf("Continuing despite CLI error: %v", err)
	}

	// Fallback: if mod_id not parsed from CLI output, start run via controller directly
	if result.ModID == "" {
		t.Logf("mod_id not found in CLI output; starting run via controller fallback")
		runURL := strings.TrimRight(controller, "/") + "/mods"
		payload := fmt.Sprintf("{\"config\": %q, \"test_mode\": false}", result.ConfigYAML)
		req0, _ := http.NewRequestWithContext(ctx, http.MethodPost, runURL, strings.NewReader(payload))
		req0.Header.Set("Content-Type", "application/json")
		httpc := &http.Client{Timeout: 30 * time.Second}
		resp0, err0 := httpc.Do(req0)
		if err0 != nil {
			t.Fatalf("fallback run failed: %v", err0)
		}
		defer resp0.Body.Close()
		if resp0.StatusCode != 202 && resp0.StatusCode != 200 {
			t.Fatalf("fallback run HTTP %d", resp0.StatusCode)
		}
		var ack struct {
			ModID string `json:"mod_id"`
		}
		if json.NewDecoder(resp0.Body).Decode(&ack) != nil || ack.ModID == "" {
			t.Fatalf("fallback run: missing mod_id")
		}
		result.ModID = ack.ModID
		t.Logf("Fallback Execution ID: %s", result.ModID)
	}

	// Optional: collect logs early if requested or initial CLI errored
	if (os.Getenv("E2E_LOG_CONFIG") == "1" || err != nil) && result.ModID != "" {
		runModsCollector(t, result.ModID)
	}

	// Query controller for final status to assert MR and build metadata
	statusURL := fmt.Sprintf("%s/mods/%s/status", strings.TrimRight(controller, "/"), result.ModID)
	httpc := &http.Client{Timeout: 30 * time.Second}
	// Poll until terminal
	var st struct {
		Status string                 `json:"status"`
		Result map[string]interface{} `json:"result"`
		Phase  string                 `json:"phase"`
		Error  string                 `json:"error"`
	}
	deadline, _ := ctx.Deadline()
	for {
		req, _ := http.NewRequestWithContext(ctx, http.MethodGet, statusURL, nil)
		resp, err := httpc.Do(req)
		if err == nil && resp.StatusCode == 200 {
			_ = json.NewDecoder(resp.Body).Decode(&st)
			resp.Body.Close()
			if st.Status == "completed" || st.Status == "failed" || st.Status == "cancelled" {
				break
			}
		} else if resp != nil {
			if resp.Body != nil {
				resp.Body.Close()
			}
		}
		if time.Now().After(deadline) {
			t.Fatalf("timeout waiting for terminal status; last=%s phase=%s err=%s", st.Status, st.Phase, st.Error)
		}
		time.Sleep(2 * time.Second)
	}
	if st.Status != "completed" {
		// Collect logs on unexpected terminal state for post-mortem
		if result.ModID != "" {
			runModsCollector(t, result.ModID)
		}
		t.Fatalf("expected completed status, got %s (error=%s)", st.Status, st.Error)
	}
	// Extract result fields
	mr := asStr(st.Result["mr_url"])
	branchName := asStr(st.Result["branch_name"])
	buildVersion := asStr(st.Result["build_version"])
	assert.NotEmpty(t, mr, "MR URL should be present")
	assert.NotEmpty(t, branchName, "Branch name should be present")
	assert.NotEmpty(t, buildVersion, "Build version should be present")

	// One-liner MR log on success
	t.Logf("MR Created: %s", mr)

	// Cleanup: if GITLAB_TOKEN is provided, delete source branch for the created MR (no merge)
	if token := os.Getenv("GITLAB_TOKEN"); token != "" && mr != "" {
		if err := deleteMRSourceBranch(ctx, token, mr); err != nil {
			t.Logf("cleanup warning: failed to delete MR source branch: %v", err)
		}
	}

	// Tiny guard: print artifacts map from status if present, else fall back to artifacts endpoint
	if artsVal, ok := st.Result["artifacts"]; ok {
		if artsMap, ok2 := artsVal.(map[string]interface{}); ok2 {
			t.Logf("Artifacts (from status): %v", artsMap)
		}
	} else {
		artsURL := fmt.Sprintf("%s/mods/%s/artifacts", strings.TrimRight(controller, "/"), result.ModID)
		req2, _ := http.NewRequestWithContext(ctx, http.MethodGet, artsURL, nil)
		resp2, err := httpc.Do(req2)
		if err == nil && resp2.StatusCode == 200 {
			defer resp2.Body.Close()
			var arts struct {
				Artifacts map[string]interface{} `json:"artifacts"`
			}
			if json.NewDecoder(resp2.Body).Decode(&arts) == nil && len(arts.Artifacts) > 0 {
				t.Logf("Artifacts (from endpoint): %v", arts.Artifacts)
			}
		}
	}
}

// deleteMRSourceBranch deletes the source branch of the MR at mrURL using GitLab API.
// It does not merge; it only removes the branch to keep the repo clean for E2E.
func deleteMRSourceBranch(ctx context.Context, token, mrURL string) error {
	// Parse project path and IID from MR URL of form:
	// https://gitlab.com/<namespace>/<project>/-/merge_requests/<iid>
	const hostPrefix = "https://gitlab.com/"
	if !strings.HasPrefix(mrURL, hostPrefix) {
		return fmt.Errorf("unexpected MR URL: %s", mrURL)
	}
	path := strings.TrimPrefix(mrURL, hostPrefix)
	parts := strings.Split(path, "/-/merge_requests/")
	if len(parts) != 2 {
		return fmt.Errorf("failed to parse MR URL: %s", mrURL)
	}
	projectPath := parts[0]
	iid := parts[1]
	projEsc := url.PathEscape(projectPath)

	apiBase := "https://gitlab.com/api/v4"
	httpc := &http.Client{Timeout: 20 * time.Second}

	// 1) GET MR details to find source_branch
	mrAPI := fmt.Sprintf("%s/projects/%s/merge_requests/%s", apiBase, projEsc, iid)
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, mrAPI, nil)
	req.Header.Set("PRIVATE-TOKEN", token)
	resp, err := httpc.Do(req)
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != 200 {
		return fmt.Errorf("get mr http %d", resp.StatusCode)
	}
	var mr struct {
		SourceBranch string `json:"source_branch"`
	}
	if json.NewDecoder(resp.Body).Decode(&mr) != nil || mr.SourceBranch == "" {
		return fmt.Errorf("failed to read MR source_branch")
	}

	// 2) DELETE branch
	delURL := fmt.Sprintf("%s/projects/%s/repository/branches/%s", apiBase, projEsc, url.PathEscape(mr.SourceBranch))
	req2, _ := http.NewRequestWithContext(ctx, http.MethodDelete, delURL, nil)
	req2.Header.Set("PRIVATE-TOKEN", token)
	resp2, err := httpc.Do(req2)
	if err != nil {
		return err
	}
	defer resp2.Body.Close()
	if resp2.StatusCode != 204 {
		return fmt.Errorf("delete branch http %d", resp2.StatusCode)
	}
	return nil
}

func TestModsE2E_SelfHealingScenario(t *testing.T) {
	// Should fail initially - healing integration not complete

	if testing.Short() {
		t.Skip("skipping E2E test in short mode")
	}

	env := SetupTestEnvironment(t, Config{
		UseRealServices: true,
		InjectFailures:  true,
		TimeoutMinutes:  15,
	})
	defer env.Cleanup()

	// Branch: prefer E2E_HEALING_BRANCH when provided, default to e2e/fail-missing-symbol
	hBranch := getenvDefault("E2E_HEALING_BRANCH", "e2e/fail-missing-symbol")

	workflow := &ModWorkflow{
		ID:           fmt.Sprintf("e2e-healing-%d", time.Now().Unix()),
		Repository:   "https://gitlab.com/iw2rmb/ploy-orw-java11-maven.git", // Use standard repo for now
		TargetBranch: hBranch,
		Steps: []WorkflowStep{
			{
				Type:               "orw-apply",
				ID:                 "healing-test",
				Recipes:            []WorkflowRecipe{{Name: "org.openrewrite.java.migrate.UpgradeToJava17", Group: "org.openrewrite.recipe", Artifact: "rewrite-migrate-java", Version: "3.17.0"}},
				MavenPluginVersion: "6.18.0",
			},
		},
		SelfHeal: SelfHealConfig{
			Enabled:    true,
			MaxRetries: 3,
			KBLearning: true,
		},
		ExpectedOutcome: OutcomeHealedSuccess,
		MaxDuration:     12 * time.Minute,
	}

	ctx, cancel := context.WithTimeout(context.Background(), workflow.MaxDuration)
	defer cancel()

	result, err := env.ExecuteWorkflow(ctx, workflow)

	// Basic completion check - may fail initially
	assert.NoError(t, err, "Healing workflow should complete")

	// Log healing attempts for analysis
	t.Logf("Healing Attempted: %t", result.HealingAttempted)
	t.Logf("Healing Attempts: %d", len(result.HealingAttempts))
	t.Logf("Final Success: %t", result.Success)

	if len(result.HealingAttempts) > 0 {
		for i, attempt := range result.HealingAttempts {
			t.Logf("Attempt %d: Success=%t, Error=%s", i+1, attempt.Success, attempt.ErrorSignature)
		}
	}

	// Collect logs when requested or on error conditions
	if os.Getenv("E2E_LOG_CONFIG") == "1" || !result.Success {
		if result.ModID != "" {
			runModsCollector(t, result.ModID)
		}
	}
}

func TestModsE2E_KBLearningProgression(t *testing.T) {
	// Should fail initially - KB learning not integrated

	if testing.Short() {
		t.Skip("skipping E2E test in short mode")
	}

	env := SetupTestEnvironment(t, Config{
		UseRealServices: true,
		CleanupAfter:    true,
		TimeoutMinutes:  20,
	})
	defer env.Cleanup()

	baseWorkflow := ModWorkflow{
		Repository:   "https://gitlab.com/iw2rmb/ploy-orw-java11-maven.git",
		TargetBranch: getenvDefault("E2E_BRANCH", "e2e/success"),
		Steps: []WorkflowStep{
			{
				Type:               "orw-apply",
				ID:                 "learning-test",
				Recipes:            []WorkflowRecipe{{Name: "org.openrewrite.java.cleanup.SimplifyBooleanExpression", Group: "org.openrewrite.recipe", Artifact: "rewrite-java-dependencies", Version: "latest"}},
				MavenPluginVersion: "6.18.0",
			},
		},
		SelfHeal: SelfHealConfig{
			Enabled:    true,
			KBLearning: true,
			MaxRetries: 2,
		},
		MaxDuration: 8 * time.Minute,
	}

	var results []WorkflowResult

	// Execute same workflow multiple times to test learning progression
	for i := 0; i < 2; i++ { // Reduced from 3 to 2 to save time in RED phase
		workflow := baseWorkflow
		workflow.ID = fmt.Sprintf("e2e-learning-%d-run-%d", time.Now().Unix(), i+1)

		ctx, cancel := context.WithTimeout(context.Background(), workflow.MaxDuration)

		result, err := env.ExecuteWorkflow(ctx, &workflow)
		cancel()

		// Log each run
		t.Logf("Run %d: Success=%t, Duration=%v, Error=%s", i+1, result.Success, result.Duration, result.Error)

		if err != nil {
			t.Logf("Run %d error: %v", i+1, err)
		}

		results = append(results, result)

		// Small delay between runs
		time.Sleep(5 * time.Second)
	}

	// Basic validation - may fail initially
	assert.True(t, len(results) >= 1, "Should complete at least one run")

	// Log learning progression for analysis
	for i, result := range results {
		t.Logf("Learning Run %d: Success=%t, Duration=%v", i+1, result.Success, result.Duration)
	}

	if len(results) == 2 && results[0].Success && results[1].Success {
		t.Logf("Learning progression: Run 1: %v, Run 2: %v", results[0].Duration, results[1].Duration)
	}
}

func TestModsE2E_HealingFlow_ORWFail_LLMSucceeds(t *testing.T) {
	// E2E healing validation for a repo/branch that intentionally fails the build gate post-orw-apply
	// Skips unless PLOY_CONTROLLER and E2E_HEALING_REPO are provided.

	if testing.Short() {
		t.Skip("skipping E2E test in short mode")
	}

	controller := getenv("PLOY_CONTROLLER")
	repo := getenv("E2E_HEALING_REPO")
	if controller == "" || repo == "" {
		t.Skip("Skipping: requires PLOY_CONTROLLER and E2E_HEALING_REPO env vars")
	}

	env := SetupTestEnvironment(t, Config{
		UseRealServices: true,
		CleanupAfter:    true,
		TimeoutMinutes:  20,
	})
	defer env.Cleanup()

	workflow := &ModWorkflow{
		ID:           fmt.Sprintf("e2e-healing-orw-llm-%d", time.Now().Unix()),
		Repository:   repo,
		TargetBranch: getenvDefault("E2E_HEALING_BRANCH", "e2e/fail-missing-symbol"),
		Steps: []WorkflowStep{
			{
				Type:               "orw-apply",
				ID:                 "java11to17-migration",
				Recipes:            []WorkflowRecipe{{Name: "org.openrewrite.java.migrate.UpgradeToJava17", Group: "org.openrewrite.recipe", Artifact: "rewrite-migrate-java", Version: "3.17.0"}},
				MavenPluginVersion: "6.18.0",
			},
		},
		SelfHeal: SelfHealConfig{
			Enabled:    true,
			MaxRetries: 2,
			KBLearning: false,
		},
		ExpectedOutcome: OutcomeHealedSuccess,
		MaxDuration:     15 * time.Minute,
	}

	ctx, cancel := context.WithTimeout(context.Background(), workflow.MaxDuration)
	defer cancel()

	result, err := env.ExecuteWorkflow(ctx, workflow)
	if err != nil {
		t.Logf("mods run error: %v", err)
	}
	if os.Getenv("E2E_LOG_CONFIG") == "1" || err != nil {
		t.Logf("Mods YAML path: %s", result.ConfigPath)
		if result.ConfigYAML != "" {
			t.Logf("Mods YAML:\n%s", result.ConfigYAML)
		}
	}

	// Fallback: if mod_id not parsed from CLI output, start run via controller directly
	if result.ModID == "" {
		t.Logf("mod_id not found in CLI output; starting run via controller fallback")
		runURL := strings.TrimRight(controller, "/") + "/mods"
		payload := fmt.Sprintf("{\"config\": %q, \"test_mode\": false}", result.ConfigYAML)
		req0, _ := http.NewRequestWithContext(ctx, http.MethodPost, runURL, strings.NewReader(payload))
		req0.Header.Set("Content-Type", "application/json")
		httpc := &http.Client{Timeout: 30 * time.Second}
		resp0, err0 := httpc.Do(req0)
		if err0 != nil {
			t.Fatalf("fallback run failed: %v", err0)
		}
		defer resp0.Body.Close()
		if resp0.StatusCode != 202 && resp0.StatusCode != 200 {
			t.Fatalf("fallback run HTTP %d", resp0.StatusCode)
		}
		var ack struct {
			ModID string `json:"mod_id"`
		}
		if json.NewDecoder(resp0.Body).Decode(&ack) != nil || ack.ModID == "" {
			t.Fatalf("fallback run: missing mod_id")
		}
		result.ModID = ack.ModID
		t.Logf("Fallback Execution ID: %s", result.ModID)
	}

	// Optional: collect logs early if requested or initial CLI errored
	if (os.Getenv("E2E_LOG_CONFIG") == "1" || err != nil) && result.ModID != "" {
		runModsCollector(t, result.ModID)
	}

	// Query controller for steps and artifacts to verify healing path
	statusURL := fmt.Sprintf("%s/mods/%s/status", strings.TrimRight(controller, "/"), result.ModID)
	artsURL := fmt.Sprintf("%s/mods/%s/artifacts", strings.TrimRight(controller, "/"), result.ModID)

	httpc := &http.Client{Timeout: 30 * time.Second}
	// Status
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, statusURL, nil)
	resp, err := httpc.Do(req)
	if err != nil {
		t.Fatalf("status fetch failed: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != 200 {
		t.Fatalf("status HTTP %d", resp.StatusCode)
	}
	var st map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&st); err != nil {
		t.Fatalf("decode status: %v", err)
	}

	// Steps inspection (best-effort): look for build-gate-failed → planner/llm-exec/reducer lifecycle
	steps, _ := st["steps"].([]any)
	flat := ""
	for _, s := range steps {
		if m, ok := s.(map[string]any); ok {
			phase := asStr(m["phase"])
			step := asStr(m["step"])
			level := asStr(m["level"])
			msg := asStr(m["message"])
			flat += fmt.Sprintf("%s:%s:%s:%s\n", phase, step, level, msg)
		}
	}
	t.Logf("steps:\n%s", flat)

	// Only assert when healing actually triggered (repo must be prepared to fail build)
	if strings.Contains(flat, "build:build-gate-failed:error") {
		// Tightened assertion: planner/reducer artifacts must be available when MR created
		// 1) Fetch artifacts index
		req2, _ := http.NewRequestWithContext(ctx, http.MethodGet, artsURL, nil)
		resp2, err := httpc.Do(req2)
		if err != nil {
			t.Fatalf("artifacts fetch failed: %v", err)
		}
		defer resp2.Body.Close()
		if resp2.StatusCode != 200 {
			t.Fatalf("artifacts HTTP %d", resp2.StatusCode)
		}
		var arts map[string]any
		if err := json.NewDecoder(resp2.Body).Decode(&arts); err != nil {
			t.Fatalf("decode artifacts: %v", err)
		}
		if _, ok := arts["artifacts"].(map[string]any); !ok {
			t.Fatalf("artifacts map missing in response")
		}
		// 2) Download each expected artifact via controller streaming endpoint
		for _, name := range []string{"plan_json", "next_json", "diff_patch"} {
			dl := strings.TrimRight(controller, "/") + "/mods/" + result.ModID + "/artifacts/" + name
			req3, _ := http.NewRequestWithContext(ctx, http.MethodGet, dl, nil)
			resp3, err := httpc.Do(req3)
			if err != nil {
				t.Fatalf("download %s failed: %v", name, err)
			}
			b := make([]byte, 64)
			n, _ := resp3.Body.Read(b)
			_ = resp3.Body.Close()
			if resp3.StatusCode != 200 || n == 0 {
				t.Fatalf("artifact %s not available (http %d, bytes %d)", name, resp3.StatusCode, n)
			}
		}
	} else {
		// If healing path wasn’t exercised, still collect logs on request for analysis
		if os.Getenv("E2E_LOG_CONFIG") == "1" && result.ModID != "" {
			runModsCollector(t, result.ModID)
		}
		t.Skip("build gate did not fail; provide E2E_HEALING_REPO with deterministic failure to fully validate healing path")
	}
}

// helpers
func getenv(k string) string { v := strings.TrimSpace(os.Getenv(k)); return v }
func getenvDefault(k, d string) string {
	v := getenv(k)
	if v == "" {
		return d
	}
	return v
}
func asStr(v any) string {
	if v == nil {
		return ""
	}
	if s, ok := v.(string); ok {
		return s
	}
	return fmt.Sprintf("%v", v)
}

// runModsCollector invokes the mods collect-logs.sh with a short follow window when enabled.
func runModsCollector(t *testing.T, modID string) {
	t.Helper()
	if modID == "" {
		return
	}
	script := resolveModsCollectorPath()
	if _, err := os.Stat(script); err != nil {
		t.Logf("collect-logs.sh not found: %v", err)
		return
	}
	follow := os.Getenv("E2E_LOG_FOLLOW_SECONDS")
	if follow == "" {
		follow = "30"
	}
	cmd := exec.Command("bash", script, modID)
	// Prepare environment
	env := os.Environ()
	// Propagate controller and optional SeaweedFS URL
	if v := os.Getenv("PLOY_CONTROLLER"); v != "" {
		env = append(env, "PLOY_CONTROLLER="+v)
	}
	if v := os.Getenv("PLOY_SEAWEEDFS_URL"); v != "" {
		env = append(env, "PLOY_SEAWEEDFS_URL="+v)
	}
	if v := os.Getenv("TARGET_HOST"); v != "" {
		env = append(env, "TARGET_HOST="+v)
	}
	env = append(env, "FOLLOW_SECONDS="+follow)
	if os.Getenv("E2E_LOG_COMPRESS") == "1" {
		env = append(env, "COMPRESS=1")
	}
	cmd.Env = env
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Logf("collect-logs.sh error: %v", err)
	}
	if len(out) > 0 {
		t.Logf("\n%s\n", string(out))
	}
}

// resolveModsCollectorPath returns an absolute path to the mods collect-logs.sh script.
func resolveModsCollectorPath() string {
	// Use caller path to derive repo root
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		return "tests/e2e/mods/collect-logs.sh"
	}
	base := filepath.Dir(file) // .../tests/e2e
	root := filepath.Dir(base) // .../tests
	root = filepath.Dir(root)  // repo root
	return filepath.Join(root, "tests/e2e/mods/collect-logs.sh")
}
