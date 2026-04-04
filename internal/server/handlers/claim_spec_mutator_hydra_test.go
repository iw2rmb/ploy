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
	}
	// Job-type → target routing: steps-target jobs get STEPS_KEY, gates-target jobs get GATES_KEY.
	for _, jt := range []struct {
		jobType    domaintypes.JobType
		expectKeys []string
		rejectKeys []string
	}{
		{domaintypes.JobTypeMig, []string{"STEPS_KEY"}, []string{"GATES_KEY"}},
		{domaintypes.JobTypeHeal, []string{"STEPS_KEY"}, []string{"GATES_KEY"}},
		{domaintypes.JobTypePreGate, []string{"GATES_KEY"}, []string{"STEPS_KEY"}},
		{domaintypes.JobTypeReGate, []string{"GATES_KEY"}, []string{"STEPS_KEY"}},
		{domaintypes.JobTypePostGate, []string{"GATES_KEY"}, []string{"STEPS_KEY"}},
	} {
		tests = append(tests, struct {
			name       string
			spec       map[string]any
			env        map[string][]GlobalEnvVar
			jobType    domaintypes.JobType
			expectKeys []string
			rejectKeys []string
			checkEnvs  map[string]string
		}{
			name:       string(jt.jobType) + " job gets correct target",
			spec:       map[string]any{},
			env:        targetTestEnv(),
			jobType:    jt.jobType,
			expectKeys: jt.expectKeys,
			rejectKeys: jt.rejectKeys,
		})
	}
	tests = append(tests, []struct {
		name       string
		spec       map[string]any
		env        map[string][]GlobalEnvVar
		jobType    domaintypes.JobType
		expectKeys []string
		rejectKeys []string
		checkEnvs  map[string]string
	}{
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
			env:     map[string][]GlobalEnvVar{"CUSTOM_CERT_DATA": {{Value: "-----BEGIN CERT-----\n...", Target: domaintypes.GlobalEnvTargetSteps, Secret: true}}},
			jobType: domaintypes.JobTypeMig,
			checkEnvs: map[string]string{
				"EXISTING":         "yes",
				"CUSTOM_CERT_DATA": "-----BEGIN CERT-----\n...",
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
	}...)

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
			assertEnvs(t, m, tt.checkEnvs, tt.expectKeys, tt.rejectKeys)
		})
	}
}

// ---------------------------------------------------------------------------
// Typed Hydra overlay merge (envs, ca, in, out, home)
// ---------------------------------------------------------------------------

