package nodeagent

import (
	"encoding/json"
	"slices"
	"testing"

	"github.com/iw2rmb/ploy/internal/workflow/contracts"
)

func assertImage(t *testing.T, label string, img contracts.JobImage, stack contracts.ModStack, want string) {
	t.Helper()
	got, err := img.ResolveImage(stack)
	if err != nil {
		t.Fatalf("%s: resolve: %v", label, err)
	}
	if got != want {
		t.Errorf("%s: got %q, want %q", label, got, want)
	}
}

func TestParseSpec_ProducesTypedOptions(t *testing.T) {
	t.Parallel()

	specJSON := `{
		"steps": [{
			"image": "docker.io/test/mig:latest",
			"command": "run-test.sh"
		}],
		"build_gate": {
			"enabled": false
		},
		"gitlab_pat": "glpat-secret",
		"mr_on_success": true,
		"job_id": "` + testKSUID + `",
		"artifact_name": "bundle.tar.gz",
		"artifact_paths": ["a.txt", "b/"]
	}`

	var raw json.RawMessage = []byte(specJSON)
	_, typedOpts, _ := parseSpec(raw)

	// Spot-check: JSON→parseSpec→RunOptions pipeline populates key fields.
	// Exhaustive field coverage lives in TestModsSpecToRunOptions_DirectConversion.
	assertImage(t, "Execution.Image", typedOpts.Execution.Image, contracts.ModStackUnknown, "docker.io/test/mig:latest")
	if typedOpts.Execution.Command.Shell != "run-test.sh" {
		t.Errorf("Command.Shell: got %q, want run-test.sh", typedOpts.Execution.Command.Shell)
	}
	if !typedOpts.MRFlagsPresent.MROnSuccessSet || !typedOpts.MRWiring.MROnSuccess {
		t.Errorf("expected mr_on_success present and true")
	}
	if len(typedOpts.Artifacts.Paths) != 2 {
		t.Fatalf("expected 2 artifact_paths, got %d", len(typedOpts.Artifacts.Paths))
	}
}

func TestParseSpec_EnvMergingSemantics(t *testing.T) {
	t.Parallel()

	t.Run("single_step_merges_step_env", func(t *testing.T) {
		specJSON := `{
			"env": {"A":"1","B":"2"},
			"steps": [{
				"image": "img",
				"env": {"B":"step","C":"3"}
			}]
		}`

		var raw json.RawMessage = []byte(specJSON)
		env, _, _ := parseSpec(raw)

		if env["A"] != "1" || env["B"] != "step" || env["C"] != "3" {
			t.Fatalf("env merge mismatch: got %+v", env)
		}
	})

	t.Run("multi_step_returns_global_env_only", func(t *testing.T) {
		specJSON := `{
			"env": {"A":"1"},
			"steps": [
				{"image":"a","env":{"A":"step0","B":"0"}},
				{"image":"b","env":{"A":"step1","B":"1"}}
			]
		}`

		var raw json.RawMessage = []byte(specJSON)
		env, typedOpts, _ := parseSpec(raw)

		if env["A"] != "1" || len(env) != 1 {
			t.Fatalf("env should contain only global env for multi-step, got %+v", env)
		}
		if len(typedOpts.Steps) != 2 {
			t.Fatalf("expected 2 steps, got %d", len(typedOpts.Steps))
		}
		if typedOpts.Steps[0].Env["A"] != "step0" || typedOpts.Steps[1].Env["A"] != "step1" {
			t.Fatalf("expected per-step env in typed options, got step0=%+v step1=%+v", typedOpts.Steps[0].Env, typedOpts.Steps[1].Env)
		}
	})
}

func TestParseSpec_ModIndexRejected(t *testing.T) {
	t.Parallel()

	specJSON := `{
		"mod_index": 1,
		"steps": [
			{"image":"docker.io/test/step-a:v1"},
			{"image":"docker.io/test/step-b:v1"}
		]
	}`

	var raw json.RawMessage = []byte(specJSON)
	_, typedOpts, _ := parseSpec(raw)

	if len(typedOpts.Steps) != 0 {
		t.Fatalf("expected mod_index to be rejected (zero typed options), got steps_len=%d", len(typedOpts.Steps))
	}
	if !typedOpts.Execution.Image.IsEmpty() {
		t.Fatalf("expected mod_index to be rejected (zero typed options), got execution.image=%v", typedOpts.Execution.Image)
	}
}

