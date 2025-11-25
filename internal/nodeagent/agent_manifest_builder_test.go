package nodeagent

import (
	"strings"
	"testing"

	"github.com/iw2rmb/ploy/internal/workflow/contracts"

	types "github.com/iw2rmb/ploy/internal/domain/types"
)

// Manifest builder unit tests: buildManifestFromRequest scenarios.

// TestBuildManifestFromRequest verifies that a run manifest is correctly built from a StartRunRequest.
// This includes validation of required fields, defaults, and hydration configuration.
func TestBuildManifestFromRequest(t *testing.T) {
	t.Run("valid request with all fields", func(t *testing.T) {
		req := StartRunRequest{
			RunID:     types.RunID("run-123"),
			RepoURL:   types.RepoURL("https://github.com/example/repo.git"),
			BaseRef:   types.GitRef("main"),
			TargetRef: types.GitRef("feature-branch"),
			CommitSHA: types.CommitSHA("abc123"),
			Env: map[string]string{
				"FOO": "bar",
			},
		}

		manifest, err := buildManifestFromRequest(req, parseRunOptions(req.Options), 0)
		if err != nil {
			t.Fatalf("buildManifestFromRequest() error: %v", err)
		}

		if manifest.ID.String() != req.RunID.String() {
			t.Errorf("expected ID %q, got %q", req.RunID, manifest.ID.String())
		}
		if manifest.Image != "ubuntu:latest" {
			t.Errorf("expected default image ubuntu:latest, got %q", manifest.Image)
		}
		if manifest.WorkingDir != "/workspace" {
			t.Errorf("expected working dir /workspace, got %q", manifest.WorkingDir)
		}
		if len(manifest.Inputs) != 1 {
			t.Fatalf("expected 1 input, got %d", len(manifest.Inputs))
		}

		input := manifest.Inputs[0]
		if input.Name != "workspace" {
			t.Errorf("expected input name workspace, got %q", input.Name)
		}
		if input.MountPath != "/workspace" {
			t.Errorf("expected mount path /workspace, got %q", input.MountPath)
		}
		if input.Mode != contracts.StepInputModeReadWrite {
			t.Errorf("expected read-write mode, got %q", input.Mode)
		}
		if input.Hydration == nil {
			t.Fatal("expected hydration to be set")
		}
		if input.Hydration.Repo == nil {
			t.Fatal("expected repo to be set in hydration")
		}

		repo := input.Hydration.Repo
		if string(repo.URL) != req.RepoURL.String() {
			t.Errorf("expected repo URL %q, got %q", req.RepoURL, string(repo.URL))
		}
		if repo.BaseRef.String() != req.BaseRef.String() {
			t.Errorf("expected base ref %q, got %q", req.BaseRef, repo.BaseRef.String())
		}
		if repo.TargetRef.String() != req.TargetRef.String() {
			t.Errorf("expected target ref %q, got %q", req.TargetRef, repo.TargetRef.String())
		}
		if repo.Commit.String() != req.CommitSHA.String() {
			t.Errorf("expected commit %q, got %q", req.CommitSHA, repo.Commit.String())
		}

		if len(manifest.Env) != 1 {
			t.Errorf("expected 1 env var, got %d", len(manifest.Env))
		}
		if manifest.Env["FOO"] != "bar" {
			t.Errorf("expected env FOO=bar, got %q", manifest.Env["FOO"])
		}
	})

	t.Run("missing run_id", func(t *testing.T) {
		req := StartRunRequest{
			RepoURL: types.RepoURL("https://github.com/example/repo.git"),
		}

		_, err := buildManifestFromRequest(req, parseRunOptions(req.Options), 0)
		if err == nil {
			t.Fatal("expected error for missing run_id")
		}
		if !strings.Contains(err.Error(), "run_id required") {
			t.Errorf("expected error about run_id, got %v", err)
		}
	})

	t.Run("missing repo_url", func(t *testing.T) {
		req := StartRunRequest{
			RunID: types.RunID("run-123"),
		}

		_, err := buildManifestFromRequest(req, parseRunOptions(req.Options), 0)
		if err == nil {
			t.Fatal("expected error for missing repo_url")
		}
		if !strings.Contains(err.Error(), "repo_url required") {
			t.Errorf("expected error about repo_url, got %v", err)
		}
	})

	t.Run("defaults target_ref from base_ref", func(t *testing.T) {
		req := StartRunRequest{
			RunID:   types.RunID("run-123"),
			RepoURL: types.RepoURL("https://github.com/example/repo.git"),
			BaseRef: types.GitRef("main"),
		}

		manifest, err := buildManifestFromRequest(req, parseRunOptions(req.Options), 0)
		if err != nil {
			t.Fatalf("buildManifestFromRequest() error: %v", err)
		}

		if manifest.Inputs[0].Hydration.Repo.TargetRef.String() != "main" {
			t.Errorf("expected target_ref to default to main, got %q", manifest.Inputs[0].Hydration.Repo.TargetRef.String())
		}
	})

	t.Run("validates manifest", func(t *testing.T) {
		req := StartRunRequest{
			RunID:     types.RunID("run-123"),
			RepoURL:   types.RepoURL("https://github.com/example/repo.git"),
			TargetRef: types.GitRef("main"),
		}

		manifest, err := buildManifestFromRequest(req, parseRunOptions(req.Options), 0)
		if err != nil {
			t.Fatalf("buildManifestFromRequest() error: %v", err)
		}

		if err := manifest.Validate(); err != nil {
			t.Errorf("manifest validation failed: %v", err)
		}
	})

	// Accept command as either []string or single string.
	t.Run("command option string maps to shell", func(t *testing.T) {
		req := StartRunRequest{
			RunID:   types.RunID("run-123"),
			RepoURL: types.RepoURL("https://github.com/example/repo.git"),
			Options: map[string]any{
				"command": "echo hi",
			},
		}

		manifest, err := buildManifestFromRequest(req, parseRunOptions(req.Options), 0)
		if err != nil {
			t.Fatalf("buildManifestFromRequest() error: %v", err)
		}
		want := []string{"/bin/sh", "-c", "echo hi"}
		if len(manifest.Command) != len(want) {
			t.Fatalf("command len=%d, want %d", len(manifest.Command), len(want))
		}
		for i := range want {
			if manifest.Command[i] != want[i] {
				t.Fatalf("command[%d]=%q, want %q", i, manifest.Command[i], want[i])
			}
		}
	})

	// New behavior: only inject placeholder command when using default image.
	// If a custom image is provided and no command is set, leave command empty
	// so the image's own CMD/ENTRYPOINT drives execution.
	t.Run("no command injected when custom image provided", func(t *testing.T) {
		req := StartRunRequest{
			RunID:   types.RunID("run-123"),
			RepoURL: types.RepoURL("https://github.com/example/repo.git"),
			Options: map[string]any{
				"image": "docker.io/example/mods-openrewrite:latest",
			},
		}
		manifest, err := buildManifestFromRequest(req, parseRunOptions(req.Options), 0)
		if err != nil {
			t.Fatalf("buildManifestFromRequest() error: %v", err)
		}
		if got, want := manifest.Image, "docker.io/example/mods-openrewrite:latest"; got != want {
			t.Fatalf("image=%q, want %q", got, want)
		}
		if len(manifest.Command) != 0 {
			t.Fatalf("expected no command to be injected for custom image, got len=%d", len(manifest.Command))
		}
	})

	t.Run("placeholder command injected only for default ubuntu image", func(t *testing.T) {
		req := StartRunRequest{
			RunID:   types.RunID("run-456"),
			RepoURL: types.RepoURL("https://github.com/example/repo.git"),
		}
		manifest, err := buildManifestFromRequest(req, parseRunOptions(req.Options), 0)
		if err != nil {
			t.Fatalf("buildManifestFromRequest() error: %v", err)
		}
		want := []string{"/bin/sh", "-c", "echo 'Build gate placeholder'"}
		if len(manifest.Command) != len(want) {
			t.Fatalf("command len=%d, want %d", len(manifest.Command), len(want))
		}
		for i := range want {
			if manifest.Command[i] != want[i] {
				t.Fatalf("command[%d]=%q, want %q", i, manifest.Command[i], want[i])
			}
		}
	})

	t.Run("gitlab options are extracted and stored in manifest", func(t *testing.T) {
		req := StartRunRequest{
			RunID:   types.RunID("run-789"),
			RepoURL: types.RepoURL("https://github.com/example/repo.git"),
			Options: map[string]any{
				"gitlab_pat":       "glpat-secret-token",
				"gitlab_domain":    "gitlab.example.com",
				"mr_on_success":    true,
				"mr_on_fail":       false,
				"retain_container": true,
			},
		}
		manifest, err := buildManifestFromRequest(req, parseRunOptions(req.Options), 0)
		if err != nil {
			t.Fatalf("buildManifestFromRequest() error: %v", err)
		}

		// Verify GitLab options are stored in manifest.Options.
		if manifest.Options == nil {
			t.Fatal("expected Options to be set")
		}
		if pat, ok := manifest.Options["gitlab_pat"].(string); !ok || pat != "glpat-secret-token" {
			t.Errorf("expected gitlab_pat=glpat-secret-token, got %v", manifest.Options["gitlab_pat"])
		}
		if domain, ok := manifest.Options["gitlab_domain"].(string); !ok || domain != "gitlab.example.com" {
			t.Errorf("expected gitlab_domain=gitlab.example.com, got %v", manifest.Options["gitlab_domain"])
		}
		if mrSuccess, ok := manifest.Options["mr_on_success"].(bool); !ok || !mrSuccess {
			t.Errorf("expected mr_on_success=true, got %v", manifest.Options["mr_on_success"])
		}
		if mrFail, ok := manifest.Options["mr_on_fail"].(bool); !ok || mrFail {
			t.Errorf("expected mr_on_fail=false, got %v", manifest.Options["mr_on_fail"])
		}
	})

	t.Run("gitlab options are trimmed and only included when non-empty", func(t *testing.T) {
		req := StartRunRequest{
			RunID:   types.RunID("run-890"),
			RepoURL: types.RepoURL("https://github.com/example/repo.git"),
			Options: map[string]any{
				"gitlab_pat":    "  trimmed-token  ",
				"gitlab_domain": "",
				"mr_on_success": true,
			},
		}
		manifest, err := buildManifestFromRequest(req, parseRunOptions(req.Options), 0)
		if err != nil {
			t.Fatalf("buildManifestFromRequest() error: %v", err)
		}

		if pat, ok := manifest.Options["gitlab_pat"].(string); !ok || pat != "trimmed-token" {
			t.Errorf("expected trimmed gitlab_pat=trimmed-token, got %v", manifest.Options["gitlab_pat"])
		}
		if _, ok := manifest.Options["gitlab_domain"]; ok {
			t.Errorf("expected gitlab_domain to be omitted when empty")
		}
	})

	// Multi-step execution tests: verify step-by-step manifest building.
	t.Run("multi-step run builds manifest for each step from mods array", func(t *testing.T) {
		req := StartRunRequest{
			RunID:   types.RunID("run-multi-123"),
			RepoURL: types.RepoURL("https://github.com/example/repo.git"),
			BaseRef: types.GitRef("main"),
			Env:     map[string]string{"BASE_VAR": "base_value"},
			Options: map[string]any{
				"mods": []any{
					map[string]any{
						"image":   "mods-orw:latest",
						"command": []any{"--apply", "--dir", "/workspace"},
						"env":     map[string]any{"STEP_VAR": "step0"},
					},
					map[string]any{
						"image":   "mods-fmt:latest",
						"command": "fmt --check",
						"env":     map[string]any{"STEP_VAR": "step1"},
					},
				},
			},
		}

		typedOpts := parseRunOptions(req.Options)
		if len(typedOpts.Steps) != 2 {
			t.Fatalf("expected 2 steps, got %d", len(typedOpts.Steps))
		}

		// Build manifest for step 0.
		manifest0, err := buildManifestFromRequest(req, typedOpts, 0)
		if err != nil {
			t.Fatalf("buildManifestFromRequest(step=0) error: %v", err)
		}
		if manifest0.Image != "mods-orw:latest" {
			t.Errorf("step 0: expected image mods-orw:latest, got %q", manifest0.Image)
		}
		wantCmd0 := []string{"--apply", "--dir", "/workspace"}
		if len(manifest0.Command) != len(wantCmd0) {
			t.Errorf("step 0: command len=%d, want %d", len(manifest0.Command), len(wantCmd0))
		}
		for i := range wantCmd0 {
			if manifest0.Command[i] != wantCmd0[i] {
				t.Errorf("step 0: command[%d]=%q, want %q", i, manifest0.Command[i], wantCmd0[i])
			}
		}
		if manifest0.Env["BASE_VAR"] != "base_value" {
			t.Errorf("step 0: expected BASE_VAR=base_value, got %q", manifest0.Env["BASE_VAR"])
		}
		if manifest0.Env["STEP_VAR"] != "step0" {
			t.Errorf("step 0: expected STEP_VAR=step0, got %q", manifest0.Env["STEP_VAR"])
		}

		// Build manifest for step 1.
		manifest1, err := buildManifestFromRequest(req, typedOpts, 1)
		if err != nil {
			t.Fatalf("buildManifestFromRequest(step=1) error: %v", err)
		}
		if manifest1.Image != "mods-fmt:latest" {
			t.Errorf("step 1: expected image mods-fmt:latest, got %q", manifest1.Image)
		}
		wantCmd1 := []string{"/bin/sh", "-c", "fmt --check"}
		if len(manifest1.Command) != len(wantCmd1) {
			t.Errorf("step 1: command len=%d, want %d", len(manifest1.Command), len(wantCmd1))
		}
		for i := range wantCmd1 {
			if manifest1.Command[i] != wantCmd1[i] {
				t.Errorf("step 1: command[%d]=%q, want %q", i, manifest1.Command[i], wantCmd1[i])
			}
		}
		if manifest1.Env["BASE_VAR"] != "base_value" {
			t.Errorf("step 1: expected BASE_VAR=base_value, got %q", manifest1.Env["BASE_VAR"])
		}
		if manifest1.Env["STEP_VAR"] != "step1" {
			t.Errorf("step 1: expected STEP_VAR=step1, got %q", manifest1.Env["STEP_VAR"])
		}
	})

	t.Run("multi-step run: step env overrides base env", func(t *testing.T) {
		req := StartRunRequest{
			RunID:   types.RunID("run-multi-456"),
			RepoURL: types.RepoURL("https://github.com/example/repo.git"),
			Env:     map[string]string{"SHARED_VAR": "base", "UNIQUE_BASE": "base"},
			Options: map[string]any{
				"mods": []any{
					map[string]any{
						"image": "mods-step:latest",
						"env":   map[string]any{"SHARED_VAR": "step_override"},
					},
				},
			},
		}

		typedOpts := parseRunOptions(req.Options)
		manifest, err := buildManifestFromRequest(req, typedOpts, 0)
		if err != nil {
			t.Fatalf("buildManifestFromRequest() error: %v", err)
		}

		if manifest.Env["SHARED_VAR"] != "step_override" {
			t.Errorf("expected step env to override base: SHARED_VAR=step_override, got %q", manifest.Env["SHARED_VAR"])
		}
		if manifest.Env["UNIQUE_BASE"] != "base" {
			t.Errorf("expected base env to be preserved: UNIQUE_BASE=base, got %q", manifest.Env["UNIQUE_BASE"])
		}
	})

	t.Run("multi-step run: step index out of range returns error", func(t *testing.T) {
		req := StartRunRequest{
			RunID:   types.RunID("run-multi-789"),
			RepoURL: types.RepoURL("https://github.com/example/repo.git"),
			Options: map[string]any{
				"mods": []any{
					map[string]any{"image": "mods-step:latest"},
				},
			},
		}

		typedOpts := parseRunOptions(req.Options)
		_, err := buildManifestFromRequest(req, typedOpts, 1) // Index 1 out of range (only 1 step).
		if err == nil {
			t.Fatal("expected error for out of range step index")
		}
		if !strings.Contains(err.Error(), "out of range") {
			t.Errorf("expected out of range error, got %v", err)
		}
	})

	t.Run("single-step run: stepIndex is ignored when Steps is empty", func(t *testing.T) {
		req := StartRunRequest{
			RunID:   types.RunID("run-single-123"),
			RepoURL: types.RepoURL("https://github.com/example/repo.git"),
			Options: map[string]any{
				"image":   "single-mod:latest",
				"command": "run-single",
			},
		}

		typedOpts := parseRunOptions(req.Options)
		if len(typedOpts.Steps) != 0 {
			t.Fatalf("expected single-step run (len(Steps)=0), got %d", len(typedOpts.Steps))
		}

		// stepIndex is ignored for single-step runs (always uses Execution options).
		manifest, err := buildManifestFromRequest(req, typedOpts, 42) // Arbitrary stepIndex.
		if err != nil {
			t.Fatalf("buildManifestFromRequest() error: %v", err)
		}

		if manifest.Image != "single-mod:latest" {
			t.Errorf("expected image single-mod:latest, got %q", manifest.Image)
		}
		wantCmd := []string{"/bin/sh", "-c", "run-single"}
		if len(manifest.Command) != len(wantCmd) {
			t.Errorf("command len=%d, want %d", len(manifest.Command), len(wantCmd))
		}
	})
}