func TestApplyHydraOverlay_TypedMerge(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		spec      map[string]any
		overlays  map[string]*HydraJobConfig
		checkEnvs map[string]string
		slices    []sliceCheck
	}{
		{
			name: "envs key-based override spec wins",
			spec: map[string]any{
				"envs": map[string]any{"SPEC_KEY": "spec_val", "SHARED": "from_spec"},
			},
			overlays: map[string]*HydraJobConfig{
				"mig": {Envs: map[string]string{"OVERLAY_KEY": "overlay_val", "SHARED": "from_overlay"}},
			},
			checkEnvs: map[string]string{"SPEC_KEY": "spec_val", "OVERLAY_KEY": "overlay_val", "SHARED": "from_spec"},
		},
		{
			name: "ca append with dedup",
			spec: map[string]any{
				"ca": []any{"abcdef1234ab", "/ca/extra.pem"},
			},
			overlays: map[string]*HydraJobConfig{
				"mig": {CA: []string{"abcdef1234ab", "/ca/new.pem"}},
			},
			slices: []sliceCheck{{"ca", 3, ""}},
		},
		{
			name: "in out home merge by destination",
			spec: map[string]any{
				"in":   []any{"/a.txt:/in/config.json"},
				"out":  []any{"/b.txt:/out/result.txt"},
				"home": []any{"/c.txt:.config/app.toml:ro"},
			},
			overlays: map[string]*HydraJobConfig{
				"mig": {
					In:   []string{"/overlay.txt:/in/config.json", "/overlay2.txt:/in/extra.json"},
					Out:  []string{"/overlay.txt:/out/new.txt"},
					Home: []string{"/overlay.txt:.config/app.toml", "/overlay.txt:.config/other.toml"},
				},
			},
			slices: []sliceCheck{
				{"in", 2, "/a.txt:/in/config.json"},
				{"out", 2, ""},
				{"home", 2, "/c.txt:.config/app.toml:ro"},
			},
		},
		{
			name: "empty spec block gets overlay fields",
			spec: map[string]any{},
			overlays: map[string]*HydraJobConfig{
				"mig": {Envs: map[string]string{"K": "V"}, CA: []string{"abc1234567ab"}, In: []string{"/f:/in/f.txt"}},
			},
			checkEnvs: map[string]string{"K": "V"},
			slices:    []sliceCheck{{"ca", 1, ""}},
		},
		{
			name:      "nil overlay does nothing",
			spec:      map[string]any{"envs": map[string]any{"K": "V"}},
			checkEnvs: map[string]string{"K": "V"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			m := cloneSpecMap(tt.spec)
			err := applyHydraOverlayMutator(m, claimSpecMutatorInput{
				job:           store.Job{Meta: []byte(`{}`)},
				jobType:       domaintypes.JobTypeMig,
				hydraOverlays: tt.overlays,
			})
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			assertEnvs(t, m, tt.checkEnvs, nil, nil)
			assertSlices(t, m, tt.slices)
		})
	}
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

	allOverlays := map[string]*HydraJobConfig{
		"pre_gate":  {Envs: map[string]string{"PHASE": "pre"}, CA: []string{"aaa1234567ab"}},
		"re_gate":   {Envs: map[string]string{"PHASE": "re"}, CA: []string{"bbb1234567ab"}},
		"post_gate": {Envs: map[string]string{"PHASE": "post"}, CA: []string{"ccc1234567ab"}},
	}

	tests := []struct {
		name          string
		spec          map[string]any
		jobType       domaintypes.JobType
		overlays      map[string]*HydraJobConfig
		noRouter      bool              // expect no router block created
		checkRouterKV map[string]string  // expected router envs key→value
		routerSlices  []sliceCheck       // slice checks on router sub-block
	}{
		{
			name: "pre_gate claim inherits from pre_gate",
			spec: map[string]any{
				"build_gate": map[string]any{
					"pre":    map[string]any{"target": "unit"},
					"router": map[string]any{"image": "router:latest"},
				},
			},
			jobType:       domaintypes.JobTypePreGate,
			overlays:      allOverlays,
			checkRouterKV: map[string]string{"PHASE": "pre"},
			routerSlices:  []sliceCheck{{"ca", 1, "aaa1234567ab"}},
		},
		{
			name: "re_gate claim inherits from re_gate not pre_gate",
			spec: map[string]any{
				"build_gate": map[string]any{
					"pre":    map[string]any{"target": "unit"},
					"router": map[string]any{"image": "router:latest"},
				},
			},
			jobType:       domaintypes.JobTypeReGate,
			overlays:      allOverlays,
			checkRouterKV: map[string]string{"PHASE": "re"},
			routerSlices:  []sliceCheck{{"ca", 1, "bbb1234567ab"}},
		},
		{
			name: "post_gate claim inherits from post_gate even when pre exists",
			spec: map[string]any{
				"build_gate": map[string]any{
					"pre":    map[string]any{"target": "unit"},
					"post":   map[string]any{"target": "build"},
					"router": map[string]any{"image": "router:latest"},
				},
			},
			jobType:       domaintypes.JobTypePostGate,
			overlays:      allOverlays,
			checkRouterKV: map[string]string{"PHASE": "post"},
			routerSlices:  []sliceCheck{{"ca", 1, "ccc1234567ab"}},
		},
		{
			name: "mig claim falls back to spec presence pre_gate",
			spec: map[string]any{
				"build_gate": map[string]any{
					"pre":    map[string]any{"target": "unit"},
					"router": map[string]any{"image": "router:latest"},
				},
			},
			jobType:       domaintypes.JobTypeMig,
			overlays:      allOverlays,
			checkRouterKV: map[string]string{"PHASE": "pre"},
			routerSlices:  []sliceCheck{{"ca", 1, "aaa1234567ab"}},
		},
		{
			name: "mig claim falls back to post_gate when only post configured",
			spec: map[string]any{
				"build_gate": map[string]any{
					"post":   map[string]any{"target": "build"},
					"router": map[string]any{"image": "router:latest"},
				},
			},
			jobType:       domaintypes.JobTypeMig,
			overlays:      allOverlays,
			checkRouterKV: map[string]string{"PHASE": "post"},
			routerSlices:  []sliceCheck{{"ca", 1, "ccc1234567ab"}},
		},
		{
			name: "no router section does nothing",
			spec: map[string]any{
				"build_gate": map[string]any{
					"pre": map[string]any{"target": "unit"},
				},
			},
			jobType:  domaintypes.JobTypePreGate,
			overlays: allOverlays,
			noRouter: true,
		},
		{
			name: "router spec envs win over overlay envs",
			spec: map[string]any{
				"build_gate": map[string]any{
					"pre":    map[string]any{"target": "unit"},
					"router": map[string]any{"image": "router:latest", "envs": map[string]any{"SHARED": "from_spec"}},
				},
			},
			jobType: domaintypes.JobTypePreGate,
			overlays: map[string]*HydraJobConfig{
				"pre_gate": {Envs: map[string]string{"SHARED": "from_overlay", "NEW": "overlay_val"}},
			},
			checkRouterKV: map[string]string{"SHARED": "from_spec", "NEW": "overlay_val"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			m := cloneSpecMap(tt.spec)
			err := applyHydraOverlayMutator(m, claimSpecMutatorInput{
				job:           store.Job{Meta: []byte(`{}`)},
				jobType:       tt.jobType,
				hydraOverlays: tt.overlays,
			})
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			bg := m["build_gate"].(map[string]any)
			if tt.noRouter {
				if _, ok := bg["router"]; ok {
					t.Error("router section should not be created when absent")
				}
				return
			}
			router := bg["router"].(map[string]any)
			assertEnvs(t, router, tt.checkRouterKV, nil, nil)
			assertSlices(t, router, tt.routerSlices)
		})
	}
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

	byKind := m["build_gate"].(map[string]any)["healing"].(map[string]any)["by_error_kind"].(map[string]any)

	infra := byKind["infra"].(map[string]any)
	assertEnvs(t, infra, map[string]string{"EXISTING": "spec_val", "HEAL_KEY": "heal_val"}, nil, nil)
	assertSlice(t, infra, "ca", 1, "heal1234567ab")

	logic := byKind["logic"].(map[string]any)
	assertEnvs(t, logic, map[string]string{"HEAL_KEY": "heal_val"}, nil, nil)
}

