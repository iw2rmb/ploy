package handlers

import (
	"strings"
	"testing"

	domaintypes "github.com/iw2rmb/ploy/internal/domain/types"
	"github.com/iw2rmb/ploy/internal/store"
)

// ---------------------------------------------------------------------------
// Section routing
// ---------------------------------------------------------------------------

func TestApplyHydraOverlay_SectionRouting(t *testing.T) {
	t.Parallel()

	overlays := map[string]*HydraJobConfig{
		"pre_gate":  {Envs: map[string]string{"SECTION": "pre_gate"}},
		"post_gate": {Envs: map[string]string{"SECTION": "post_gate"}},
		"mig":       {Envs: map[string]string{"SECTION": "mig"}},
	}

	tests := []struct {
		jobType     domaintypes.JobType
		wantSection string
	}{
		{domaintypes.JobTypePreGate, "pre_gate"},
		{domaintypes.JobTypePostGate, "post_gate"},
		{domaintypes.JobTypeMig, "mig"},
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
		errSubstr string
		slices    []sliceCheck
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
			name: "spec and overlay share in dst replaces with spec entry",
			spec: map[string]any{
				"steps": []any{
					map[string]any{
						"image": "img:latest",
						"in":    []any{"/spec:/in/config.json"},
					},
				},
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
				"steps": []any{
					map[string]any{
						"image": "img:latest",
						"out":   []any{"/spec:/out/result.txt"},
					},
				},
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
				"steps": []any{
					map[string]any{
						"image": "img:latest",
						"home":  []any{"/spec:.config/app.toml:ro"},
					},
				},
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
				"steps": []any{
					map[string]any{
						"image": "img:latest",
						"in":    []any{"/spec:/in/a.json"},
					},
				},
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
			if len(tt.slices) > 0 {
				assertSlices(t, firstStepMap(t, m), tt.slices)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// Three-layer precedence: server overlay + global env to spec
// ---------------------------------------------------------------------------

func TestApplyHydraOverlay_ThreeLayerPrecedence(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		spec      map[string]any
		globalEnv map[string][]GlobalEnvVar
		overlays  map[string]*HydraJobConfig
		jobType   domaintypes.JobType
		checkEnvs map[string]string
		slices    []sliceCheck
	}{
		{
			name: "spec wins over overlay and global for shared env key",
			spec: map[string]any{
				"envs": map[string]any{"SHARED_ALL": "from_spec", "SPEC_ONLY": "spec"},
				"steps": []any{
					map[string]any{
						"image": "img:latest",
						"in":    []any{"/spec/data:/in/data.json"},
						"home":  []any{"/spec/auth:.auth/config.json:ro"},
					},
				},
			},
			globalEnv: map[string][]GlobalEnvVar{
				"GLOBAL_ONLY": {{Value: "global", Target: domaintypes.GlobalEnvTargetSteps}},
				"SHARED_ALL":  {{Value: "from_global", Target: domaintypes.GlobalEnvTargetSteps}},
			},
			overlays: map[string]*HydraJobConfig{
				"mig": {
					Envs: map[string]string{"OVERLAY_ONLY": "overlay", "SHARED_ALL": "from_overlay", "GLOBAL_ONLY": "overlay_override"},
					In:   []string{"/overlay/extra:/in/extra.json", "/overlay/data:/in/data.json"},
					Home: []string{"/overlay/auth:.auth/config.json"},
				},
			},
			jobType:   domaintypes.JobTypeMig,
			checkEnvs: map[string]string{"SHARED_ALL": "from_spec", "SPEC_ONLY": "spec", "OVERLAY_ONLY": "overlay", "GLOBAL_ONLY": "overlay_override"},
			slices: []sliceCheck{
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
			spec: map[string]any{
				"steps": []any{
					map[string]any{"image": "img:latest"},
				},
			},
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
			if len(tt.slices) > 0 {
				assertSlices(t, firstStepMap(t, m), tt.slices)
			}
		})
	}
}
