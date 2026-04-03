package nodeagent

import (
	"encoding/json"
	"slices"
	"testing"

	"github.com/iw2rmb/ploy/internal/workflow/contracts"
)

func assertImage(t *testing.T, label string, img contracts.JobImage, stack contracts.MigStack, want string) {
	t.Helper()
	got, err := img.ResolveImage(stack)
	if err != nil {
		t.Fatalf("%s: resolve: %v", label, err)
	}
	if got != want {
		t.Errorf("%s: got %q, want %q", label, got, want)
	}
}

// --- spec-builder helpers ---

func makeStep(img string, mutators ...func(*contracts.MigStep)) contracts.MigStep {
	s := contracts.MigStep{Image: testJobImage(img)}
	for _, m := range mutators {
		m(&s)
	}
	return s
}

func withHealing(kind string, action contracts.HealingActionSpec) *contracts.BuildGateConfig {
	return &contracts.BuildGateConfig{
		Healing: &contracts.HealingSpec{
			SelectedErrorKind: kind,
			ByErrorKind:       map[string]contracts.HealingActionSpec{kind: action},
		},
	}
}

func withRouter(r contracts.RouterSpec) *contracts.BuildGateConfig {
	return &contracts.BuildGateConfig{
		Router: &r,
	}
}

func singleStepSpec(img string, mutators ...func(*contracts.MigSpec)) *contracts.MigSpec {
	s := &contracts.MigSpec{Steps: []contracts.MigStep{makeStep(img)}}
	for _, m := range mutators {
		m(s)
	}
	return s
}

// --- parseSpec pipeline tests ---

func TestParseSpec(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		json  string
		check func(t *testing.T, env map[string]string, opts RunOptions, err error)
	}{
		{
			name: "produces_typed_options",
			json: `{
				"steps": [{
					"image": "docker.io/test/mig:latest",
					"command": "run-test.sh"
				}],
				"build_gate": {"enabled": false},
				"gitlab_pat": "glpat-secret",
				"mr_on_success": true,
				"job_id": "` + testKSUID + `",
				"artifact_name": "bundle.tar.gz",
				"artifact_paths": ["a.txt", "b/"]
			}`,
			check: func(t *testing.T, _ map[string]string, opts RunOptions, _ error) {
				assertImage(t, "Execution.Image", opts.Execution.Image, contracts.MigStackUnknown, "docker.io/test/mig:latest")
				if opts.Execution.Command.Shell != "run-test.sh" {
					t.Errorf("Command.Shell: got %q, want run-test.sh", opts.Execution.Command.Shell)
				}
				if !opts.MRFlagsPresent.MROnSuccessSet || !opts.MRWiring.MROnSuccess {
					t.Errorf("expected mr_on_success present and true")
				}
				if len(opts.Artifacts.Paths) != 2 {
					t.Fatalf("expected 2 artifact_paths, got %d", len(opts.Artifacts.Paths))
				}
			},
		},
		{
			name: "single_step_merges_step_env",
			json: `{
				"envs": {"A":"1","B":"2"},
				"steps": [{"image": "img", "envs": {"B":"step","C":"3"}}]
			}`,
			check: func(t *testing.T, env map[string]string, _ RunOptions, _ error) {
				if env["A"] != "1" || env["B"] != "step" || env["C"] != "3" {
					t.Fatalf("env merge mismatch: got %+v", env)
				}
			},
		},
		{
			name: "multi_step_returns_global_env_only",
			json: `{
				"envs": {"A":"1"},
				"steps": [
					{"image":"a","envs":{"A":"step0","B":"0"}},
					{"image":"b","envs":{"A":"step1","B":"1"}}
				]
			}`,
			check: func(t *testing.T, env map[string]string, opts RunOptions, _ error) {
				if env["A"] != "1" || len(env) != 1 {
					t.Fatalf("env should contain only global env for multi-step, got %+v", env)
				}
				if len(opts.Steps) != 2 {
					t.Fatalf("expected 2 steps, got %d", len(opts.Steps))
				}
				if opts.Steps[0].Env["A"] != "step0" || opts.Steps[1].Env["A"] != "step1" {
					t.Fatalf("expected per-step env in typed options, got step0=%+v step1=%+v", opts.Steps[0].Env, opts.Steps[1].Env)
				}
			},
		},
		{
			name: "mig_index_rejected",
			json: `{
				"mig_index": 1,
				"steps": [
					{"image":"docker.io/test/step-a:v1"},
					{"image":"docker.io/test/step-b:v1"}
				]
			}`,
			check: func(t *testing.T, _ map[string]string, opts RunOptions, _ error) {
				if len(opts.Steps) != 0 {
					t.Fatalf("expected mig_index to be rejected (zero typed options), got steps_len=%d", len(opts.Steps))
				}
				if !opts.Execution.Image.IsEmpty() {
					t.Fatalf("expected mig_index to be rejected (zero typed options), got execution.image=%v", opts.Execution.Image)
				}
			},
		},
		{
			name: "image_map_populates_execution_image",
			json: `{
				"steps": [{
					"image": {
						"java-maven": "ghcr.io/iw2rmb/ploy/orw-cli:latest",
						"java-gradle": "ghcr.io/iw2rmb/ploy/orw-cli:latest"
					}
				}]
			}`,
			check: func(t *testing.T, _ map[string]string, opts RunOptions, _ error) {
				assertImage(t, "Execution.Image(maven)", opts.Execution.Image, contracts.MigStackJavaMaven, "ghcr.io/iw2rmb/ploy/orw-cli:latest")
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			var raw json.RawMessage = []byte(tt.json)
			env, opts, err := parseSpec(raw)
			tt.check(t, env, opts, err)
		})
	}
}