// ---------------------------------------------------------------------------
// Destination collision detection
// ---------------------------------------------------------------------------

func TestApplyHydraOverlay_DestinationCollision(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		spec      map[string]any
		jobType   domaintypes.JobType
		overlays  map[string]*HydraJobConfig
		wantErr   bool
		errSubstr string // required substring in error message
		slices    []sliceCheck // non-error: verify replacement behavior
	}{
		{
			name:    "duplicate in destinations in overlay",
			spec:    map[string]any{},
			jobType: domaintypes.JobTypeMig,
			overlays: map[string]*HydraJobConfig{
				"mig": {In: []string{"/a:/in/config.json", "/b:/in/config.json"}},
			},
			wantErr:   true,
			errSubstr: "/in/config.json",
		},
		{
			name:    "duplicate out destinations in overlay",
			spec:    map[string]any{},
			jobType: domaintypes.JobTypeMig,
			overlays: map[string]*HydraJobConfig{
				"mig": {Out: []string{"/a:/out/result.txt", "/b:/out/result.txt"}},
			},
			wantErr:   true,
			errSubstr: "/out/result.txt",
		},
		{
			name:    "duplicate home destinations in overlay",
			spec:    map[string]any{},
			jobType: domaintypes.JobTypeMig,
			overlays: map[string]*HydraJobConfig{
				"mig": {Home: []string{"/a:.config/app.toml", "/b:.config/app.toml:ro"}},
			},
			wantErr:   true,
			errSubstr: ".config/app.toml",
		},
		{
			name: "router overlay collision via gate phase",
			spec: map[string]any{
				"build_gate": map[string]any{
					"pre":    map[string]any{"target": "unit"},
					"router": map[string]any{"image": "router:latest"},
				},
			},
			jobType: domaintypes.JobTypeMig,
			overlays: map[string]*HydraJobConfig{
				"mig":      {},
				"pre_gate": {Out: []string{"/a:/out/result.txt", "/b:/out/result.txt"}},
			},
			wantErr:   true,
			errSubstr: "build_gate.router",
		},
		{
			name: "heal overlay collision detected via non-heal job",
			spec: map[string]any{
				"build_gate": map[string]any{
					"healing": map[string]any{
						"by_error_kind": map[string]any{
							"infra": map[string]any{"image": "heal:latest"},
						},
					},
				},
			},
			jobType: domaintypes.JobTypeMig,
			overlays: map[string]*HydraJobConfig{
				"mig":  {},
				"heal": {In: []string{"/a:/in/data.json", "/b:/in/data.json"}},
			},
			wantErr:   true,
			errSubstr: "build_gate.healing",
		},
		{
			name: "spec and overlay share in dst replaces with spec entry",
			spec: map[string]any{
				"in": []any{"/spec:/in/config.json"},
			},
			jobType: domaintypes.JobTypeMig,
			overlays: map[string]*HydraJobConfig{
				"mig": {In: []string{"/overlay:/in/config.json"}},
			},
			slices: []sliceCheck{{"in", 1, "/spec:/in/config.json"}},
		},
		{
			name: "spec and overlay share out dst replaces with spec entry",
			spec: map[string]any{
				"out": []any{"/spec:/out/result.txt"},
			},
			jobType: domaintypes.JobTypeMig,
			overlays: map[string]*HydraJobConfig{
				"mig": {Out: []string{"/overlay:/out/result.txt"}},
			},
			slices: []sliceCheck{{"out", 1, "/spec:/out/result.txt"}},
		},
		{
			name: "spec and overlay share home dst replaces with spec entry",
			spec: map[string]any{
				"home": []any{"/spec:.config/app.toml:ro"},
			},
			jobType: domaintypes.JobTypeMig,
			overlays: map[string]*HydraJobConfig{
				"mig": {Home: []string{"/overlay:.config/app.toml"}},
			},
			slices: []sliceCheck{{"home", 1, "/spec:.config/app.toml:ro"}},
		},
		{
			name: "overlay appends non-colliding dst to spec",
			spec: map[string]any{
				"in": []any{"/spec:/in/a.json"},
			},
			jobType: domaintypes.JobTypeMig,
			overlays: map[string]*HydraJobConfig{
				"mig": {In: []string{"/overlay:/in/b.json"}},
			},
			slices: []sliceCheck{{"in", 2, "/spec:/in/a.json"}},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			m := cloneSpecMap(tt.spec)
			err := applyHydraOverlayMutator(m, claimSpecMutatorInput{
				job:           store.Job{Meta: []byte(`{}`)},
				jobType:       tt.jobType,
				hydraOverlays: tt.overlays,
			})
			if tt.wantErr {
				if err == nil {
					t.Fatal("expected collision error")
				}
				if !strings.Contains(err.Error(), "hydra overlay collision") {
					t.Errorf("error = %v, want hydra overlay collision", err)
				}
				if tt.errSubstr != "" && !strings.Contains(err.Error(), tt.errSubstr) {
					t.Errorf("error = %v, want substring %q", err, tt.errSubstr)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			assertSlices(t, m, tt.slices)
		})
	}
}

