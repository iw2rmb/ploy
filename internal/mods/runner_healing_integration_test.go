package mods

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/iw2rmb/ploy/internal/cli/common"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Test runner integration
func TestModRunnerWithHealing(t *testing.T) {
	t.Run("healing triggered on build failure", func(t *testing.T) {
		store := newSeaweedStub()

		oldDL := downloadToFileFn
		oldGet := getJSONFn
		oldVDP := validateDiffPathsFn
		oldVUD := validateUnifiedDiffFn
		oldAD := applyUnifiedDiffFn
		oldHasChanges := hasRepoChangesFn
		// Stub remote artifact calls to avoid network access
		downloadToFileFn = func(_ string, dest string) error {
			_ = os.MkdirAll(filepath.Dir(dest), 0755)
			diff := "--- a/pom.xml\n+++ b/pom.xml\n@@ -1 +1 @@\n-<project></project>\n+<project><modelVersion>4.0.0</modelVersion></project>\n"
			return os.WriteFile(dest, []byte(diff), 0644)
		}
		getJSONFn = func(string, string) ([]byte, int, error) { return nil, 404, nil }
		validateDiffPathsFn = func(string, []string) error { return nil }
		validateUnifiedDiffFn = func(context.Context, string, string) error { return nil }
		applyUnifiedDiffFn = func(context.Context, string, string) error { return nil }
		hasRepoChangesFn = func(string) (bool, error) { return true, nil }

		defer func() {
			downloadToFileFn = oldDL
			getJSONFn = oldGet
			validateDiffPathsFn = oldVDP
			validateUnifiedDiffFn = oldVUD
			applyUnifiedDiffFn = oldAD
			hasRepoChangesFn = oldHasChanges
		}()
		// MOD_ID used in artifact paths and events
		_ = os.Setenv("MOD_ID", "mod-test-exec")
		defer func() { _ = os.Unsetenv("MOD_ID") }()
		// Setup
		config := &ModConfig{
			ID:         "healing-test",
			TargetRepo: "https://github.com/test/repo",
			BaseRef:    "main",
			SelfHeal: &SelfHealConfig{
				MaxRetries: 2,
				Enabled:    true,
			},
			Steps: []ModStep{
				{
					Type:               "orw-apply",
					ID:                 "java-migration",
					Recipes:            []RecipeEntry{recipeEntry("org.openrewrite.java.migrate.UpgradeToJava17", "org.openrewrite.recipe", "rewrite-migrate-java", "3.17.0")},
					MavenPluginVersion: "6.18.0",
				},
			},
		}

		// Mocks
		mockGit := &MockGitOperations{}
		mockRecipe := &MockRecipeExecutor{}
		var check seqBuildChecker
		mockBuild := &check

		runner, err := NewModRunner(config, "/tmp/workspace")
		require.NoError(t, err)

		runner.SetGitOperations(mockGit)
		runner.SetRecipeExecutor(mockRecipe)
		runner.SetBuildChecker(mockBuild)
		runner.SetJobHelper(testJobHelper{})
		runner.SetHCLSubmitter(okHCLSubmitter{})
		// Non-nil jobSubmitter enables healing path; injected jobHelper handles planner/reducer
		runner.SetJobSubmitter(NoopJobSubmitter{})
		runner.SetHealingOrchestrator(okHealer{})
		runner.SetArtifactUploader(store)

		// Ensure a minimal build file exists to pass ORW guard
		_ = os.MkdirAll("/tmp/workspace/repo", 0755)
		_ = os.WriteFile("/tmp/workspace/repo/pom.xml", []byte("<project></project>"), 0644)

		// Execute
		ctx := context.Background()
		result, err := runner.Run(ctx)

		// Verify healing was attempted
		assert.NoError(t, err)
		assert.NotNil(t, result)
		assert.NotNil(t, result.HealingSummary)
		assert.True(t, result.HealingSummary.Enabled)
		assert.Greater(t, result.HealingSummary.AttemptsCount, 0)
	})
}

