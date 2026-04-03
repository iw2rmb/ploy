package handlers

import (
	"encoding/json"
	"strings"
	"testing"

	domaintypes "github.com/iw2rmb/ploy/internal/domain/types"
	"github.com/iw2rmb/ploy/internal/store"
)

// ---------------------------------------------------------------------------
// Global env → envs routing (migrated from spec_utils_global_env_test.go)
// ---------------------------------------------------------------------------

func TestApplyHydraOverlay_GlobalEnvRouting(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		spec       map[string]any
		env        map[string][]GlobalEnvVar
		jobType    domaintypes.JobType
		expectKeys []string
		rejectKeys []string
		checkEnvs  map[string]string
	}{
		{
			name:    "nil env leaves spec unchanged",
			spec:    map[string]any{"foo": "bar"},
			env:     nil,
			jobType: domaintypes.JobTypeMig,
		},
		{
			name:    "empty env leaves spec unchanged",
			spec:    map[string]any{"foo": "bar"},
			env:     map[string][]GlobalEnvVar{},
			jobType: domaintypes.JobTypeMig,
		},
		{
			name:       "empty spec creates envs map",
			spec:       map[string]any{},
			env:        map[string][]GlobalEnvVar{"API_KEY": {{Value: "secret123", Target: domaintypes.GlobalEnvTargetSteps, Secret: true}}},
			jobType:    domaintypes.JobTypeMig,
			expectKeys: []string{"API_KEY"},
			checkEnvs:  map[string]string{"API_KEY": "secret123"},
		},
		{
			name:       "mig job gets steps target",
			spec:       map[string]any{},
			env:        targetTestEnv(),
			jobType:    domaintypes.JobTypeMig,
			expectKeys: []string{"STEPS_KEY"},
			rejectKeys: []string{"GATES_KEY"},
		},
		{
			name:       "heal job gets steps target",
			spec:       map[string]any{},
			env:        targetTestEnv(),
			jobType:    domaintypes.JobTypeHeal,
			expectKeys: []string{"STEPS_KEY"},
			rejectKeys: []string{"GATES_KEY"},
		},
		{
			name:       "pre_gate job gets gates target",
			spec:       map[string]any{},
			env:        targetTestEnv(),
			jobType:    domaintypes.JobTypePreGate,
			expectKeys: []string{"GATES_KEY"},
			rejectKeys: []string{"STEPS_KEY"},
		},
		{
			name:       "re_gate job gets gates target",
			spec:       map[string]any{},
			env:        targetTestEnv(),
			jobType:    domaintypes.JobTypeReGate,
			expectKeys: []string{"GATES_KEY"},
			rejectKeys: []string{"STEPS_KEY"},
		},
		{
			name:       "post_gate job gets gates target",
			spec:       map[string]any{},
			env:        targetTestEnv(),
			jobType:    domaintypes.JobTypePostGate,
			expectKeys: []string{"GATES_KEY"},
			rejectKeys: []string{"STEPS_KEY"},
		},
		{
			name: "per-run envs takes precedence over global",
			spec: map[string]any{
				"envs": map[string]any{"API_KEY": "per-run-value", "OTHER": "existing"},
			},
			env:     map[string][]GlobalEnvVar{"API_KEY": {{Value: "global-value", Target: domaintypes.GlobalEnvTargetSteps}}, "NEW_KEY": {{Value: "new-value", Target: domaintypes.GlobalEnvTargetSteps}}},
			jobType: domaintypes.JobTypeMig,
			checkEnvs: map[string]string{
				"API_KEY": "per-run-value",
				"OTHER":   "existing",
				"NEW_KEY": "new-value",
			},
		},
		{
			name: "preserves other spec fields",
			spec: map[string]any{
				"repo": "github.com/test", "timeout": float64(300),
				"envs": map[string]any{"EXISTING": "yes"},
			},
			env:     map[string][]GlobalEnvVar{"PLOY_CA_CERTS": {{Value: "-----BEGIN CERT-----\n...", Target: domaintypes.GlobalEnvTargetSteps, Secret: true}}},
			jobType: domaintypes.JobTypeMig,
			checkEnvs: map[string]string{
				"EXISTING":      "yes",
				"PLOY_CA_CERTS": "-----BEGIN CERT-----\n...",
			},
		},
		{
			name: "nodes-target provides fallback for mig job",
			spec: map[string]any{},
			env: map[string][]GlobalEnvVar{
				"SHARED_KEY": {{Value: "nodes-val", Target: domaintypes.GlobalEnvTargetNodes}},
			},
			jobType:   domaintypes.JobTypeMig,
			checkEnvs: map[string]string{"SHARED_KEY": "nodes-val"},
		},
		{
			name: "job-target overrides nodes-target on key collision",
			spec: map[string]any{},
			env: map[string][]GlobalEnvVar{
				"SHARED_KEY": {
					{Value: "nodes-val", Target: domaintypes.GlobalEnvTargetNodes},
					{Value: "steps-val", Target: domaintypes.GlobalEnvTargetSteps},
				},
			},
			jobType:   domaintypes.JobTypeMig,
			checkEnvs: map[string]string{"SHARED_KEY": "steps-val"},
		},
		{
			name: "gates-target overrides nodes-target for gate job",
			spec: map[string]any{},
			env: map[string][]GlobalEnvVar{
				"SHARED_KEY": {
					{Value: "nodes-val", Target: domaintypes.GlobalEnvTargetNodes},
					{Value: "gates-val", Target: domaintypes.GlobalEnvTargetGates},
				},
			},
			jobType:   domaintypes.JobTypePostGate,
			checkEnvs: map[string]string{"SHARED_KEY": "gates-val"},
		},
		{
			name: "per-run envs overrides both job-target and nodes-target",
			spec: map[string]any{
				"envs": map[string]any{"SHARED_KEY": "per-run-val"},
			},
			env: map[string][]GlobalEnvVar{
				"SHARED_KEY": {
					{Value: "nodes-val", Target: domaintypes.GlobalEnvTargetNodes},
					{Value: "steps-val", Target: domaintypes.GlobalEnvTargetSteps},
				},
			},
			jobType:   domaintypes.JobTypeMig,
			checkEnvs: map[string]string{"SHARED_KEY": "per-run-val"},
		},
		{
			name: "server-target not injected into jobs",
			spec: map[string]any{},
			env: map[string][]GlobalEnvVar{
				"SERVER_ONLY": {{Value: "server-val", Target: domaintypes.GlobalEnvTargetServer}},
			},
			jobType:    domaintypes.JobTypeMig,
			rejectKeys: []string{"SERVER_ONLY"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			m := cloneSpecMap(tt.spec)
			err := applyHydraOverlayMutator(m, claimSpecMutatorInput{
				job:       store.Job{Meta: []byte(`{}`)},
				globalEnv: tt.env,
				jobType:   tt.jobType,
			})
			if err != nil {
				t.Fatalf("applyHydraOverlayMutator: %v", err)
			}

			em, _ := m["envs"].(map[string]any)

			for _, key := range tt.expectKeys {
				if _, ok := em[key]; !ok {
					t.Errorf("expected key %q to be present in envs", key)
				}
			}
			for _, key := range tt.rejectKeys {
				if em != nil {
					if _, ok := em[key]; ok {
						t.Errorf("expected key %q to be absent from envs", key)
					}
				}
			}
			for key, want := range tt.checkEnvs {
				if em == nil {
					t.Fatalf("envs map is nil, expected key %q=%q", key, want)
				}
				if got := em[key]; got != want {
					t.Errorf("envs[%q] = %v, want %q", key, got, want)
				}
			}
		})
	}
}