// ---------------------------------------------------------------------------
// Three-layer precedence: server overlay + global env → spec (table-driven)
// ---------------------------------------------------------------------------

func TestApplyHydraOverlay_ThreeLayerPrecedence(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		spec      map[string]any
		globalEnv map[string][]GlobalEnvVar
		overlays  map[string]*HydraJobConfig
		jobType   domaintypes.JobType
		// assertions
		checkEnvs map[string]string // key → expected value
		slices    []sliceCheck
	}{
		{
			name: "spec wins over overlay and global for shared env key",
			spec: map[string]any{
				"envs": map[string]any{"SHARED_ALL": "from_spec", "SPEC_ONLY": "spec"},
				"ca":   []any{"cccccc1234ab"},
				"in":   []any{"/spec/data:/in/data.json"},
				"home": []any{"/spec/auth:.auth/config.json:ro"},
			},
			globalEnv: map[string][]GlobalEnvVar{
				"GLOBAL_ONLY": {{Value: "global", Target: domaintypes.GlobalEnvTargetSteps}},
				"SHARED_ALL":  {{Value: "from_global", Target: domaintypes.GlobalEnvTargetSteps}},
			},
			overlays: map[string]*HydraJobConfig{
				"mig": {
					Envs: map[string]string{"OVERLAY_ONLY": "overlay", "SHARED_ALL": "from_overlay", "GLOBAL_ONLY": "overlay_override"},
					CA:   []string{"aaaaaa1234ab", "cccccc1234ab"},
					In:   []string{"/overlay/extra:/in/extra.json", "/overlay/data:/in/data.json"},
					Home: []string{"/overlay/auth:.auth/config.json"},
				},
			},
			jobType:   domaintypes.JobTypeMig,
			checkEnvs: map[string]string{"SHARED_ALL": "from_spec", "SPEC_ONLY": "spec", "OVERLAY_ONLY": "overlay", "GLOBAL_ONLY": "overlay_override"},
			slices: []sliceCheck{
				{"ca", 2, "cccccc1234ab"},
				{"in", 2, "/spec/data:/in/data.json"},
				{"home", 1, "/spec/auth:.auth/config.json:ro"},
			},
		},
		{
			name: "overlay wins over global for same env key",
			spec: map[string]any{},
			globalEnv: map[string][]GlobalEnvVar{
				"KEY": {{Value: "global", Target: domaintypes.GlobalEnvTargetSteps}},
			},
			overlays: map[string]*HydraJobConfig{
				"mig": {Envs: map[string]string{"KEY": "overlay"}},
			},
			jobType:   domaintypes.JobTypeMig,
			checkEnvs: map[string]string{"KEY": "overlay"},
		},
		{
			name: "global fills when overlay has no envs",
			spec: map[string]any{},
			globalEnv: map[string][]GlobalEnvVar{
				"ONLY_GLOBAL": {{Value: "g", Target: domaintypes.GlobalEnvTargetSteps}},
			},
			overlays: map[string]*HydraJobConfig{
				"mig": {In: []string{"/f:/in/f.txt"}},
			},
			jobType:   domaintypes.JobTypeMig,
			checkEnvs: map[string]string{"ONLY_GLOBAL": "g"},
			slices:    []sliceCheck{{"in", 1, "/f:/in/f.txt"}},
		},
		{
			name: "spec envs win when no overlay or global conflict",
			spec: map[string]any{
				"envs": map[string]any{"A": "spec_a"},
			},
			overlays: map[string]*HydraJobConfig{
				"mig": {Envs: map[string]string{"B": "overlay_b"}},
			},
			jobType:   domaintypes.JobTypeMig,
			checkEnvs: map[string]string{"A": "spec_a", "B": "overlay_b"},
		},
		{
			name: "gate job three-layer with nodes fallback",
			spec: map[string]any{
				"envs": map[string]any{"SPEC": "s"},
			},
			globalEnv: map[string][]GlobalEnvVar{
				"NODES_KEY":  {{Value: "nodes", Target: domaintypes.GlobalEnvTargetNodes}},
				"GATES_KEY":  {{Value: "gates", Target: domaintypes.GlobalEnvTargetGates}},
				"NODES_KEY2": {{Value: "nodes2", Target: domaintypes.GlobalEnvTargetNodes}},
			},
			overlays: map[string]*HydraJobConfig{
				"pre_gate": {Envs: map[string]string{"OVERLAY": "o"}},
			},
			jobType:   domaintypes.JobTypePreGate,
			checkEnvs: map[string]string{"SPEC": "s", "OVERLAY": "o", "GATES_KEY": "gates", "NODES_KEY": "nodes", "NODES_KEY2": "nodes2"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			m := cloneSpecMap(tt.spec)
			err := applyHydraOverlayMutator(m, claimSpecMutatorInput{
				job:           store.Job{Meta: []byte(`{}`)},
				jobType:       tt.jobType,
				globalEnv:     tt.globalEnv,
				hydraOverlays: tt.overlays,
			})
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			assertEnvs(t, m, tt.checkEnvs, nil, nil)
			assertSlices(t, m, tt.slices)
		})
	}
}