func TestParseSpec_ImageMap_PopulatesExecutionImage(t *testing.T) {
	t.Parallel()

	specJSON := `{
		"steps": [{
			"image": {
				"default": "docker.io/user/migs-orw:latest",
				"java-maven": "docker.io/user/orw-cli:latest",
				"java-gradle": "docker.io/user/orw-cli:latest"
			}
		}]
	}`

	var raw json.RawMessage = []byte(specJSON)
	_, typedOpts, _ := parseSpec(raw)

	assertImage(t, "Execution.Image(maven)", typedOpts.Execution.Image, contracts.ModStackJavaMaven, "docker.io/user/orw-cli:latest")
}

func TestModsSpecToRunOptions_DirectConversion(t *testing.T) {
	t.Parallel()

	t.Run("single_step_with_all_options", func(t *testing.T) {
		t.Parallel()

		mrOnSuccess := true
		mrOnFail := false

		spec := &contracts.ModsSpec{
			JobID: "job-direct-test-123",
			Steps: []contracts.ModStep{
				{
					Image:   contracts.JobImage{Universal: "docker.io/test/mig:v1"},
					Command: contracts.CommandSpec{Exec: []string{"echo", "hello"}},
					Env:     map[string]string{"KEY": "value"},
				},
			},
			BuildGate: &contracts.BuildGateConfig{
				Enabled: true,
				Pre: &contracts.BuildGatePhaseConfig{
					Target: contracts.GateProfileTargetUnit,
					Always: true,
					GateProfile: &contracts.BuildGateProfileOverride{
						Command: contracts.CommandSpec{Shell: "go test ./..."},
						Env:     map[string]string{"GOFLAGS": "-mod=readonly"},
					},
				},
				Post: &contracts.BuildGatePhaseConfig{
					Target: contracts.GateProfileTargetAllTests,
					Always: false,
					GateProfile: &contracts.BuildGateProfileOverride{
						Command: contracts.CommandSpec{Exec: []string{"go", "test", "./...", "-run", "TestUnit"}},
						Env:     map[string]string{"CGO_ENABLED": "0"},
					},
				},
				Healing: &contracts.HealingSpec{
					SelectedErrorKind: "infra",
					ByErrorKind: map[string]contracts.HealingActionSpec{
						"infra": {
							Retries: 3,
							Image:   contracts.JobImage{Universal: "docker.io/test/heal:v1"},
							Command: contracts.CommandSpec{Shell: "fix.sh"},
							Env:     map[string]string{"MODE": "auto"},
						},
					},
				},
			},
			GitLabPAT:     "glpat-secret",
			GitLabDomain:  "gitlab.example.com",
			MROnSuccess:   &mrOnSuccess,
			MROnFail:      &mrOnFail,
			ArtifactPaths: []string{"path/to/file.txt", "path/to/dir/"},
			ArtifactName:  "my-artifact",
		}

		runOpts := modsSpecToRunOptions(spec)

		if runOpts.ServerMetadata.JobID.String() != "job-direct-test-123" {
			t.Errorf("JobID: got %q, want %q", runOpts.ServerMetadata.JobID.String(), "job-direct-test-123")
		}

		// Execution (single-step extraction).
		assertImage(t, "Execution.Image", runOpts.Execution.Image, contracts.ModStackUnknown, "docker.io/test/mig:v1")
		if want := []string{"echo", "hello"}; !slices.Equal(runOpts.Execution.Command.Exec, want) {
			t.Fatalf("Execution.Command.Exec = %v, want %v", runOpts.Execution.Command.Exec, want)
		}

		// BuildGate — Pre/Post are assigned by pointer from spec, so verify
		// the pointers arrived and spot-check a derived field on each.
		if !runOpts.BuildGate.Enabled {
			t.Error("BuildGate.Enabled: expected true")
		}
		if runOpts.BuildGate.Pre != spec.BuildGate.Pre {
			t.Error("BuildGate.Pre: expected same pointer as spec")
		}
		if runOpts.BuildGate.Post != spec.BuildGate.Post {
			t.Error("BuildGate.Post: expected same pointer as spec")
		}

		// Healing.
		if runOpts.Healing == nil {
			t.Fatal("expected Healing config")
		}
		if runOpts.Healing.Retries != 3 {
			t.Errorf("Healing.Retries: got %d, want 3", runOpts.Healing.Retries)
		}
		assertImage(t, "Healing.Mod.Image", runOpts.Healing.Mod.Image, contracts.ModStackUnknown, "docker.io/test/heal:v1")

		// MR wiring.
		if runOpts.MRWiring.GitLabPAT != "glpat-secret" {
			t.Errorf("MRWiring.GitLabPAT: got %q, want glpat-secret", runOpts.MRWiring.GitLabPAT)
		}
		if !runOpts.MRFlagsPresent.MROnSuccessSet || !runOpts.MRWiring.MROnSuccess {
			t.Errorf("expected mr_on_success present and true")
		}
		if !runOpts.MRFlagsPresent.MROnFailSet || runOpts.MRWiring.MROnFail {
			t.Errorf("expected mr_on_fail present and false")
		}

		// Artifacts.
		if runOpts.Artifacts.Name != "my-artifact" {
			t.Errorf("Artifacts.Name: got %q, want my-artifact", runOpts.Artifacts.Name)
		}
		if len(runOpts.Artifacts.Paths) != 2 {
			t.Errorf("Artifacts.Paths: expected 2, got %d", len(runOpts.Artifacts.Paths))
		}

		if len(runOpts.Steps) != 0 {
			t.Errorf("Steps: expected empty for single-step spec, got %d", len(runOpts.Steps))
		}
	})

	t.Run("multi_step_spec", func(t *testing.T) {
		t.Parallel()

		spec := &contracts.ModsSpec{
			Steps: []contracts.ModStep{
				{
					Image:   contracts.JobImage{Universal: "docker.io/test/step1:v1"},
					Command: contracts.CommandSpec{Shell: "step1.sh"},
					Env:     map[string]string{"STEP": "1"},
				},
				{
					Image:   contracts.JobImage{Universal: "docker.io/test/step2:v1"},
					Command: contracts.CommandSpec{Exec: []string{"step2", "--flag"}},
					Env:     map[string]string{"STEP": "2"},
				},
			},
		}

		runOpts := modsSpecToRunOptions(spec)

		if len(runOpts.Steps) != 2 {
			t.Fatalf("Steps: expected 2, got %d", len(runOpts.Steps))
		}

		assertImage(t, "Steps[0].Image", runOpts.Steps[0].Image, contracts.ModStackUnknown, "docker.io/test/step1:v1")
		if runOpts.Steps[0].Command.Shell != "step1.sh" {
			t.Errorf("Steps[0].Command.Shell: got %q, want step1.sh", runOpts.Steps[0].Command.Shell)
		}
		if runOpts.Steps[0].Env["STEP"] != "1" {
			t.Errorf("Steps[0].Env[STEP]: got %q, want 1", runOpts.Steps[0].Env["STEP"])
		}

		assertImage(t, "Steps[1].Image", runOpts.Steps[1].Image, contracts.ModStackUnknown, "docker.io/test/step2:v1")
		if want := []string{"step2", "--flag"}; !slices.Equal(runOpts.Steps[1].Command.Exec, want) {
			t.Fatalf("Steps[1].Command.Exec = %v, want %v", runOpts.Steps[1].Command.Exec, want)
		}

		if !runOpts.Execution.Image.IsEmpty() {
			t.Errorf("Execution.Image: expected empty for multi-step spec")
		}
	})

	t.Run("nil_spec_returns_zero_value", func(t *testing.T) {
		t.Parallel()

		runOpts := modsSpecToRunOptions(nil)
		if !runOpts.Execution.Image.IsEmpty() {
			t.Error("expected empty Execution.Image for nil spec")
		}
		if runOpts.Healing != nil {
			t.Error("expected nil Healing for nil spec")
		}
	})

	t.Run("healing_retries_defaults_to_1", func(t *testing.T) {
		t.Parallel()

		spec := &contracts.ModsSpec{
			Steps: []contracts.ModStep{{Image: contracts.JobImage{Universal: "img"}}},
			BuildGate: &contracts.BuildGateConfig{
				Healing: &contracts.HealingSpec{
					SelectedErrorKind: "infra",
					ByErrorKind: map[string]contracts.HealingActionSpec{
						"infra": {
							Retries: 0,
							Image:   contracts.JobImage{Universal: "heal"},
						},
					},
				},
			},
		}

		runOpts := modsSpecToRunOptions(spec)

		if runOpts.Healing == nil {
			t.Fatal("expected Healing config")
		}
		if runOpts.Healing.Retries != 1 {
			t.Errorf("Healing.Retries: got %d, want 1 (default)", runOpts.Healing.Retries)
		}
	})

	t.Run("stack_aware_image_preserved", func(t *testing.T) {
		t.Parallel()

		spec := &contracts.ModsSpec{
			Steps: []contracts.ModStep{
				{
					Image: contracts.JobImage{
						ByStack: map[contracts.ModStack]string{
							contracts.ModStackDefault:    "docker.io/test/default:v1",
							contracts.ModStackJavaMaven:  "docker.io/test/maven:v1",
							contracts.ModStackJavaGradle: "docker.io/test/gradle:v1",
						},
					},
				},
			},
		}

		runOpts := modsSpecToRunOptions(spec)

		assertImage(t, "Maven image", runOpts.Execution.Image, contracts.ModStackJavaMaven, "docker.io/test/maven:v1")
	})
}