func TestModRunner_AttemptHealingIncludesBuilderLogsKey(t *testing.T) {
	config := &ModConfig{
		ID:         "builder-log-test",
		TargetRepo: "https://example.com/repo.git",
		BaseRef:    "main",
		Steps:      []ModStep{{ID: "s", Type: "recipe"}},
		SelfHeal: &SelfHealConfig{
			Enabled:    true,
			MaxRetries: 1,
		},
	}

	runner, err := NewModRunner(config, t.TempDir())
	require.NoError(t, err)
	runner.SetJobHelper(testJobHelper{})
	capHealer := &capturingHealer{}
	runner.SetHealingOrchestrator(capHealer)

	buildRes := &common.DeployResult{Success: false, DeploymentID: "mod-app-123", BuilderLogsKey: ""}

	_, healErr := runner.attemptHealing(context.Background(), t.TempDir(), "build failed", buildRes)
	require.NoError(t, healErr)
	require.NotEmpty(t, capHealer.branches, "expected healing branches to be constructed")
	inputs := capHealer.branches[0].Inputs
	key, ok := inputs["builder_logs_key"].(string)
	require.True(t, ok, "builder_logs_key not present in branch inputs")
	assert.Equal(t, "build-logs/mod-app-123.log", key)
}

func TestModRunner_HealingMRIncludesORWAndLLMDiffs(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git binary not available")
	}

	workspace := t.TempDir()
	origin := filepath.Join(workspace, "origin")
	bare := filepath.Join(workspace, "bare.git")
	setupGitRepository(t, origin)
	runCmd(t, workspace, "git", "clone", "--bare", origin, bare)

	config := &ModConfig{
		ID:           "healing-mr-diffs",
		TargetRepo:   bare,
		TargetBranch: "main",
		BaseRef:      "main",
		Lane:         "C",
		SelfHeal: &SelfHealConfig{
			Enabled:    true,
			MaxRetries: 1,
		},
		Steps: []ModStep{
			{
				Type:               string(StepTypeORWApply),
				ID:                 "java11to17-migration",
				Recipes:            []RecipeEntry{recipeEntry("org.openrewrite.java.migrate.UpgradeToJava17", "org.openrewrite.recipe", "rewrite-migrate-java", "3.17.0")},
				MavenPluginVersion: "6.18.0",
			},
		},
		MR: &MRConfigYAML{
			Labels: []string{"ploy", "tfl", "healing"},
		},
	}

	runner, err := NewModRunner(config, workspace)
	require.NoError(t, err)

	modID := "mod-healing-mr"
	seaweedBase := "http://seaweed.test"
	require.NoError(t, os.Setenv("MOD_ID", modID))
	require.NoError(t, os.Setenv("PLOY_SEAWEEDFS_URL", seaweedBase))
	defer func() {
		_ = os.Unsetenv("MOD_ID")
		_ = os.Unsetenv("PLOY_SEAWEEDFS_URL")
	}()

	orwDiff := `diff --git a/src/main/java/App.java b/src/main/java/App.java
index 0000000..0000000 100644
--- a/src/main/java/App.java
+++ b/src/main/java/App.java
@@ -1 +1,5 @@
-public class App { public static void main(String[] a){} }
+public class App {
+    public static void main(String[] a) {
+        Helper.greet();
+    }
+}
`
	llmDiff := `diff --git a/src/main/java/App.java b/src/main/java/App.java
index 1f59f67..1d42819 100644
--- a/src/main/java/App.java
+++ b/src/main/java/App.java
@@ -2,4 +2,10 @@ public class App {
     public static void main(String[] a) {
         Helper.greet();
     }
+
+    static class Helper {
+        static void greet() {
+            System.out.println("hi from helper");
+        }
+    }
 }
`

	store := newSeaweedStub()
	oldDL := downloadToFileFn
	oldGetJSON := getJSONFn
	oldHead := headURLFn

	downloadToFileFn = func(url, dest string) error {
		key := seaweedKey(seaweedBase, url)
		if key == "" {
			return fmt.Errorf("unsupported url: %s", url)
		}
		if data, ok := store.getFile(key); ok {
			if err := os.MkdirAll(filepath.Dir(dest), 0755); err != nil {
				return err
			}
			return os.WriteFile(dest, data, 0644)
		}
		if strings.Contains(key, "/branches/java11to17-migration/") {
			store.setFile(key, []byte(orwDiff))
			if err := os.MkdirAll(filepath.Dir(dest), 0755); err != nil {
				return err
			}
			return os.WriteFile(dest, []byte(orwDiff), 0644)
		}
		return fmt.Errorf("artifact not found: %s", key)
	}
	getJSONFn = func(base, key string) ([]byte, int, error) {
		if base != seaweedBase {
			return nil, 0, fmt.Errorf("unexpected base: %s", base)
		}
		if data, ok := store.getJSON(key); ok {
			return data, 200, nil
		}
		return nil, 404, nil
	}
	headURLFn = func(url string) bool {
		key := seaweedKey(seaweedBase, url)
		if key == "" {
			return false
		}
		_, ok := store.getFile(key)
		return ok
	}

	defer func() {
		downloadToFileFn = oldDL
		getJSONFn = oldGetJSON
		headURLFn = oldHead
	}()

	oldValidate := validateJob
	oldSubmit := submitAndWaitTerminal
	validateJob = func(string) error { return nil }
	submitAndWaitTerminal = func(string, time.Duration) error { return nil }
	defer func() {
		validateJob = oldValidate
		submitAndWaitTerminal = oldSubmit
	}()

	runner.SetGitOperations(NewAPIGitOperations(workspace))
	runner.SetJobSubmitter(NoopJobSubmitter{})
	runner.SetHCLSubmitter(okHCLSubmitter{})
	runner.SetBuildChecker(&stagedBuildChecker{results: []bool{true, false, true}})
	runner.SetJobHelper(&healingJobHelper{})
	runner.SetHealingOrchestrator(&healingFanout{
		modID:      modID,
		seaweedURL: seaweedBase,
		diff:       []byte(llmDiff),
		store:      store,
	})
	runner.SetArtifactUploader(store)

	gitProvider := NewMockGitProvider()
	runner.SetGitProvider(gitProvider)

	repoPath := filepath.Join(workspace, "repo")
	ctx := context.Background()
	result, err := runner.Run(ctx)
	require.NoError(t, err)
	require.True(t, result.Success, "mods run should succeed")
	require.NotNil(t, result.HealingSummary)
	require.True(t, result.HealingSummary.FinalSuccess)
	require.NotEmpty(t, result.MRURL)
	require.True(t, gitProvider.MRCalled)

	appBytes, err := os.ReadFile(filepath.Join(repoPath, "src", "main", "java", "App.java"))
	require.NoError(t, err)
	appContent := string(appBytes)
	assert.Contains(t, appContent, "Helper.greet();")
	javaDirEntries, err := os.ReadDir(filepath.Join(repoPath, "src", "main", "java"))
	require.NoError(t, err)
	var names []string
	for _, entry := range javaDirEntries {
		names = append(names, entry.Name())
	}
	t.Logf("java dir entries: %v", names)
	patchPath := filepath.Join(workspace, "branch-apply", "out", "chain-s-healing.patch")
	if patchBytes, err := os.ReadFile(patchPath); err == nil {
		t.Logf("healing patch contents:\n%s", string(patchBytes))
	} else {
		t.Logf("failed to read healing patch at %s: %v", patchPath, err)
	}
	assert.Contains(t, appContent, "static class Helper")

	logCmd := exec.Command("git", "log", "--pretty=%s")
	logCmd.Dir = repoPath
	logOut, err := logCmd.Output()
	require.NoError(t, err)
	logStr := string(logOut)
	assert.Contains(t, logStr, "Applied recipe transformations", "ORW commit missing")
	assert.Contains(t, logStr, "apply(healing): reducer patch", "healing commit missing")

	assert.Contains(t, gitProvider.MRConfig.RepoURL, bare)
	assert.NotEmpty(t, gitProvider.MRConfig.SourceBranch)
}