// ---------------------------------------------------------------------------
// hydraExtractDst — first-colon split aligned with Hydra parser semantics
// ---------------------------------------------------------------------------

func TestHydraExtractDst_FirstColonSplit(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		field string
		entry string
		want  string
	}{
		{
			name:  "in simple",
			field: "in",
			entry: "abcdef0:/in/data.json",
			want:  "/in/data.json",
		},
		{
			name:  "out simple",
			field: "out",
			entry: "abcdef0:/out/results",
			want:  "/out/results",
		},
		{
			name:  "in with colon in destination",
			field: "in",
			entry: "abcdef0:/in/some:path",
			want:  "/in/some:path",
		},
		{
			name:  "out with colon in destination",
			field: "out",
			entry: "abcdef0:/out/some:path",
			want:  "/out/some:path",
		},
		{
			name:  "home simple rw",
			field: "home",
			entry: "abcdef0:.config/app",
			want:  ".config/app",
		},
		{
			name:  "home simple ro",
			field: "home",
			entry: "abcdef0:.config/app:ro",
			want:  ".config/app",
		},
		{
			name:  "home with colon in destination",
			field: "home",
			entry: "abcdef0:.config/some:dir",
			want:  ".config/some:dir",
		},
		{
			name:  "home double slash cleaned",
			field: "home",
			entry: "abcdef0:.config//app",
			want:  ".config/app",
		},
		{
			name:  "no colon returns full entry for in",
			field: "in",
			entry: "nocolon",
			want:  "nocolon",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := hydraExtractDst(tc.field, tc.entry)
			if got != tc.want {
				t.Errorf("hydraExtractDst(%q, %q) = %q, want %q", tc.field, tc.entry, got, tc.want)
			}
		})
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
	out := mustMutateAndUnmarshal(t, claimSpecMutatorInput{
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

	if got := out["job_id"]; got != jobID.String() {
		t.Errorf("job_id = %v, want %s", got, jobID.String())
	}
	assertEnvs(t, out, map[string]string{"EXISTING": "1", "GLOBAL": "g"}, nil, nil)
	assertSlices(t, out, []sliceCheck{
		{"ca", 1, "abc1234567ab"},
		{"in", 1, "/data:/in/data.json"},
	})
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

type sliceCheck struct {
	field     string
	wantLen   int
	wantFirst string
}

func assertSlice(t *testing.T, m map[string]any, field string, wantLen int, wantFirst string) {
	t.Helper()
	raw := m[field].([]any)
	if len(raw) != wantLen {
		t.Fatalf("%s length = %d, want %d: %v", field, len(raw), wantLen, raw)
	}
	if wantFirst != "" && raw[0] != wantFirst {
		t.Errorf("%s[0] = %v, want %s", field, raw[0], wantFirst)
	}
}

func assertSlices(t *testing.T, m map[string]any, checks []sliceCheck) {
	t.Helper()
	for _, sc := range checks {
		assertSlice(t, m, sc.field, sc.wantLen, sc.wantFirst)
	}
}

func assertEnvs(t *testing.T, m map[string]any, checkEnvs map[string]string, expectKeys, rejectKeys []string) {
	t.Helper()
	em, _ := m["envs"].(map[string]any)
	for _, key := range expectKeys {
		if _, ok := em[key]; !ok {
			t.Errorf("expected key %q to be present in envs", key)
		}
	}
	for _, key := range rejectKeys {
		if em != nil {
			if _, ok := em[key]; ok {
				t.Errorf("expected key %q to be absent from envs", key)
			}
		}
	}
	for key, want := range checkEnvs {
		if em == nil {
			t.Fatalf("envs map is nil, expected key %q=%q", key, want)
		}
		if got := em[key]; got != want {
			t.Errorf("envs[%q] = %v, want %q", key, got, want)
		}
	}
}

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