func TestModsSpecToRunOptions_FieldPropagation(t *testing.T) {
	t.Parallel()

	bundle := &contracts.TmpBundleRef{
		BundleID: "bun-123",
		CID:      "cid-abc",
		Digest:   "sha256:deadbeef",
		Entries:  []string{"config.json", "secret.txt"},
	}
	amataSpec := &contracts.AmataRunSpec{
		Spec: "task: fix-it\nprompt: fix the bug",
		Set: []contracts.AmataSetParam{
			{Param: "repo", Value: "myrepo"},
			{Param: "env", Value: "prod"},
		},
	}

	type propagationCase struct {
		name  string
		spec  *contracts.ModsSpec
		check func(t *testing.T, opts RunOptions)
	}

	runCases := func(t *testing.T, cases []propagationCase) {
		t.Helper()
		for _, tc := range cases {
			t.Run(tc.name, func(t *testing.T) {
				t.Parallel()
				opts := modsSpecToRunOptions(tc.spec)
				tc.check(t, opts)
			})
		}
	}

	t.Run("TmpBundle", func(t *testing.T) {
		t.Parallel()
		runCases(t, []propagationCase{
			{
				name: "single_step",
				spec: &contracts.ModsSpec{
					Steps: []contracts.ModStep{{Image: contracts.JobImage{Universal: "img"}, TmpBundle: bundle}},
				},
				check: func(t *testing.T, opts RunOptions) {
					if opts.Execution.TmpBundle == nil || opts.Execution.TmpBundle.BundleID != "bun-123" {
						t.Fatalf("Execution.TmpBundle: got %v", opts.Execution.TmpBundle)
					}
				},
			},
			{
				name: "multi_step",
				spec: &contracts.ModsSpec{
					Steps: []contracts.ModStep{
						{Image: contracts.JobImage{Universal: "img1"}, TmpBundle: bundle},
						{Image: contracts.JobImage{Universal: "img2"}},
					},
				},
				check: func(t *testing.T, opts RunOptions) {
					if len(opts.Steps) != 2 {
						t.Fatalf("Steps len: got %d, want 2", len(opts.Steps))
					}
					if opts.Steps[0].TmpBundle == nil || opts.Steps[0].TmpBundle.BundleID != "bun-123" {
						t.Errorf("Steps[0].TmpBundle: got %v", opts.Steps[0].TmpBundle)
					}
					if opts.Steps[1].TmpBundle != nil {
						t.Errorf("Steps[1].TmpBundle: got non-nil, want nil")
					}
				},
			},
			{
				name: "healing",
				spec: &contracts.ModsSpec{
					Steps: []contracts.ModStep{{Image: contracts.JobImage{Universal: "img"}}},
					BuildGate: &contracts.BuildGateConfig{
						Healing: &contracts.HealingSpec{
							SelectedErrorKind: "code",
							ByErrorKind: map[string]contracts.HealingActionSpec{
								"code": {Image: contracts.JobImage{Universal: "heal-img"}, TmpBundle: bundle},
							},
						},
					},
				},
				check: func(t *testing.T, opts RunOptions) {
					if opts.Healing == nil {
						t.Fatal("expected Healing config")
					}
					if opts.Healing.Mod.TmpBundle == nil || opts.Healing.Mod.TmpBundle.BundleID != "bun-123" {
						t.Fatalf("Healing.Mod.TmpBundle: got %v", opts.Healing.Mod.TmpBundle)
					}
				},
			},
			{
				name: "router",
				spec: &contracts.ModsSpec{
					Steps: []contracts.ModStep{{Image: contracts.JobImage{Universal: "img"}}},
					BuildGate: &contracts.BuildGateConfig{
						Router: &contracts.RouterSpec{Image: contracts.JobImage{Universal: "router-img"}, TmpBundle: bundle},
					},
				},
				check: func(t *testing.T, opts RunOptions) {
					if opts.Router == nil {
						t.Fatal("expected Router config")
					}
					if opts.Router.TmpBundle == nil || opts.Router.TmpBundle.BundleID != "bun-123" {
						t.Fatalf("Router.TmpBundle: got %v", opts.Router.TmpBundle)
					}
				},
			},
		})
	})

	t.Run("Amata", func(t *testing.T) {
		t.Parallel()
		runCases(t, []propagationCase{
			{
				name: "single_step",
				spec: &contracts.ModsSpec{
					Steps: []contracts.ModStep{{Image: contracts.JobImage{Universal: "img"}, Amata: amataSpec}},
				},
				check: func(t *testing.T, opts RunOptions) {
					if opts.Execution.Amata == nil || opts.Execution.Amata.Spec != amataSpec.Spec {
						t.Fatalf("Execution.Amata: got %v", opts.Execution.Amata)
					}
					if len(opts.Execution.Amata.Set) != 2 {
						t.Fatalf("Execution.Amata.Set len: got %d, want 2", len(opts.Execution.Amata.Set))
					}
				},
			},
			{
				name: "multi_step",
				spec: &contracts.ModsSpec{
					Steps: []contracts.ModStep{
						{Image: contracts.JobImage{Universal: "img0"}},
						{Image: contracts.JobImage{Universal: "img1"}, Amata: amataSpec},
					},
				},
				check: func(t *testing.T, opts RunOptions) {
					if len(opts.Steps) != 2 {
						t.Fatalf("Steps len: got %d, want 2", len(opts.Steps))
					}
					if opts.Steps[0].Amata != nil {
						t.Errorf("Steps[0].Amata: got %+v, want nil", opts.Steps[0].Amata)
					}
					if opts.Steps[1].Amata == nil || opts.Steps[1].Amata.Spec != amataSpec.Spec {
						t.Fatalf("Steps[1].Amata: got %v", opts.Steps[1].Amata)
					}
				},
			},
			{
				name: "healing",
				spec: &contracts.ModsSpec{
					Steps: []contracts.ModStep{{Image: contracts.JobImage{Universal: "img"}}},
					BuildGate: &contracts.BuildGateConfig{
						Healing: &contracts.HealingSpec{
							SelectedErrorKind: "code",
							ByErrorKind: map[string]contracts.HealingActionSpec{
								"code": {Image: contracts.JobImage{Universal: "heal-img"}, Amata: amataSpec},
							},
						},
					},
				},
				check: func(t *testing.T, opts RunOptions) {
					if opts.Healing == nil {
						t.Fatal("expected Healing config")
					}
					if opts.Healing.Mod.Amata == nil || opts.Healing.Mod.Amata.Spec != amataSpec.Spec {
						t.Fatalf("Healing.Mod.Amata: got %v", opts.Healing.Mod.Amata)
					}
					if len(opts.Healing.Mod.Amata.Set) != 2 {
						t.Fatalf("Healing.Mod.Amata.Set len: got %d, want 2", len(opts.Healing.Mod.Amata.Set))
					}
				},
			},
			{
				name: "router",
				spec: &contracts.ModsSpec{
					Steps: []contracts.ModStep{{Image: contracts.JobImage{Universal: "img"}}},
					BuildGate: &contracts.BuildGateConfig{
						Router: &contracts.RouterSpec{Image: contracts.JobImage{Universal: "router-img"}, Amata: amataSpec},
					},
				},
				check: func(t *testing.T, opts RunOptions) {
					if opts.Router == nil {
						t.Fatal("expected Router config")
					}
					if opts.Router.Amata == nil || opts.Router.Amata.Spec != amataSpec.Spec {
						t.Fatalf("Router.Amata: got %v", opts.Router.Amata)
					}
					if len(opts.Router.Amata.Set) != 2 {
						t.Fatalf("Router.Amata.Set len: got %d, want 2", len(opts.Router.Amata.Set))
					}
				},
			},
			{
				name: "nil_propagates_nil",
				spec: &contracts.ModsSpec{
					Steps: []contracts.ModStep{{Image: contracts.JobImage{Universal: "img"}}},
					BuildGate: &contracts.BuildGateConfig{
						Router: &contracts.RouterSpec{Image: contracts.JobImage{Universal: "router-img"}},
						Healing: &contracts.HealingSpec{
							SelectedErrorKind: "code",
							ByErrorKind: map[string]contracts.HealingActionSpec{
								"code": {Image: contracts.JobImage{Universal: "heal-img"}},
							},
						},
					},
				},
				check: func(t *testing.T, opts RunOptions) {
					if opts.Router != nil && opts.Router.Amata != nil {
						t.Error("Router.Amata: expected nil when not configured")
					}
					if opts.Healing != nil && opts.Healing.Mod.Amata != nil {
						t.Error("Healing.Mod.Amata: expected nil when not configured")
					}
				},
			},
		})
	})
}