type stagedBuildChecker struct {
	results []bool
	idx     int
}

func (s *stagedBuildChecker) CheckBuild(ctx context.Context, cfg common.DeployConfig) (*common.DeployResult, error) {
	if s.idx >= len(s.results) {
		return &common.DeployResult{Success: true, Message: "build ok", Version: "v-default"}, nil
	}
	success := s.results[s.idx]
	s.idx++
	if success {
		return &common.DeployResult{Success: true, Message: "build ok", Version: fmt.Sprintf("v-%d", s.idx)}, nil
	}
	return &common.DeployResult{Success: false, Message: "compilation failed: undefined symbol Helper"}, nil
}

type healingJobHelper struct{}

func (healingJobHelper) SubmitPlannerJob(ctx context.Context, cfg *ModConfig, buildError string, workspace string) (*PlanResult, error) {
	return &PlanResult{PlanID: "plan-123", Options: []map[string]interface{}{{"id": "healing-branch", "type": string(StepTypeLLMExec)}}}, nil
}

func (healingJobHelper) SubmitReducerJob(ctx context.Context, planID string, results []BranchResult, winner *BranchResult, workspace string) (*NextAction, error) {
	branchID := "healing-branch"
	if winner != nil && winner.ID != "" {
		branchID = winner.ID
	}
	return &NextAction{Action: "apply", StepID: branchID}, nil
}

