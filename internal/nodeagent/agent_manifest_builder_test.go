package nodeagent

import (
	"slices"
	"strings"
	"testing"

	"github.com/iw2rmb/ploy/internal/workflow/contracts"
)

// Manifest builder unit tests: buildManifestFromRequest scenarios.

func TestBuildManifestFromRequest(t *testing.T) {
	t.Run("valid request with all fields", func(t *testing.T) {
		req := newStartRunRequest(
			withRunTargetRef("feature-branch"),
			withRunCommitSHA("abc123"),
			withRunEnv(map[string]string{"FOO": "bar"}),
		)

		manifest, err := buildManifestDefault(req)
		if err != nil {
			t.Fatalf("buildManifestDefault() error: %v", err)
		}

		if manifest.ID.String() != req.JobID.String() {
			t.Errorf("expected ID %q (JobID), got %q", req.JobID, manifest.ID.String())
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

	t.Run("validation errors", func(t *testing.T) {
		tests := []struct {
			name    string
			req     StartRunRequest
			wantErr string
		}{
			{
				name:    "missing run_id",
				req:     StartRunRequest{RepoURL: "https://github.com/example/repo.git", TypedOptions: RunOptions{}},
				wantErr: "run_id required",
			},
			{
				name:    "missing repo_url",
				req:     StartRunRequest{RunID: "run-123", JobID: "job-123", TypedOptions: RunOptions{}},
				wantErr: "repo_url required",
			},
		}
		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				_, err := buildManifestDefault(tt.req)
				if err == nil {
					t.Fatalf("expected error containing %q", tt.wantErr)
				}
				if !strings.Contains(err.Error(), tt.wantErr) {
					t.Errorf("error=%v, want substring %q", err, tt.wantErr)
				}
			})
		}
	})

	t.Run("defaults target_ref from base_ref", func(t *testing.T) {
		req := newStartRunRequest()
		manifest, err := buildManifestDefault(req)
		if err != nil {
			t.Fatalf("buildManifestDefault() error: %v", err)
		}
		if manifest.Inputs[0].Hydration.Repo.TargetRef.String() != "main" {
			t.Errorf("expected target_ref to default to main, got %q", manifest.Inputs[0].Hydration.Repo.TargetRef.String())
		}
	})

	t.Run("validates manifest", func(t *testing.T) {
		req := newStartRunRequest(withRunTargetRef("main"))
		manifest, err := buildManifestDefault(req)
		if err != nil {
			t.Fatalf("buildManifestDefault() error: %v", err)
		}
		if err := manifest.Validate(); err != nil {
			t.Errorf("manifest validation failed: %v", err)
		}
	})

	t.Run("command option string maps to shell", func(t *testing.T) {
		req := newStartRunRequest(withRunOptions(RunOptions{
			Execution: ModContainerSpec{Command: contracts.CommandSpec{Shell: "echo hi"}},
		}))
		manifest, err := buildManifestDefault(req)
		if err != nil {
			t.Fatalf("buildManifestDefault() error: %v", err)
		}
		if want := []string{"/bin/sh", "-c", "echo hi"}; !slices.Equal(manifest.Command, want) {
			t.Fatalf("command = %v, want %v", manifest.Command, want)
		}
	})

	t.Run("no command injected when custom image provided", func(t *testing.T) {
		req := newStartRunRequest(withRunOptions(RunOptions{
			Execution: ModContainerSpec{Image: contracts.JobImage{Universal: "docker.io/example/migs-openrewrite:latest"}},
		}))
		manifest, err := buildManifestDefault(req)
		if err != nil {
			t.Fatalf("buildManifestDefault() error: %v", err)
		}
		if got, want := manifest.Image, "docker.io/example/migs-openrewrite:latest"; got != want {
			t.Fatalf("image=%q, want %q", got, want)
		}
		if len(manifest.Command) != 0 {
			t.Fatalf("expected no command for custom image, got len=%d", len(manifest.Command))
		}
	})

	t.Run("placeholder command injected only for default ubuntu image", func(t *testing.T) {
		req := newStartRunRequest() // empty options -> default image
		manifest, err := buildManifestDefault(req)
		if err != nil {
			t.Fatalf("buildManifestDefault() error: %v", err)
		}
		if want := []string{"/bin/sh", "-c", "echo 'Build gate placeholder'"}; !slices.Equal(manifest.Command, want) {
			t.Fatalf("command = %v, want %v", manifest.Command, want)
		}
	})

	t.Run("gitlab options are extracted and stored in manifest", func(t *testing.T) {
		req := newStartRunRequest(withRunOptions(RunOptions{
			MRWiring: MRWiringOptions{
				GitLabPAT: "glpat-secret-token", GitLabDomain: "gitlab.example.com",
				MROnSuccess: true, MROnFail: false,
			},
			MRFlagsPresent: MRFlagsPresence{MROnSuccessSet: true, MROnFailSet: true},
		}))
		manifest, err := buildManifestDefault(req)
		if err != nil {
			t.Fatalf("buildManifestDefault() error: %v", err)
		}
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
		req := newStartRunRequest(withRunOptions(RunOptions{
			MRWiring:       MRWiringOptions{GitLabPAT: "  trimmed-token  ", GitLabDomain: "", MROnSuccess: true},
			MRFlagsPresent: MRFlagsPresence{MROnSuccessSet: true},
		}))
		manifest, err := buildManifestDefault(req)
		if err != nil {
			t.Fatalf("buildManifestDefault() error: %v", err)
		}
		if pat, ok := manifest.Options["gitlab_pat"].(string); !ok || pat != "trimmed-token" {
			t.Errorf("expected trimmed gitlab_pat=trimmed-token, got %v", manifest.Options["gitlab_pat"])
		}
		if _, ok := manifest.Options["gitlab_domain"]; ok {
			t.Errorf("expected gitlab_domain to be omitted when empty")
		}
	})

	t.Run("multi-step run builds manifest for each step from steps array", func(t *testing.T) {
		req := newStartRunRequest(
			withRunEnv(map[string]string{"BASE_VAR": "base_value"}),
			withRunOptions(RunOptions{
				Steps: []StepMod{
					{ModContainerSpec: ModContainerSpec{
						Image:   contracts.JobImage{Universal: "migs-orw:latest"},
						Command: contracts.CommandSpec{Exec: []string{"--apply", "--dir", "/workspace"}},
						Env:     map[string]string{"STEP_VAR": "step0"},
					}},
					{ModContainerSpec: ModContainerSpec{
						Image:   contracts.JobImage{Universal: "migs-fmt:latest"},
						Command: contracts.CommandSpec{Shell: "fmt --check"},
						Env:     map[string]string{"STEP_VAR": "step1"},
					}},
				},
			}),
		)

		// Step 0.
		m0, err := buildManifestAtStep(req, 0)
		if err != nil {
			t.Fatalf("step 0 error: %v", err)
		}
		if m0.Image != "migs-orw:latest" {
			t.Errorf("step 0: image=%q, want migs-orw:latest", m0.Image)
		}
		if want := []string{"--apply", "--dir", "/workspace"}; !slices.Equal(m0.Command, want) {
			t.Fatalf("step 0: command = %v, want %v", m0.Command, want)
		}
		if m0.Env["BASE_VAR"] != "base_value" {
			t.Errorf("step 0: BASE_VAR=%q, want base_value", m0.Env["BASE_VAR"])
		}
		if m0.Env["STEP_VAR"] != "step0" {
			t.Errorf("step 0: STEP_VAR=%q, want step0", m0.Env["STEP_VAR"])
		}

		// Step 1.
		m1, err := buildManifestAtStep(req, 1)
		if err != nil {
			t.Fatalf("step 1 error: %v", err)
		}
		if m1.Image != "migs-fmt:latest" {
			t.Errorf("step 1: image=%q, want migs-fmt:latest", m1.Image)
		}
		if want := []string{"/bin/sh", "-c", "fmt --check"}; !slices.Equal(m1.Command, want) {
			t.Fatalf("step 1: command = %v, want %v", m1.Command, want)
		}
		if m1.Env["BASE_VAR"] != "base_value" {
			t.Errorf("step 1: BASE_VAR=%q, want base_value", m1.Env["BASE_VAR"])
		}
		if m1.Env["STEP_VAR"] != "step1" {
			t.Errorf("step 1: STEP_VAR=%q, want step1", m1.Env["STEP_VAR"])
		}
	})

	t.Run("single-step amata uses amata command", func(t *testing.T) {
		req := newStartRunRequest(withRunOptions(RunOptions{
			Execution: ModContainerSpec{
				Image: contracts.JobImage{Universal: "migs-codex:latest"},
				Amata: &contracts.AmataRunSpec{
					Spec: "version: amata/v1\n",
					Set:  []contracts.AmataSetParam{{Param: "mode", Value: "step"}},
				},
			},
		}))
		manifest, err := buildManifestDefault(req)
		if err != nil {
			t.Fatalf("buildManifestDefault() error: %v", err)
		}
		if want := []string{"amata", "run", "/in/amata.yaml", "--set", "mode=step"}; !slices.Equal(manifest.Command, want) {
			t.Fatalf("command = %v, want %v", manifest.Command, want)
		}
	})

	t.Run("multi-step amata uses amata command for selected step", func(t *testing.T) {
		req := newStartRunRequest(withRunOptions(RunOptions{
			Steps: []StepMod{
				{ModContainerSpec: ModContainerSpec{
					Image: contracts.JobImage{Universal: "migs-orw:latest"}, Command: contracts.CommandSpec{Shell: "echo plain"},
				}},
				{ModContainerSpec: ModContainerSpec{
					Image: contracts.JobImage{Universal: "migs-codex:latest"},
					Amata: &contracts.AmataRunSpec{
						Spec: "version: amata/v1\n",
						Set:  []contracts.AmataSetParam{{Param: "model", Value: "gpt-5"}},
					},
				}},
			},
		}))
		manifest, err := buildManifestAtStep(req, 1)
		if err != nil {
			t.Fatalf("step 1 error: %v", err)
		}
		if want := []string{"amata", "run", "/in/amata.yaml", "--set", "model=gpt-5"}; !slices.Equal(manifest.Command, want) {
			t.Fatalf("command = %v, want %v", manifest.Command, want)
		}
	})

	t.Run("multi-step run: step env overrides base env", func(t *testing.T) {
		req := newStartRunRequest(
			withRunEnv(map[string]string{"SHARED_VAR": "base", "UNIQUE_BASE": "base"}),
			withRunOptions(RunOptions{
				Steps: []StepMod{{ModContainerSpec: ModContainerSpec{
					Image: contracts.JobImage{Universal: "migs-step:latest"},
					Env:   map[string]string{"SHARED_VAR": "step_override"},
				}}},
			}),
		)
		manifest, err := buildManifestAtStep(req, 0)
		if err != nil {
			t.Fatalf("buildManifestAtStep() error: %v", err)
		}
		if manifest.Env["SHARED_VAR"] != "step_override" {
			t.Errorf("expected step env override: SHARED_VAR=step_override, got %q", manifest.Env["SHARED_VAR"])
		}
		if manifest.Env["UNIQUE_BASE"] != "base" {
			t.Errorf("expected base env preserved: UNIQUE_BASE=base, got %q", manifest.Env["UNIQUE_BASE"])
		}
	})

	t.Run("multi-step run: step index out of range returns error", func(t *testing.T) {
		req := newStartRunRequest(withRunOptions(RunOptions{
			Steps: []StepMod{{ModContainerSpec: ModContainerSpec{Image: contracts.JobImage{Universal: "migs-step:latest"}}}},
		}))
		_, err := buildManifestAtStep(req, 1)
		if err == nil {
			t.Fatal("expected error for out of range step index")
		}
		if !strings.Contains(err.Error(), "out of range") {
			t.Errorf("expected out of range error, got %v", err)
		}
	})

	t.Run("single-step run: stepIndex is ignored when Steps is empty", func(t *testing.T) {
		req := newStartRunRequest(withRunOptions(RunOptions{
			Execution: ModContainerSpec{
				Image:   contracts.JobImage{Universal: "single-mig:latest"},
				Command: contracts.CommandSpec{Shell: "run-single"},
			},
		}))
		manifest, err := buildManifestAtStep(req, 42) // arbitrary stepIndex
		if err != nil {
			t.Fatalf("buildManifestAtStep() error: %v", err)
		}
		if manifest.Image != "single-mig:latest" {
			t.Errorf("expected image single-mig:latest, got %q", manifest.Image)
		}
		if want := []string{"/bin/sh", "-c", "run-single"}; !slices.Equal(manifest.Command, want) {
			t.Fatalf("command = %v, want %v", manifest.Command, want)
		}
	})
}

