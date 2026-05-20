package handlers

import (
	"encoding/json"
	"testing"

	domaintypes "github.com/iw2rmb/ploy/internal/domain/types"
	"github.com/iw2rmb/ploy/internal/store"
)

// ---------------------------------------------------------------------------
// Global env to envs routing (migrated from spec_utils_global_env_test.go)
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
	// Job-type to target routing: steps-target jobs get STEPS_KEY, gates-target jobs get GATES_KEY.
	for _, jt := range []struct {
		jobType    domaintypes.JobType
		expectKeys []string
		rejectKeys []string
	}{
		{domaintypes.JobTypeMig, []string{"STEPS_KEY"}, []string{"GATES_KEY"}},
		{domaintypes.JobTypeMig, []string{"STEPS_KEY"}, []string{"GATES_KEY"}},
		{domaintypes.JobTypePreGate, []string{"GATES_KEY"}, []string{"STEPS_KEY"}},
		{domaintypes.JobTypePostGate, []string{"GATES_KEY"}, []string{"STEPS_KEY"}},
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
// Typed Hydra overlay merge (envs, in, out, home)
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
			name: "in out home merge by destination",
			spec: map[string]any{
				"steps": []any{
					map[string]any{
						"image": "img:latest",
						"in":    []any{"/a.txt:/in/config.json"},
						"out":   []any{"/b.txt:/out/result.txt"},
						"home":  []any{"/c.txt:.config/app.toml:ro"},
					},
				},
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
			spec: map[string]any{
				"steps": []any{
					map[string]any{"image": "img:latest"},
				},
			},
			overlays: map[string]*HydraJobConfig{
				"mig": {Envs: map[string]string{"K": "V"}, In: []string{"/f:/in/f.txt"}},
			},
			checkEnvs: map[string]string{"K": "V"},
			slices:    []sliceCheck{{"in", 1, "/f:/in/f.txt"}},
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
			if len(tt.slices) > 0 {
				assertSlices(t, firstStepMap(t, m), tt.slices)
			}
		})
	}
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

func firstStepMap(t *testing.T, spec map[string]any) map[string]any {
	t.Helper()
	steps, ok := spec["steps"].([]any)
	if !ok || len(steps) == 0 {
		t.Fatalf("expected non-empty steps, got %T", spec["steps"])
	}
	step, ok := steps[0].(map[string]any)
	if !ok {
		t.Fatalf("expected step object, got %T", steps[0])
	}
	return step
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