// ---------------------------------------------------------------------------
// Typed Hydra overlay merge (envs, ca, in, out, home)
// ---------------------------------------------------------------------------

func TestApplyHydraOverlay_TypedMerge(t *testing.T) {
	t.Parallel()

	t.Run("envs key-based override spec wins", func(t *testing.T) {
		t.Parallel()
		m := map[string]any{
			"envs": map[string]any{"SPEC_KEY": "spec_val", "SHARED": "from_spec"},
		}
		err := applyHydraOverlayMutator(m, claimSpecMutatorInput{
			job:     store.Job{Meta: []byte(`{}`)},
			jobType: domaintypes.JobTypeMig,
			hydraOverlays: map[string]*HydraJobConfig{
				"mig": {Envs: map[string]string{"OVERLAY_KEY": "overlay_val", "SHARED": "from_overlay"}},
			},
		})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		envs := m["envs"].(map[string]any)
		if envs["SPEC_KEY"] != "spec_val" {
			t.Errorf("SPEC_KEY = %v, want spec_val", envs["SPEC_KEY"])
		}
		if envs["OVERLAY_KEY"] != "overlay_val" {
			t.Errorf("OVERLAY_KEY = %v, want overlay_val", envs["OVERLAY_KEY"])
		}
		if envs["SHARED"] != "from_spec" {
			t.Errorf("SHARED = %v, want from_spec (spec wins)", envs["SHARED"])
		}
	})

	t.Run("ca append with dedup", func(t *testing.T) {
		t.Parallel()
		m := map[string]any{
			"ca": []any{"abcdef1234ab", "/ca/extra.pem"},
		}
		err := applyHydraOverlayMutator(m, claimSpecMutatorInput{
			job:     store.Job{Meta: []byte(`{}`)},
			jobType: domaintypes.JobTypeMig,
			hydraOverlays: map[string]*HydraJobConfig{
				"mig": {CA: []string{"abcdef1234ab", "/ca/new.pem"}},
			},
		})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		ca := m["ca"].([]any)
		if len(ca) != 3 {
			t.Fatalf("ca length = %d, want 3: %v", len(ca), ca)
		}
	})

	t.Run("in out home merge by destination", func(t *testing.T) {
		t.Parallel()
		m := map[string]any{
			"in":   []any{"/a.txt:/in/config.json"},
			"out":  []any{"/b.txt:/out/result.txt"},
			"home": []any{"/c.txt:.config/app.toml:ro"},
		}
		err := applyHydraOverlayMutator(m, claimSpecMutatorInput{
			job:     store.Job{Meta: []byte(`{}`)},
			jobType: domaintypes.JobTypeMig,
			hydraOverlays: map[string]*HydraJobConfig{
				"mig": {
					In:   []string{"/overlay.txt:/in/config.json", "/overlay2.txt:/in/extra.json"},
					Out:  []string{"/overlay.txt:/out/new.txt"},
					Home: []string{"/overlay.txt:.config/app.toml", "/overlay.txt:.config/other.toml"},
				},
			},
		})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		in := m["in"].([]any)
		if len(in) != 2 {
			t.Fatalf("in length = %d, want 2: %v", len(in), in)
		}
		if in[0] != "/a.txt:/in/config.json" {
			t.Errorf("in[0] = %v, want spec entry", in[0])
		}

		out := m["out"].([]any)
		if len(out) != 2 {
			t.Fatalf("out length = %d, want 2: %v", len(out), out)
		}

		home := m["home"].([]any)
		if len(home) != 2 {
			t.Fatalf("home length = %d, want 2: %v", len(home), home)
		}
		if home[0] != "/c.txt:.config/app.toml:ro" {
			t.Errorf("home[0] = %v, want spec entry preserved", home[0])
		}
	})

	t.Run("empty spec block gets overlay fields", func(t *testing.T) {
		t.Parallel()
		m := map[string]any{}
		err := applyHydraOverlayMutator(m, claimSpecMutatorInput{
			job:     store.Job{Meta: []byte(`{}`)},
			jobType: domaintypes.JobTypeMig,
			hydraOverlays: map[string]*HydraJobConfig{
				"mig": {
					Envs: map[string]string{"K": "V"},
					CA:   []string{"abc1234567ab"},
					In:   []string{"/f:/in/f.txt"},
				},
			},
		})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		envs := m["envs"].(map[string]any)
		if envs["K"] != "V" {
			t.Fatalf("envs mismatch: %+v", envs)
		}
		ca := m["ca"].([]any)
		if len(ca) != 1 {
			t.Fatalf("ca length = %d, want 1", len(ca))
		}
	})

	t.Run("nil overlay does nothing", func(t *testing.T) {
		t.Parallel()
		m := map[string]any{"envs": map[string]any{"K": "V"}}
		err := applyHydraOverlayMutator(m, claimSpecMutatorInput{
			job:     store.Job{Meta: []byte(`{}`)},
			jobType: domaintypes.JobTypeMig,
		})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if m["envs"].(map[string]any)["K"] != "V" {
			t.Fatal("spec changed unexpectedly")
		}
	})
}