func TestManifestBuildWithGateRepoMeta(t *testing.T) {
	t.Parallel()

	t.Run("ref precedence", func(t *testing.T) {
		t.Parallel()
		tests := []struct {
			name      string
			commitSHA string
			targetRef string
			baseRef   string
			wantRef   string
		}{
			{"CommitSHA takes precedence", "abc123def456", "feature/gate-wiring", "main", "abc123def456"},
			{"TargetRef when no CommitSHA", "", "feature/gate-wiring", "main", "feature/gate-wiring"},
			{"BaseRef as fallback", "", "", "main", "main"},
			{"empty when no refs provided", "", "", "", ""},
		}
		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				t.Parallel()
				req := newStartRunRequest(
					withRunRepoURL("https://gitlab.com/iw2rmb/ploy-orw.git"),
					withRunBaseRef(tt.baseRef),
					withRunTargetRef(tt.targetRef),
					withRunCommitSHA(tt.commitSHA),
				)
				manifest, err := buildManifestDefault(req)
				if err != nil {
					t.Fatalf("buildManifestDefault() error: %v", err)
				}
				if manifest.Gate == nil {
					t.Fatal("expected Gate spec to be set")
				}
				if manifest.Gate.RepoURL.String() != req.RepoURL.String() {
					t.Errorf("Gate.RepoURL=%q, want %q", manifest.Gate.RepoURL, req.RepoURL.String())
				}
				if manifest.Gate.Ref.String() != tt.wantRef {
					t.Errorf("Gate.Ref=%q, want %q", manifest.Gate.Ref, tt.wantRef)
				}
			})
		}
	})

	t.Run("gate repo metadata trimmed of whitespace", func(t *testing.T) {
		t.Parallel()
		req := newStartRunRequest(
			withRunRepoURL("  https://gitlab.com/iw2rmb/ploy-orw.git  "),
			withRunCommitSHA("  abc123  "),
		)
		manifest, err := buildManifestDefault(req)
		if err != nil {
			t.Fatalf("buildManifestDefault() error: %v", err)
		}
		if manifest.Gate.RepoURL.String() != "https://gitlab.com/iw2rmb/ploy-orw.git" {
			t.Errorf("Gate.RepoURL=%q, want trimmed", manifest.Gate.RepoURL)
		}
		if manifest.Gate.Ref.String() != "abc123" {
			t.Errorf("Gate.Ref=%q, want trimmed", manifest.Gate.Ref)
		}
	})

	t.Run("gate image overrides threaded from build_gate.images", func(t *testing.T) {
		t.Parallel()
		req := newStartRunRequest(
			withRunRepoURL("https://gitlab.com/iw2rmb/ploy-orw.git"),
			withRunTargetRef("main"),
			withRunOptions(RunOptions{BuildGate: BuildGateOptions{
				Enabled: true,
				Images: []contracts.BuildGateImageRule{{
					Stack: contracts.StackExpectation{Language: "java", Tool: "maven", Release: "17"},
					Image: "maven:3-eclipse-temurin-17",
				}},
			}}),
		)
		manifest, err := buildManifestDefault(req)
		if err != nil {
			t.Fatalf("buildManifestDefault() error: %v", err)
		}
		if !manifest.Gate.Enabled {
			t.Error("expected Gate.Enabled=true")
		}
		if len(manifest.Gate.ImageOverrides) != 1 {
			t.Fatalf("len(Gate.ImageOverrides)=%d, want 1", len(manifest.Gate.ImageOverrides))
		}
		if got := manifest.Gate.ImageOverrides[0].Image; got != "maven:3-eclipse-temurin-17" {
			t.Errorf("Gate.ImageOverrides[0].Image=%q, want maven:3-eclipse-temurin-17", got)
		}
		if manifest.Gate.RepoURL.String() != req.RepoURL.String() {
			t.Errorf("Gate.RepoURL=%q, want %q", manifest.Gate.RepoURL, req.RepoURL.String())
		}
	})
}