type healingFanout struct {
	modID      string
	seaweedURL string
	diff       []byte
	store      *seaweedStub
}

func (f *healingFanout) RunFanout(ctx context.Context, runCtx interface{}, branches []BranchSpec, maxParallel int) (BranchResult, []BranchResult, error) {
	if len(branches) == 0 {
		return BranchResult{}, nil, fmt.Errorf("no branches provided")
	}
	branch := branches[0]
	stepID := "s-healing"
	key := computeBranchDiffKey(f.modID, branch.ID, stepID)
	diffBytes := f.diff
	if len(diffBytes) == 0 || diffBytes[len(diffBytes)-1] != '\n' {
		diffBytes = append(diffBytes, '\n')
	}
	f.store.setFile(key, diffBytes)
	if err := writeBranchChainStepMeta(context.Background(), NewHTTPArtifactUploader(), f.seaweedURL, f.modID, branch.ID, stepID, key); err != nil {
		return BranchResult{}, nil, err
	}
	now := time.Now()
	res := BranchResult{ID: branch.ID, Status: "completed", JobID: stepID, StartedAt: now, FinishedAt: now}
	return res, []BranchResult{res}, nil
}

type seaweedStub struct {
	mu    sync.Mutex
	files map[string][]byte
	json  map[string][]byte
}

func newSeaweedStub() *seaweedStub {
	return &seaweedStub{files: make(map[string][]byte), json: make(map[string][]byte)}
}

func (s *seaweedStub) UploadFile(ctx context.Context, baseURL, key, srcPath, contentType string) error {
	_ = ctx
	_ = baseURL
	_ = contentType
	data, err := os.ReadFile(srcPath)
	if err != nil {
		return err
	}
	s.setFile(key, data)
	return nil
}

func (s *seaweedStub) UploadJSON(ctx context.Context, baseURL, key string, body []byte) error {
	_ = ctx
	_ = baseURL
	s.setJSON(key, body)
	return nil
}

func (s *seaweedStub) setFile(key string, data []byte) {
	s.mu.Lock()
	defer s.mu.Unlock()
	copyBuf := append([]byte(nil), data...)
	s.files[key] = copyBuf
}

func (s *seaweedStub) getFile(key string) ([]byte, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	data, ok := s.files[key]
	if !ok {
		return nil, false
	}
	return append([]byte(nil), data...), true
}

func (s *seaweedStub) setJSON(key string, data []byte) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.json[key] = append([]byte(nil), data...)
}

func (s *seaweedStub) getJSON(key string) ([]byte, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	data, ok := s.json[key]
	if !ok {
		return nil, false
	}
	return append([]byte(nil), data...), true
}

func seaweedKey(base, url string) string {
	trimmed := strings.TrimRight(base, "/") + "/artifacts/"
	if !strings.HasPrefix(url, trimmed) {
		return ""
	}
	return strings.TrimPrefix(url, trimmed)
}

func setupGitRepository(t *testing.T, path string) {
	t.Helper()
	runCmd(t, "", "git", "init", "-b", "main", path)
	runCmd(t, path, "git", "config", "user.email", "test@example.com")
	runCmd(t, path, "git", "config", "user.name", "Test User")
	if err := os.MkdirAll(filepath.Join(path, "src", "main", "java"), 0755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(path, "src", "main", "java", "App.java"), []byte("public class App { public static void main(String[] a){} }\n"), 0644); err != nil {
		t.Fatalf("write App.java: %v", err)
	}
	if err := os.WriteFile(filepath.Join(path, "pom.xml"), []byte("<project></project>\n"), 0644); err != nil {
		t.Fatalf("write pom.xml: %v", err)
	}
	runCmd(t, path, "git", "add", ".")
	runCmd(t, path, "git", "commit", "-m", "chore: initial")
}

func runCmd(t *testing.T, dir string, name string, args ...string) {
	t.Helper()
	cmd := exec.Command(name, args...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("%s %v failed: %v: %s", name, args, err, string(out))
	}
}