// ---------------------------------------------------------------------------
// Section routing
// ---------------------------------------------------------------------------

func TestApplyHydraOverlay_SectionRouting(t *testing.T) {
	t.Parallel()

	overlays := map[string]*HydraJobConfig{
		"pre_gate":  {Envs: map[string]string{"SECTION": "pre_gate"}},
		"re_gate":   {Envs: map[string]string{"SECTION": "re_gate"}},
		"post_gate": {Envs: map[string]string{"SECTION": "post_gate"}},
		"mig":       {Envs: map[string]string{"SECTION": "mig"}},
		"heal":      {Envs: map[string]string{"SECTION": "heal"}},
	}

	tests := []struct {
		jobType     domaintypes.JobType
		wantSection string
	}{
		{domaintypes.JobTypePreGate, "pre_gate"},
		{domaintypes.JobTypeReGate, "re_gate"},
		{domaintypes.JobTypePostGate, "post_gate"},
		{domaintypes.JobTypeMig, "mig"},
		{domaintypes.JobTypeHeal, "heal"},
	}

	for _, tt := range tests {
		t.Run(string(tt.jobType), func(t *testing.T) {
			t.Parallel()
			m := map[string]any{}
			err := applyHydraOverlayMutator(m, claimSpecMutatorInput{
				job:           store.Job{Meta: []byte(`{}`)},
				jobType:       tt.jobType,
				hydraOverlays: overlays,
			})
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			envs := m["envs"].(map[string]any)
			if got := envs["SECTION"]; got != tt.wantSection {
				t.Errorf("SECTION = %v, want %q", got, tt.wantSection)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// Router phase inheritance
// ---------------------------------------------------------------------------

func TestApplyHydraOverlay_RouterPhaseInheritance(t *testing.T) {
	t.Parallel()

	t.Run("router inherits from pre_gate when pre configured", func(t *testing.T) {
		t.Parallel()
		m := map[string]any{
			"build_gate": map[string]any{
				"pre":    map[string]any{"target": "unit"},
				"router": map[string]any{"image": "router:latest"},
			},
		}
		err := applyHydraOverlayMutator(m, claimSpecMutatorInput{
			job:     store.Job{Meta: []byte(`{}`)},
			jobType: domaintypes.JobTypePreGate,
			hydraOverlays: map[string]*HydraJobConfig{
				"pre_gate":  {Envs: map[string]string{"PHASE": "pre"}, CA: []string{"aaa1234567ab"}},
				"post_gate": {Envs: map[string]string{"PHASE": "post"}},
			},
		})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		router := m["build_gate"].(map[string]any)["router"].(map[string]any)
		envs := router["envs"].(map[string]any)
		if envs["PHASE"] != "pre" {
			t.Errorf("router envs[PHASE] = %v, want pre (from pre_gate overlay)", envs["PHASE"])
		}
		ca := router["ca"].([]any)
		if len(ca) != 1 || ca[0] != "aaa1234567ab" {
			t.Errorf("router ca = %v, want [aaa1234567ab]", ca)
		}
	})

	t.Run("router inherits from post_gate when only post configured", func(t *testing.T) {
		t.Parallel()
		m := map[string]any{
			"build_gate": map[string]any{
				"post":   map[string]any{"target": "build"},
				"router": map[string]any{"image": "router:latest"},
			},
		}
		err := applyHydraOverlayMutator(m, claimSpecMutatorInput{
			job:     store.Job{Meta: []byte(`{}`)},
			jobType: domaintypes.JobTypePostGate,
			hydraOverlays: map[string]*HydraJobConfig{
				"pre_gate":  {Envs: map[string]string{"PHASE": "pre"}},
				"post_gate": {Envs: map[string]string{"PHASE": "post"}},
			},
		})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		router := m["build_gate"].(map[string]any)["router"].(map[string]any)
		envs := router["envs"].(map[string]any)
		if envs["PHASE"] != "post" {
			t.Errorf("router envs[PHASE] = %v, want post (from post_gate overlay)", envs["PHASE"])
		}
	})

	t.Run("no router section does nothing", func(t *testing.T) {
		t.Parallel()
		m := map[string]any{
			"build_gate": map[string]any{
				"pre": map[string]any{"target": "unit"},
			},
		}
		err := applyHydraOverlayMutator(m, claimSpecMutatorInput{
			job:     store.Job{Meta: []byte(`{}`)},
			jobType: domaintypes.JobTypePreGate,
			hydraOverlays: map[string]*HydraJobConfig{
				"pre_gate": {Envs: map[string]string{"PHASE": "pre"}},
			},
		})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		bg := m["build_gate"].(map[string]any)
		if _, ok := bg["router"]; ok {
			t.Error("router section should not be created when absent")
		}
	})

	t.Run("router spec envs win over overlay envs", func(t *testing.T) {
		t.Parallel()
		m := map[string]any{
			"build_gate": map[string]any{
				"pre":    map[string]any{"target": "unit"},
				"router": map[string]any{"image": "router:latest", "envs": map[string]any{"SHARED": "from_spec"}},
			},
		}
		err := applyHydraOverlayMutator(m, claimSpecMutatorInput{
			job:     store.Job{Meta: []byte(`{}`)},
			jobType: domaintypes.JobTypePreGate,
			hydraOverlays: map[string]*HydraJobConfig{
				"pre_gate": {Envs: map[string]string{"SHARED": "from_overlay", "NEW": "overlay_val"}},
			},
		})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		router := m["build_gate"].(map[string]any)["router"].(map[string]any)
		envs := router["envs"].(map[string]any)
		if envs["SHARED"] != "from_spec" {
			t.Errorf("router envs[SHARED] = %v, want from_spec (spec wins)", envs["SHARED"])
		}
		if envs["NEW"] != "overlay_val" {
			t.Errorf("router envs[NEW] = %v, want overlay_val", envs["NEW"])
		}
	})
}

// ---------------------------------------------------------------------------
// Healing container overlay
// ---------------------------------------------------------------------------

func TestApplyHydraOverlay_HealContainerOverlay(t *testing.T) {
	t.Parallel()

	m := map[string]any{
		"build_gate": map[string]any{
			"healing": map[string]any{
				"by_error_kind": map[string]any{
					"infra": map[string]any{
						"image": "heal:latest",
						"envs":  map[string]any{"EXISTING": "spec_val"},
					},
					"logic": map[string]any{
						"image": "heal:latest",
					},
				},
			},
		},
	}
	err := applyHydraOverlayMutator(m, claimSpecMutatorInput{
		job:     store.Job{Meta: []byte(`{}`)},
		jobType: domaintypes.JobTypeHeal,
		hydraOverlays: map[string]*HydraJobConfig{
			"heal": {
				Envs: map[string]string{"EXISTING": "overlay_val", "HEAL_KEY": "heal_val"},
				CA:   []string{"heal1234567ab"},
			},
		},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	bg := m["build_gate"].(map[string]any)
	healing := bg["healing"].(map[string]any)
	byKind := healing["by_error_kind"].(map[string]any)

	infra := byKind["infra"].(map[string]any)
	infraEnvs := infra["envs"].(map[string]any)
	if infraEnvs["EXISTING"] != "spec_val" {
		t.Errorf("infra envs[EXISTING] = %v, want spec_val (spec wins)", infraEnvs["EXISTING"])
	}
	if infraEnvs["HEAL_KEY"] != "heal_val" {
		t.Errorf("infra envs[HEAL_KEY] = %v, want heal_val", infraEnvs["HEAL_KEY"])
	}
	infraCA := infra["ca"].([]any)
	if len(infraCA) != 1 || infraCA[0] != "heal1234567ab" {
		t.Errorf("infra ca = %v, want [heal1234567ab]", infraCA)
	}

	logic := byKind["logic"].(map[string]any)
	logicEnvs := logic["envs"].(map[string]any)
	if logicEnvs["HEAL_KEY"] != "heal_val" {
		t.Errorf("logic envs[HEAL_KEY] = %v, want heal_val", logicEnvs["HEAL_KEY"])
	}
}

// ---------------------------------------------------------------------------
// Destination collision detection
// ---------------------------------------------------------------------------

func TestApplyHydraOverlay_DestinationCollision(t *testing.T) {
	t.Parallel()

	t.Run("duplicate in destinations in overlay", func(t *testing.T) {
		t.Parallel()
		m := map[string]any{}
		err := applyHydraOverlayMutator(m, claimSpecMutatorInput{
			job:     store.Job{Meta: []byte(`{}`)},
			jobType: domaintypes.JobTypeMig,
			hydraOverlays: map[string]*HydraJobConfig{
				"mig": {In: []string{"/a:/in/config.json", "/b:/in/config.json"}},
			},
		})
		if err == nil {
			t.Fatal("expected collision error")
		}
		if !strings.Contains(err.Error(), "hydra overlay collision") {
			t.Errorf("error = %v, want hydra overlay collision", err)
		}
		if !strings.Contains(err.Error(), "/in/config.json") {
			t.Errorf("error = %v, want mention of /in/config.json", err)
		}
	})

	t.Run("duplicate home destinations in overlay", func(t *testing.T) {
		t.Parallel()
		m := map[string]any{}
		err := applyHydraOverlayMutator(m, claimSpecMutatorInput{
			job:     store.Job{Meta: []byte(`{}`)},
			jobType: domaintypes.JobTypeMig,
			hydraOverlays: map[string]*HydraJobConfig{
				"mig": {Home: []string{"/a:.config/app.toml", "/b:.config/app.toml:ro"}},
			},
		})
		if err == nil {
			t.Fatal("expected collision error")
		}
		if !strings.Contains(err.Error(), ".config/app.toml") {
			t.Errorf("error = %v, want mention of .config/app.toml", err)
		}
	})

	t.Run("router overlay collision via gate phase", func(t *testing.T) {
		t.Parallel()
		// Use mig job type so the top-level overlay (mig) is clean,
		// but the router inherits from pre_gate which has collisions.
		m := map[string]any{
			"build_gate": map[string]any{
				"pre":    map[string]any{"target": "unit"},
				"router": map[string]any{"image": "router:latest"},
			},
		}
		err := applyHydraOverlayMutator(m, claimSpecMutatorInput{
			job:     store.Job{Meta: []byte(`{}`)},
			jobType: domaintypes.JobTypeMig,
			hydraOverlays: map[string]*HydraJobConfig{
				"mig":      {},
				"pre_gate": {Out: []string{"/a:/out/result.txt", "/b:/out/result.txt"}},
			},
		})
		if err == nil {
			t.Fatal("expected collision error")
		}
		if !strings.Contains(err.Error(), "build_gate.router") {
			t.Errorf("error = %v, want build_gate.router prefix", err)
		}
	})

	t.Run("no collision when spec and overlay share dst", func(t *testing.T) {
		t.Parallel()
		m := map[string]any{
			"in": []any{"/spec:/in/config.json"},
		}
		err := applyHydraOverlayMutator(m, claimSpecMutatorInput{
			job:     store.Job{Meta: []byte(`{}`)},
			jobType: domaintypes.JobTypeMig,
			hydraOverlays: map[string]*HydraJobConfig{
				"mig": {In: []string{"/overlay:/in/config.json"}},
			},
		})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		in := m["in"].([]any)
		if len(in) != 1 {
			t.Fatalf("in length = %d, want 1 (spec wins, overlay dropped)", len(in))
		}
		if in[0] != "/spec:/in/config.json" {
			t.Errorf("in[0] = %v, want /spec:/in/config.json", in[0])
		}
	})
}

// ---------------------------------------------------------------------------
// Three-layer precedence: server overlay + global env → spec
// ---------------------------------------------------------------------------

func TestApplyHydraOverlay_ThreeLayerPrecedence(t *testing.T) {
	t.Parallel()

	m := map[string]any{
		"envs": map[string]any{
			"SPEC_ONLY":  "spec",
			"SHARED_ALL": "from_spec",
		},
		"ca":   []any{"cccccc1234ab"},
		"in":   []any{"/spec/data:/in/data.json"},
		"home": []any{"/spec/auth:.auth/config.json:ro"},
	}

	err := applyHydraOverlayMutator(m, claimSpecMutatorInput{
		job:     store.Job{Meta: []byte(`{}`)},
		jobType: domaintypes.JobTypeMig,
		globalEnv: map[string][]GlobalEnvVar{
			"GLOBAL_ONLY": {{Value: "global", Target: domaintypes.GlobalEnvTargetSteps}},
			"SHARED_ALL":  {{Value: "from_global", Target: domaintypes.GlobalEnvTargetSteps}},
		},
		hydraOverlays: map[string]*HydraJobConfig{
			"mig": {
				Envs: map[string]string{
					"OVERLAY_ONLY": "overlay",
					"SHARED_ALL":   "from_overlay",
					"GLOBAL_ONLY":  "overlay_override",
				},
				CA:   []string{"aaaaaa1234ab", "cccccc1234ab"},
				In:   []string{"/overlay/extra:/in/extra.json", "/overlay/data:/in/data.json"},
				Home: []string{"/overlay/auth:.auth/config.json"},
			},
		},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	envs := m["envs"].(map[string]any)
	if envs["SHARED_ALL"] != "from_spec" {
		t.Errorf("SHARED_ALL = %v, want from_spec (spec > overlay > global)", envs["SHARED_ALL"])
	}
	if envs["SPEC_ONLY"] != "spec" {
		t.Errorf("SPEC_ONLY = %v, want spec", envs["SPEC_ONLY"])
	}
	if envs["OVERLAY_ONLY"] != "overlay" {
		t.Errorf("OVERLAY_ONLY = %v, want overlay", envs["OVERLAY_ONLY"])
	}
	// Overlay envs override global env for same key.
	if envs["GLOBAL_ONLY"] != "overlay_override" {
		t.Errorf("GLOBAL_ONLY = %v, want overlay_override (overlay > global)", envs["GLOBAL_ONLY"])
	}

	// CA: spec's cccccc (first) + overlay's aaaaaa (deduped cccccc).
	ca := m["ca"].([]any)
	if len(ca) != 2 {
		t.Fatalf("ca length = %d, want 2: %v", len(ca), ca)
	}
	if ca[0] != "cccccc1234ab" {
		t.Errorf("ca[0] = %v, want cccccc1234ab (from spec)", ca[0])
	}

	// in: spec's /in/data.json wins; overlay's /in/extra.json appended.
	in := m["in"].([]any)
	if len(in) != 2 {
		t.Fatalf("in length = %d, want 2: %v", len(in), in)
	}
	if in[0] != "/spec/data:/in/data.json" {
		t.Errorf("in[0] = %v, want /spec/data:/in/data.json (spec wins)", in[0])
	}

	// home: spec wins for .auth/config.json:ro.
	home := m["home"].([]any)
	if len(home) != 1 {
		t.Fatalf("home length = %d, want 1: %v", len(home), home)
	}
	if home[0] != "/spec/auth:.auth/config.json:ro" {
		t.Errorf("home[0] = %v, want spec entry with :ro preserved", home[0])
	}
}

// ---------------------------------------------------------------------------
// ValidateHydraSection
// ---------------------------------------------------------------------------

func TestValidateHydraSection(t *testing.T) {
	t.Parallel()

	for _, s := range []string{"pre_gate", "re_gate", "post_gate", "mig", "heal"} {
		if err := ValidateHydraSection(s); err != nil {
			t.Errorf("ValidateHydraSection(%q) = %v, want nil", s, err)
		}
	}
	for _, s := range []string{"", "unknown", "mr", "server", "node"} {
		if err := ValidateHydraSection(s); err == nil {
			t.Errorf("ValidateHydraSection(%q) = nil, want error", s)
		}
	}
}

// ---------------------------------------------------------------------------
// ConfigHolder hydra overlay accessors
// ---------------------------------------------------------------------------

func TestConfigHolder_HydraOverlays(t *testing.T) {
	t.Parallel()

	h := &ConfigHolder{}

	// Set a valid section.
	if err := h.SetHydraJobConfig("mig", &HydraJobConfig{
		Envs: map[string]string{"K": "V"},
		CA:   []string{"abc1234567ab"},
	}); err != nil {
		t.Fatalf("SetHydraJobConfig: %v", err)
	}

	// Get returns a copy.
	overlays := h.GetHydraOverlays()
	if overlays == nil || overlays["mig"] == nil {
		t.Fatal("expected mig overlay")
	}
	if overlays["mig"].Envs["K"] != "V" {
		t.Errorf("mig envs[K] = %v, want V", overlays["mig"].Envs["K"])
	}

	// Invalid section rejected.
	if err := h.SetHydraJobConfig("bogus", &HydraJobConfig{}); err == nil {
		t.Fatal("expected error for invalid section")
	}

	// Nil deletes the section.
	if err := h.SetHydraJobConfig("mig", nil); err != nil {
		t.Fatalf("SetHydraJobConfig(nil): %v", err)
	}
	overlays = h.GetHydraOverlays()
	if overlays != nil && overlays["mig"] != nil {
		t.Error("expected mig overlay to be removed")
	}
}

// ---------------------------------------------------------------------------
// Pipeline integration: full mutateClaimSpec with Hydra overlay
// ---------------------------------------------------------------------------

func TestMutateClaimSpec_HydraOverlayInPipeline(t *testing.T) {
	t.Parallel()

	jobID := domaintypes.NewJobID()
	merged, err := mutateClaimSpec(claimSpecMutatorInput{
		spec:    []byte(`{"envs":{"EXISTING":"1"}}`),
		job:     store.Job{ID: jobID, Meta: []byte(`{}`)},
		jobType: domaintypes.JobTypeMig,
		globalEnv: map[string][]GlobalEnvVar{
			"GLOBAL": {{Value: "g", Target: domaintypes.GlobalEnvTargetSteps}},
		},
		hydraOverlays: map[string]*HydraJobConfig{
			"mig": {
				CA: []string{"abc1234567ab"},
				In: []string{"/data:/in/data.json"},
			},
		},
	})
	if err != nil {
		t.Fatalf("mutateClaimSpec: %v", err)
	}

	var out map[string]any
	if err := json.Unmarshal(merged, &out); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if got := out["job_id"]; got != jobID.String() {
		t.Errorf("job_id = %v, want %s", got, jobID.String())
	}

	envs := out["envs"].(map[string]any)
	if envs["EXISTING"] != "1" {
		t.Errorf("envs[EXISTING] = %v, want 1", envs["EXISTING"])
	}
	if envs["GLOBAL"] != "g" {
		t.Errorf("envs[GLOBAL] = %v, want g", envs["GLOBAL"])
	}

	ca := out["ca"].([]any)
	if len(ca) != 1 || ca[0] != "abc1234567ab" {
		t.Errorf("ca = %v, want [abc1234567ab]", ca)
	}

	in := out["in"].([]any)
	if len(in) != 1 || in[0] != "/data:/in/data.json" {
		t.Errorf("in = %v, want [/data:/in/data.json]", in)
	}
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

func cloneSpecMap(m map[string]any) map[string]any {
	b, _ := json.Marshal(m)
	var cp map[string]any
	_ = json.Unmarshal(b, &cp)
	if cp == nil {
		cp = map[string]any{}
	}
	return cp
}

func targetTestEnv() map[string][]GlobalEnvVar {
	return map[string][]GlobalEnvVar{
		"GATES_KEY": {{Value: "gates-value", Target: domaintypes.GlobalEnvTargetGates}},
		"STEPS_KEY": {{Value: "steps-value", Target: domaintypes.GlobalEnvTargetSteps}},
	}
}