func TestBuildGateManifestFromRequest_IgnoresStackAwareJobImages(t *testing.T) {
	t.Parallel()

	req := newStartRunRequest(withRunOptions(RunOptions{
		BuildGate: BuildGateOptions{Enabled: true},
		Steps: []StepMod{{ModContainerSpec: ModContainerSpec{
			Image: contracts.JobImage{ByStack: map[contracts.ModStack]string{
				contracts.ModStackJavaMaven:  "docker.io/example/orw-cli:latest",
				contracts.ModStackJavaGradle: "docker.io/example/orw-cli:latest",
			}},
		}}},
	}))

	manifest, err := buildGateManifestFromRequest(req, req.TypedOptions)
	if err != nil {
		t.Fatalf("buildGateManifestFromRequest() error: %v", err)
	}
	if manifest.Image != "ubuntu:latest" {
		t.Errorf("gate manifest image=%q, want ubuntu:latest", manifest.Image)
	}
	if manifest.Gate == nil {
		t.Fatal("expected Gate spec to be set")
	}
	if !manifest.Gate.Enabled {
		t.Error("expected Gate.Enabled=true")
	}
	if manifest.Gate.RepoURL.String() != req.RepoURL.String() {
		t.Errorf("Gate.RepoURL=%q, want %q", manifest.Gate.RepoURL, req.RepoURL.String())
	}
}
