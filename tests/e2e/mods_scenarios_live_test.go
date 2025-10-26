//go:build e2e

package e2e

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"testing"
	"time"
)

// TestModsScenariosLiveGrid executes configured Mods scenarios against live Grid when configuration is present.
func TestModsScenariosLiveGrid(t *testing.T) {
	cfg := LoadConfig()
	if cfg.SkipReason != "" {
		t.Skipf("live grid config missing: %s", cfg.SkipReason)
	}

	ids := liveGridScenarioIDs()
	if len(ids) == 0 {
		t.Skip("no live grid scenarios requested")
	}

	for _, id := range ids {
		scenario := mustScenario(t, id)
		t.Run(id, func(t *testing.T) {
			if err := runScenarioLive(t, cfg, scenario); err != nil {
				t.Fatalf("mods live grid scenario failed: %v", err)
			}
		})
	}
}

func TestLiveGridScenarioIDsDefault(t *testing.T) {
	if ids := liveGridScenarioIDs(); !reflect.DeepEqual(ids, []string{"simple-openrewrite"}) {
		t.Fatalf("unexpected live grid scenario ids: %v", ids)
	}
}

func TestLiveGridScenarioIDsFromEnv(t *testing.T) {
	t.Setenv("PLOY_E2E_LIVE_SCENARIOS", "parallel-healing-options, buildgate-self-heal ,simple-openrewrite")
	want := []string{"buildgate-self-heal", "parallel-healing-options", "simple-openrewrite"}
	got := liveGridScenarioIDs()
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("unexpected live grid scenario ids: got %v want %v", got, want)
	}
}

// runScenarioLive shells out to the CLI to execute a Mods scenario end-to-end on Grid.
func runScenarioLive(t *testing.T, cfg Config, scenario Scenario) error {
	t.Helper()
	binary := buildPloyBinary(t)
	repoURL := strings.TrimSpace(cfg.RepoOverride)
	if repoURL == "" {
		repoURL = strings.TrimSpace(scenario.RepoURL)
	}
	if repoURL == "" {
		return fmt.Errorf("scenario %s missing repo url", scenario.ID)
	}
	baseRef := strings.TrimSpace(scenario.BaseRef)
	if baseRef == "" {
		baseRef = "main"
	}
	targetRef := cfg.TargetRef(scenario.ID)
	workspaceHint := "mods/java"

	args := []string{
		"mod", "run",
		"--tenant", cfg.Tenant,
		"--ticket", cfg.TicketID(scenario.ID),
		"--repo-url", repoURL,
		"--repo-base-ref", baseRef,
		"--repo-target-ref", targetRef,
		"--repo-workspace-hint", workspaceHint,
	}

	rootDir := projectRootDir(t)

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Minute)
	defer cancel()

	cmd := exec.CommandContext(ctx, binary, args...)
	cmd.Dir = rootDir

	envVars := map[string]string{
		"GRID_BEACON_API_KEY":    cfg.BeaconAPIKey,
		"GRID_ID":                cfg.GridID,
		"PLOY_E2E_TENANT":        cfg.Tenant,
		"PLOY_E2E_TICKET_PREFIX": cfg.TicketPrefix,
		"PLOY_E2E_REPO_OVERRIDE": cfg.RepoOverride,
	}
	if cfg.BeaconURL != "" {
		envVars["GRID_BEACON_URL"] = cfg.BeaconURL
	}
	cmd.Env = ensureEnv(os.Environ(), envVars)

	var output bytes.Buffer
	cmd.Stdout = &output
	cmd.Stderr = &output

	if err := cmd.Run(); err != nil {
		if ctx.Err() == context.DeadlineExceeded {
			return fmt.Errorf("mods live grid scenario timed out: %w", err)
		}
		return fmt.Errorf("mods live grid scenario error: %w\n%s", err, output.String())
	}

	return nil
}

// buildPloyBinary compiles the CLI into a temporary binary for the live Grid scenario run.
func buildPloyBinary(t *testing.T) string {
	t.Helper()
	out := filepath.Join(t.TempDir(), "ploy-e2e")
	cmd := exec.Command("go", "build", "-o", out, "./cmd/ploy")
	cmd.Env = os.Environ()
	cmd.Dir = projectRootDir(t)
	if result, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("build ploy cli: %v\n%s", err, string(result))
	}
	return out
}

// projectRootDir resolves the repository root for invoking the CLI during tests.
func projectRootDir(t *testing.T) string {
	t.Helper()
	root, err := os.Getwd()
	if err != nil {
		t.Fatalf("resolve project root: %v", err)
	}
	for {
		if _, err := os.Stat(filepath.Join(root, "go.mod")); err == nil {
			return root
		}
		parent := filepath.Dir(root)
		if parent == root {
			t.Fatalf("unable to locate repository root starting from %s", root)
		}
		root = parent
	}
}

// ensureEnv merges override variables into the base environment without mutating the input slice.
func ensureEnv(base []string, overrides map[string]string) []string {
	result := make([]string, len(base))
	copy(result, base)
	for key, value := range overrides {
		if strings.TrimSpace(value) == "" {
			continue
		}
		result = upsertEnv(result, key, value)
	}
	return result
}

// upsertEnv inserts or replaces a single environment variable assignment.
func upsertEnv(env []string, key, value string) []string {
	needle := key + "="
	for idx, entry := range env {
		if strings.HasPrefix(entry, needle) {
			env[idx] = needle + value
			return env
		}
	}
	return append(env, needle+value)
}

// liveGridScenarioIDs resolves the list of scenario identifiers to execute against live Grid.
func liveGridScenarioIDs() []string {
	raw := strings.TrimSpace(os.Getenv("PLOY_E2E_LIVE_SCENARIOS"))
	if raw == "" {
		return []string{"simple-openrewrite"}
	}
	parts := strings.Split(raw, ",")
	seen := make(map[string]struct{}, len(parts))
	var ids []string
	for _, part := range parts {
		id := strings.TrimSpace(part)
		if id == "" {
			continue
		}
		if _, known := scenarioRegistry[id]; !known {
			continue
		}
		if _, exists := seen[id]; exists {
			continue
		}
		seen[id] = struct{}{}
		ids = append(ids, id)
	}
	if len(ids) == 0 {
		return []string{"simple-openrewrite"}
	}
	sort.Strings(ids)
	return ids
}