// --- migsSpecToRunOptions direct conversion tests ---

func TestMigsSpecToRunOptions_DirectConversion(t *testing.T) {
	t.Parallel()

	t.Run("single_step_with_all_options", func(t *testing.T) {
		t.Parallel()

		mrOnSuccess := true
		mrOnFail := false

		spec := &contracts.MigSpec{
			JobID: "job-direct-test-123",
			Steps: []contracts.MigStep{
				{
					Image:   testJobImage("docker.io/test/mig:v1"),
					Command: contracts.CommandSpec{Exec: []string{"echo", "hello"}},
					Envs:    map[string]string{"KEY": "value"},
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
							Image:   testJobImage("docker.io/test/heal:v1"),
							Command: contracts.CommandSpec{Shell: "fix.sh"},
							Envs:    map[string]string{"MODE": "auto"},
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

		runOpts := migsSpecToRunOptions(spec)

		if runOpts.ServerMetadata.JobID.String() != "job-direct-test-123" {
			t.Errorf("JobID: got %q, want %q", runOpts.ServerMetadata.JobID.String(), "job-direct-test-123")
		}

		assertImage(t, "Execution.Image", runOpts.Execution.Image, contracts.MigStackUnknown, "docker.io/test/mig:v1")
		if want := []string{"echo", "hello"}; !slices.Equal(runOpts.Execution.Command.Exec, want) {
			t.Fatalf("Execution.Command.Exec = %v, want %v", runOpts.Execution.Command.Exec, want)
		}

		if !runOpts.BuildGate.Enabled {
			t.Error("BuildGate.Enabled: expected true")
		}
		if runOpts.BuildGate.Pre != spec.BuildGate.Pre {
			t.Error("BuildGate.Pre: expected same pointer as spec")
		}
		if runOpts.BuildGate.Post != spec.BuildGate.Post {
			t.Error("BuildGate.Post: expected same pointer as spec")
		}

		if runOpts.Healing == nil {
			t.Fatal("expected Healing config")
		}
		if runOpts.Healing.Retries != 3 {
			t.Errorf("Healing.Retries: got %d, want 3", runOpts.Healing.Retries)
		}
		assertImage(t, "Healing.Mig.Image", runOpts.Healing.Mig.Image, contracts.MigStackUnknown, "docker.io/test/heal:v1")

		if runOpts.MRWiring.GitLabPAT != "glpat-secret" {
			t.Errorf("MRWiring.GitLabPAT: got %q, want glpat-secret", runOpts.MRWiring.GitLabPAT)
		}
		if !runOpts.MRFlagsPresent.MROnSuccessSet || !runOpts.MRWiring.MROnSuccess {
			t.Errorf("expected mr_on_success present and true")
		}
		if !runOpts.MRFlagsPresent.MROnFailSet || runOpts.MRWiring.MROnFail {
			t.Errorf("expected mr_on_fail present and false")
		}

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

		spec := &contracts.MigSpec{
			Steps: []contracts.MigStep{
				{
					Image:   testJobImage("docker.io/test/step1:v1"),
					Command: contracts.CommandSpec{Shell: "step1.sh"},
					Envs:    map[string]string{"STEP": "1"},
				},
				{
					Image:   testJobImage("docker.io/test/step2:v1"),
					Command: contracts.CommandSpec{Exec: []string{"step2", "--flag"}},
					Envs:    map[string]string{"STEP": "2"},
				},
			},
		}

		runOpts := migsSpecToRunOptions(spec)

		if len(runOpts.Steps) != 2 {
			t.Fatalf("Steps: expected 2, got %d", len(runOpts.Steps))
		}

		assertImage(t, "Steps[0].Image", runOpts.Steps[0].Image, contracts.MigStackUnknown, "docker.io/test/step1:v1")
		if runOpts.Steps[0].Command.Shell != "step1.sh" {
			t.Errorf("Steps[0].Command.Shell: got %q, want step1.sh", runOpts.Steps[0].Command.Shell)
		}
		if runOpts.Steps[0].Env["STEP"] != "1" {
			t.Errorf("Steps[0].Env[STEP]: got %q, want 1", runOpts.Steps[0].Env["STEP"])
		}

		assertImage(t, "Steps[1].Image", runOpts.Steps[1].Image, contracts.MigStackUnknown, "docker.io/test/step2:v1")
		if want := []string{"step2", "--flag"}; !slices.Equal(runOpts.Steps[1].Command.Exec, want) {
			t.Fatalf("Steps[1].Command.Exec = %v, want %v", runOpts.Steps[1].Command.Exec, want)
		}

		if !runOpts.Execution.Image.IsEmpty() {
			t.Errorf("Execution.Image: expected empty for multi-step spec")
		}
	})

	t.Run("nil_spec_returns_zero_value", func(t *testing.T) {
		t.Parallel()

		runOpts := migsSpecToRunOptions(nil)
		if !runOpts.Execution.Image.IsEmpty() {
			t.Error("expected empty Execution.Image for nil spec")
		}
		if runOpts.Healing != nil {
			t.Error("expected nil Healing for nil spec")
		}
	})

	t.Run("healing_retries_defaults_to_1", func(t *testing.T) {
		t.Parallel()

		spec := singleStepSpec("img", func(s *contracts.MigSpec) {
			s.BuildGate = withHealing("infra", contracts.HealingActionSpec{
				Retries: 0,
				Image:   testJobImage("heal"),
			})
		})

		runOpts := migsSpecToRunOptions(spec)

		if runOpts.Healing == nil {
			t.Fatal("expected Healing config")
		}
		if runOpts.Healing.Retries != 1 {
			t.Errorf("Healing.Retries: got %d, want 1 (default)", runOpts.Healing.Retries)
		}
	})

	t.Run("stack_aware_image_preserved", func(t *testing.T) {
		t.Parallel()

		spec := &contracts.MigSpec{
			Steps: []contracts.MigStep{
				{
					Image: contracts.JobImage{
						ByStack: map[contracts.MigStack]string{
							contracts.MigStackDefault:    "docker.io/test/default:v1",
							contracts.MigStackJavaMaven:  "docker.io/test/maven:v1",
							contracts.MigStackJavaGradle: "docker.io/test/gradle:v1",
						},
					},
				},
			},
		}

		runOpts := migsSpecToRunOptions(spec)
		assertImage(t, "Maven image", runOpts.Execution.Image, contracts.MigStackJavaMaven, "docker.io/test/maven:v1")
	})
}

// --- field propagation tests ---

func TestMigsSpecToRunOptions_FieldPropagation(t *testing.T) {
	t.Parallel()

	hydraCA := []string{"abc1234"}
	hydraIn := []string{"def5678:/in/config"}
	hydraOut := []string{"aaa1111:/out/result"}
	hydraHome := []string{"bbb2222:dotfile:ro"}
	amataSpec := &contracts.AmataRunSpec{
		Spec: "task: fix-it\nprompt: fix the bug",
		Set: []contracts.AmataSetParam{
			{Param: "repo", Value: "myrepo"},
			{Param: "env", Value: "prod"},
		},
	}

	type fieldProbe struct {
		name          string
		stepMutator   func(*contracts.MigStep)
		healMutator   func(*contracts.HealingActionSpec)
		routerMutator func(*contracts.RouterSpec)
		checkPresent  func(t *testing.T, mc MigContainerSpec)
		checkAbsent   func(t *testing.T, mc MigContainerSpec)
	}

	probes := []fieldProbe{
		{
			name: "HydraFields",
			stepMutator: func(s *contracts.MigStep) {
				s.CA = hydraCA
				s.In = hydraIn
				s.Out = hydraOut
				s.Home = hydraHome
			},
			healMutator: func(a *contracts.HealingActionSpec) {
				a.CA = hydraCA
				a.In = hydraIn
				a.Out = hydraOut
				a.Home = hydraHome
			},
			routerMutator: func(r *contracts.RouterSpec) {
				r.CA = hydraCA
				r.In = hydraIn
				r.Out = hydraOut
				r.Home = hydraHome
			},
			checkPresent: func(t *testing.T, mc MigContainerSpec) {
				t.Helper()
				if len(mc.CA) != 1 || mc.CA[0] != "abc1234" {
					t.Fatalf("CA: got %v", mc.CA)
				}
				if len(mc.In) != 1 || mc.In[0] != "def5678:/in/config" {
					t.Fatalf("In: got %v", mc.In)
				}
				if len(mc.Out) != 1 || mc.Out[0] != "aaa1111:/out/result" {
					t.Fatalf("Out: got %v", mc.Out)
				}
				if len(mc.Home) != 1 || mc.Home[0] != "bbb2222:dotfile:ro" {
					t.Fatalf("Home: got %v", mc.Home)
				}
			},
			checkAbsent: func(t *testing.T, mc MigContainerSpec) {
				t.Helper()
				if len(mc.CA) != 0 {
					t.Errorf("CA: got %v, want empty", mc.CA)
				}
				if len(mc.In) != 0 {
					t.Errorf("In: got %v, want empty", mc.In)
				}
			},
		},
		{
			name:          "Amata",
			stepMutator:   func(s *contracts.MigStep) { s.Amata = amataSpec },
			healMutator:   func(a *contracts.HealingActionSpec) { a.Amata = amataSpec },
			routerMutator: func(r *contracts.RouterSpec) { r.Amata = amataSpec },
			checkPresent: func(t *testing.T, mc MigContainerSpec) {
				t.Helper()
				if mc.Amata == nil || mc.Amata.Spec != amataSpec.Spec {
					t.Fatalf("Amata: got %v", mc.Amata)
				}
				if len(mc.Amata.Set) != 2 {
					t.Fatalf("Amata.Set len: got %d, want 2", len(mc.Amata.Set))
				}
			},
			checkAbsent: func(t *testing.T, mc MigContainerSpec) {
				t.Helper()
				if mc.Amata != nil {
					t.Errorf("Amata: got non-nil, want nil")
				}
			},
		},
	}

	for _, probe := range probes {
		t.Run(probe.name, func(t *testing.T) {
			t.Parallel()

			t.Run("single_step", func(t *testing.T) {
				t.Parallel()
				step := makeStep("img")
				probe.stepMutator(&step)
				opts := migsSpecToRunOptions(&contracts.MigSpec{Steps: []contracts.MigStep{step}})
				probe.checkPresent(t, opts.Execution)
			})

			t.Run("multi_step", func(t *testing.T) {
				t.Parallel()
				step0 := makeStep("img0")
				probe.stepMutator(&step0)
				opts := migsSpecToRunOptions(&contracts.MigSpec{Steps: []contracts.MigStep{step0, makeStep("img1")}})
				if len(opts.Steps) != 2 {
					t.Fatalf("Steps len: got %d, want 2", len(opts.Steps))
				}
				probe.checkPresent(t, opts.Steps[0].MigContainerSpec)
				probe.checkAbsent(t, opts.Steps[1].MigContainerSpec)
			})

			t.Run("healing", func(t *testing.T) {
				t.Parallel()
				action := contracts.HealingActionSpec{Image: testJobImage("heal-img")}
				probe.healMutator(&action)
				spec := singleStepSpec("img", func(s *contracts.MigSpec) {
					s.BuildGate = withHealing("code", action)
				})
				opts := migsSpecToRunOptions(spec)
				if opts.Healing == nil {
					t.Fatal("expected Healing config")
				}
				probe.checkPresent(t, opts.Healing.Mig)
			})

			t.Run("router", func(t *testing.T) {
				t.Parallel()
				router := contracts.RouterSpec{Image: testJobImage("router-img")}
				probe.routerMutator(&router)
				spec := singleStepSpec("img", func(s *contracts.MigSpec) {
					s.BuildGate = withRouter(router)
				})
				opts := migsSpecToRunOptions(spec)
				if opts.Router == nil {
					t.Fatal("expected Router config")
				}
				probe.checkPresent(t, *opts.Router)
			})
		})
	}

	t.Run("nil_propagates_nil", func(t *testing.T) {
		t.Parallel()
		spec := singleStepSpec("img", func(s *contracts.MigSpec) {
			s.BuildGate = &contracts.BuildGateConfig{
				Router: &contracts.RouterSpec{Image: testJobImage("router-img")},
				Healing: &contracts.HealingSpec{
					SelectedErrorKind: "code",
					ByErrorKind: map[string]contracts.HealingActionSpec{
						"code": {Image: testJobImage("heal-img")},
					},
				},
			}
		})
		opts := migsSpecToRunOptions(spec)
		if opts.Router != nil && opts.Router.Amata != nil {
			t.Error("Router.Amata: expected nil when not configured")
		}
		if opts.Healing != nil && opts.Healing.Mig.Amata != nil {
			t.Error("Healing.Mig.Amata: expected nil when not configured")
		}
	})
}
