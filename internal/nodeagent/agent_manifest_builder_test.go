package nodeagent

import (
	"slices"
	"strings"
	"testing"

	"github.com/iw2rmb/ploy/internal/workflow/contracts"
)

// Manifest builder unit tests: buildMigManifest scenarios.

func TestBuildManifestFromRequest(t *testing.T) {
	t.Run("valid request with all fields", func(t *testing.T) {
		req := newStartRunRequest(
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
		if repo.Commit.String() != req.CommitSHA.String() {
			t.Errorf("expected commit %q, got %q", req.CommitSHA, repo.Commit.String())
		}

		if len(manifest.Envs) != 1 {
			t.Errorf("expected 1 env var, got %d", len(manifest.Envs))
		}
		if manifest.Envs["FOO"] != "bar" {
			t.Errorf("expected env FOO=bar, got %q", manifest.Envs["FOO"])
		}
	})

	t.Run("keeps hydration and gate repo URLs clean", func(t *testing.T) {
		req := newStartRunRequest(
			withRunURL("https://gitlab.example.com/group/repo.git"),
		)

		manifest, err := buildManifestDefault(req)
		if err != nil {
			t.Fatalf("buildManifestDefault() error: %v", err)
		}
		if len(manifest.Inputs) != 1 || manifest.Inputs[0].Hydration == nil || manifest.Inputs[0].Hydration.Repo == nil {
			t.Fatalf("expected hydration repo to be set")
		}

		gotHydrationURL := manifest.Inputs[0].Hydration.Repo.URL.String()
		wantHydrationURL := "https://gitlab.example.com/group/repo.git"
		if gotHydrationURL != wantHydrationURL {
			t.Fatalf("hydration repo URL=%q, want %q", gotHydrationURL, wantHydrationURL)
		}
		if manifest.Gate == nil {
			t.Fatal("expected gate config")
		}
		if gotGateURL := manifest.Gate.RepoURL.String(); gotGateURL != "https://gitlab.example.com/group/repo.git" {
			t.Fatalf("gate repo URL=%q, want clean URL", gotGateURL)
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

	t.Run("keeps base_ref in repo materialization", func(t *testing.T) {
		req := newStartRunRequest()
		manifest, err := buildManifestDefault(req)
		if err != nil {
			t.Fatalf("buildManifestDefault() error: %v", err)
		}
		if manifest.Inputs[0].Hydration.Repo.BaseRef.String() != "main" {
			t.Errorf("expected base_ref main, got %q", manifest.Inputs[0].Hydration.Repo.BaseRef.String())
		}
	})

	t.Run("validates manifest", func(t *testing.T) {
		req := newStartRunRequest()
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
			Execution: ContainerSpec{Command: contracts.CommandSpec{Shell: "echo hi"}},
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
			Execution: ContainerSpec{Image: contracts.JobImage{Universal: "docker.io/example/migs-openrewrite:latest"}},
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

	t.Run("single-step image template expands stack and env placeholders", func(t *testing.T) {
		t.Setenv("PLOY_CONTAINER_REGISTRY", "registry.example/ploy")
		t.Setenv("MIG_TAG", "v2")

		req := newStartRunRequest(withRunOptions(RunOptions{
			Execution: ContainerSpec{
				Image: contracts.JobImage{
					Universal: "${PLOY_CONTAINER_REGISTRY}/my-image-${stack.language}-${stack.release}-${stack.tool}:${MIG_TAG}",
				},
			},
		}))
		req.DetectedStack = &contracts.StackExpectation{
			Language: "java",
			Release:  "17",
			Tool:     "maven",
		}

		manifest, err := buildMigManifest(req, req.TypedOptions, 0, contracts.MigStackJavaMaven)
		if err != nil {
			t.Fatalf("buildMigManifest() error: %v", err)
		}
		if got, want := manifest.Image, "registry.example/ploy/my-image-java-17-maven:v2"; got != want {
			t.Fatalf("image=%q, want %q", got, want)
		}
	})

	t.Run("single-step image template fails when stack value is unavailable", func(t *testing.T) {
		req := newStartRunRequest(withRunOptions(RunOptions{
			Execution: ContainerSpec{
				Image: contracts.JobImage{
					Universal: "ghcr.io/acme/my-image-${stack.language}-${stack.release}-${stack.tool}:latest",
				},
			},
		}))
		req.DetectedStack = &contracts.StackExpectation{
			Language: "java",
			Tool:     "maven",
		}

		_, err := buildMigManifest(req, req.TypedOptions, 0, contracts.MigStackJavaMaven)
		if err == nil {
			t.Fatal("expected error for missing stack.release placeholder")
		}
		if !strings.Contains(err.Error(), "execution image template expansion: unresolved stack placeholders: stack.release") {
			t.Fatalf("error=%q, want unresolved stack.release", err.Error())
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

	t.Run("multi-step run builds manifest for each step from steps array", func(t *testing.T) {
		req := newStartRunRequest(
			withRunEnv(map[string]string{"BASE_VAR": "base_value"}),
			withRunOptions(RunOptions{
				Steps: []StepOptions{
					{ContainerSpec: ContainerSpec{
						Image:   contracts.JobImage{Universal: "migs-orw:latest"},
						Command: contracts.CommandSpec{Exec: []string{"--apply", "--dir", "/workspace"}},
						Env:     map[string]string{"STEP_VAR": "step0"},
						Options: map[string]any{"mount_docker_socket": true},
					}},
					{ContainerSpec: ContainerSpec{
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
		if m0.Envs["BASE_VAR"] != "base_value" {
			t.Errorf("step 0: BASE_VAR=%q, want base_value", m0.Envs["BASE_VAR"])
		}
		if m0.Envs["STEP_VAR"] != "step0" {
			t.Errorf("step 0: STEP_VAR=%q, want step0", m0.Envs["STEP_VAR"])
		}
		if got, ok := m0.OptionBool("mount_docker_socket"); !ok || !got {
			t.Fatalf("step 0: mount_docker_socket option = %v, %v; want true, true", got, ok)
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
		if m1.Envs["BASE_VAR"] != "base_value" {
			t.Errorf("step 1: BASE_VAR=%q, want base_value", m1.Envs["BASE_VAR"])
		}
		if m1.Envs["STEP_VAR"] != "step1" {
			t.Errorf("step 1: STEP_VAR=%q, want step1", m1.Envs["STEP_VAR"])
		}
		if _, ok := m1.OptionBool("mount_docker_socket"); ok {
			t.Fatalf("step 1: mount_docker_socket option should be absent")
		}
	})

	t.Run("multi-step run: step env overrides base env", func(t *testing.T) {
		req := newStartRunRequest(
			withRunEnv(map[string]string{"SHARED_VAR": "base", "UNIQUE_BASE": "base"}),
			withRunOptions(RunOptions{
				Steps: []StepOptions{{ContainerSpec: ContainerSpec{
					Image: contracts.JobImage{Universal: "migs-step:latest"},
					Env:   map[string]string{"SHARED_VAR": "step_override"},
				}}},
			}),
		)
		manifest, err := buildManifestAtStep(req, 0)
		if err != nil {
			t.Fatalf("buildManifestAtStep() error: %v", err)
		}
		if manifest.Envs["SHARED_VAR"] != "step_override" {
			t.Errorf("expected step env override: SHARED_VAR=step_override, got %q", manifest.Envs["SHARED_VAR"])
		}
		if manifest.Envs["UNIQUE_BASE"] != "base" {
			t.Errorf("expected base env preserved: UNIQUE_BASE=base, got %q", manifest.Envs["UNIQUE_BASE"])
		}
	})

	t.Run("single-step run: step env and options are forwarded from typed options", func(t *testing.T) {
		req := newStartRunRequest(
			withRunEnv(map[string]string{"DOCKER_HOST": "tcp://remote:2375", "BASE_VAR": "base"}),
			withRunOptions(RunOptions{
				Execution: ContainerSpec{
					Image:   contracts.JobImage{Universal: "migs-step:latest"},
					Env:     map[string]string{"DOCKER_HOST": "unix:///var/run/docker.sock", "STEP_VAR": "step"},
					Options: map[string]any{"mount_docker_socket": true},
				},
			}),
		)
		manifest, err := buildManifestDefault(req)
		if err != nil {
			t.Fatalf("buildManifestDefault() error: %v", err)
		}
		if got := manifest.Envs["DOCKER_HOST"]; got != "unix:///var/run/docker.sock" {
			t.Fatalf("DOCKER_HOST=%q, want step env override", got)
		}
		if got := manifest.Envs["BASE_VAR"]; got != "base" {
			t.Fatalf("BASE_VAR=%q, want base", got)
		}
		if got := manifest.Envs["STEP_VAR"]; got != "step" {
			t.Fatalf("STEP_VAR=%q, want step", got)
		}
		if got, ok := manifest.OptionBool("mount_docker_socket"); !ok || !got {
			t.Fatalf("mount_docker_socket option = %v, %v; want true, true", got, ok)
		}
	})

	t.Run("single-step run: injects node-owned server url after spec env", func(t *testing.T) {
		req := newStartRunRequest(
			withRunServerURL("https://ploy.example"),
			withRunEnv(map[string]string{"PLOY_SERVER_URL": "https://user.example", "APP_VAR": "value"}),
		)
		manifest, err := buildManifestDefault(req)
		if err != nil {
			t.Fatalf("buildManifestDefault() error: %v", err)
		}
		if got := manifest.Envs["PLOY_SERVER_URL"]; got != "https://ploy.example" {
			t.Fatalf("PLOY_SERVER_URL=%q, want node-owned server URL", got)
		}
		if got := manifest.Envs["APP_VAR"]; got != "value" {
			t.Fatalf("APP_VAR=%q, want value", got)
		}
	})

	t.Run("multi-step run: injects node-owned server url after step env", func(t *testing.T) {
		req := newStartRunRequest(
			withRunServerURL("https://ploy.example"),
			withRunEnv(map[string]string{"PLOY_SERVER_URL": "https://base.example"}),
			withRunOptions(RunOptions{
				Steps: []StepOptions{
					{ContainerSpec: ContainerSpec{
						Image: contracts.JobImage{Universal: "migs-step0:latest"},
						Env:   map[string]string{"PLOY_SERVER_URL": "https://step0.example"},
					}},
					{ContainerSpec: ContainerSpec{
						Image: contracts.JobImage{Universal: "migs-step1:latest"},
						Env:   map[string]string{"PLOY_SERVER_URL": "https://step1.example"},
					}},
				},
			}),
		)
		for step := range req.TypedOptions.Steps {
			manifest, err := buildManifestAtStep(req, step)
			if err != nil {
				t.Fatalf("buildManifestAtStep(%d) error: %v", step, err)
			}
			if got := manifest.Envs["PLOY_SERVER_URL"]; got != "https://ploy.example" {
				t.Fatalf("step %d PLOY_SERVER_URL=%q, want node-owned server URL", step, got)
			}
		}
	})

	t.Run("multi-step run: step index out of range returns error", func(t *testing.T) {
		req := newStartRunRequest(withRunOptions(RunOptions{
			Steps: []StepOptions{{ContainerSpec: ContainerSpec{Image: contracts.JobImage{Universal: "migs-step:latest"}}}},
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
			Execution: ContainerSpec{
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
			baseRef   string
			wantRef   string
		}{
			{"CommitSHA takes precedence", "abc123def456", "main", "abc123def456"},
			{"BaseRef as fallback", "", "main", "main"},
			{"empty when no refs provided", "", "", ""},
		}
		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				t.Parallel()
				req := newStartRunRequest(
					withRunURL("https://gitlab.com/iw2rmb/ploy-orw.git"),
					withRunBaseRef(tt.baseRef),
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
			withRunURL("  https://gitlab.com/iw2rmb/ploy-orw.git  "),
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
			withRunURL("https://gitlab.com/iw2rmb/ploy-orw.git"),
			withRunOptions(RunOptions{BuildGate: BuildGateOptions{
				Images: []contracts.BuildGateImageRule{{
					Stack: contracts.StackExpectation{Language: "java", Tool: "maven", Release: "17"},
					Image: "maven:jdk17",
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
		if got := manifest.Gate.ImageOverrides[0].Image; got != "maven:jdk17" {
			t.Errorf("Gate.ImageOverrides[0].Image=%q, want maven:jdk17", got)
		}
		if manifest.Gate.RepoURL.String() != req.RepoURL.String() {
			t.Errorf("Gate.RepoURL=%q, want %q", manifest.Gate.RepoURL, req.RepoURL.String())
		}
	})
}

func TestBuildGateManifestFromRequest_IgnoresStackAwareJobImages(t *testing.T) {
	t.Parallel()

	req := newStartRunRequest(withRunOptions(RunOptions{
		Steps: []StepOptions{{ContainerSpec: ContainerSpec{
			Image: contracts.JobImage{ByStack: map[contracts.MigStack]string{
				contracts.MigStackJavaMaven:  "docker.io/example/orw-cli:latest",
				contracts.MigStackJavaGradle: "docker.io/example/orw-cli:latest",
			}},
		}}},
	}))

	manifest, err := buildGateManifest(req, req.TypedOptions)
	if err != nil {
		t.Fatalf("buildGateManifest() error: %v", err)
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
