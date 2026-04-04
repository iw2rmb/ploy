package main

import (
	"path/filepath"
	"strings"
	"testing"
)

// ---------------------------------------------------------------------------
// buildSpecPayload: no spec / empty
// ---------------------------------------------------------------------------

func TestBuildSpecPayload_NoSpecOrEmpty(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		opts      specPayloadOpts
		wantNil   bool
		wantImage string
		wantEnv   map[string]any
	}{
		{
			name:      "no spec file, image and env from CLI",
			opts:      specPayloadOpts{migEnvs: []string{"KEY1=value1"}, migImage: "docker.io/test/mig:latest"},
			wantImage: "docker.io/test/mig:latest",
			wantEnv:   map[string]any{"KEY1": "value1"},
		},
		{
			name:    "no spec and no overrides returns nil",
			opts:    specPayloadOpts{},
			wantNil: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			payload, err := callBuildSpecPayload(t, "", tt.opts)
			if err != nil {
				t.Fatalf("buildSpecPayload: %v", err)
			}
			if tt.wantNil {
				if payload != nil {
					t.Errorf("expected nil payload, got %s", payload)
				}
				return
			}
			result := unmarshalPayload(t, payload)
			steps := mustSteps(t, result, 1)
			assertField(t, steps[0], "image", tt.wantImage)
			if tt.wantEnv != nil {
				envs := mustDig(t, result, "envs")
				for k, v := range tt.wantEnv {
					assertField(t, envs, k, v)
				}
			}
		})
	}
}

// ---------------------------------------------------------------------------
// buildSpecPayload: GitLab domain defaulting
// ---------------------------------------------------------------------------

func TestBuildSpecPayload_GitLabDomainDefaulting(t *testing.T) {
	tests := []struct {
		name          string
		specContent   string
		gitlabPAT     string
		gitlabDomain  string
		mrSuccess     bool
		mrFail        bool
		wantDomain    string
		wantDomainSet bool
		wantMRFail    bool // extra MR flag assertion
	}{
		{
			name:          "PAT provided, no domain - defaults to gitlab.com",
			gitlabPAT:     "glpat-test",
			wantDomain:    "gitlab.com",
			wantDomainSet: true,
		},
		{
			name:          "PAT and domain both provided - uses CLI domain",
			gitlabPAT:     "glpat-test",
			gitlabDomain:  "gitlab.example.com",
			wantDomain:    "gitlab.example.com",
			wantDomainSet: true,
		},
		{
			name:          "PAT in CLI, domain in spec - spec preserved",
			specContent:   "gitlab_domain: gitlab.spec.com\n",
			gitlabPAT:     "glpat-test",
			wantDomain:    "gitlab.spec.com",
			wantDomainSet: true,
		},
		{
			name:          "PAT in CLI, domain in spec - CLI overrides spec",
			specContent:   "gitlab_domain: gitlab.spec.com\n",
			gitlabPAT:     "glpat-test",
			gitlabDomain:  "gitlab.cli.com",
			wantDomain:    "gitlab.cli.com",
			wantDomainSet: true,
		},
		{
			name:          "no PAT - domain not set",
			wantDomainSet: false,
		},
		{
			name:          "PAT in spec - defaults to gitlab.com",
			specContent:   "gitlab_pat: glpat-from-spec\n",
			wantDomain:    "gitlab.com",
			wantDomainSet: true,
		},
		{
			name: "MR flags with PAT and domain defaulting",
			specContent: `
steps:
  - image: docker.io/test/mig:latest
envs:
  KEY1: value1
`,
			gitlabPAT:    "glpat-test-123",
			mrFail:       true,
			wantDomain:   "gitlab.com",
			wantDomainSet: true,
			wantMRFail:   true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var specFile string
			if tt.specContent != "" {
				tmpDir := t.TempDir()
				specFile = filepath.Join(tmpDir, "test.yaml")
				writeFile(t, specFile, tt.specContent)
			}

			migImage := ""
			if tt.gitlabPAT != "" || strings.TrimSpace(tt.specContent) != "" || tt.gitlabDomain != "" {
				migImage = "docker.io/test/mig:latest"
			}
			payload, err := callBuildSpecPayload(t, specFile, specPayloadOpts{
				migImage:     migImage,
				gitlabPAT:    tt.gitlabPAT,
				gitlabDomain: tt.gitlabDomain,
				mrSuccess:    tt.mrSuccess,
				mrFail:       tt.mrFail,
			})
			if err != nil {
				t.Fatalf("buildSpecPayload: %v", err)
			}

			if payload == nil && !tt.wantDomainSet {
				return
			}

			result := unmarshalPayload(t, payload)

			if tt.wantDomainSet {
				assertField(t, result, "gitlab_domain", tt.wantDomain)
			} else {
				assertAbsent(t, result, "gitlab_domain")
			}

			if tt.wantMRFail {
				assertField(t, result, "mr_on_fail", true)
				// mr_on_success should be false/absent when not set.
				if v, ok := result["mr_on_success"].(bool); ok && v {
					t.Errorf("expected mr_on_success=false or absent, got true")
				}
				assertField(t, result, "gitlab_pat", tt.gitlabPAT)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// buildSpecPayload: config overlay
// ---------------------------------------------------------------------------

func TestBuildSpecPayload_ConfigOverlay(t *testing.T) {
	tests := []struct {
		name       string
		config     string // config.yaml content; empty → no config file
		spec       string
		wantEnvs   map[string]any
	}{
		{
			name: "overlay < spec precedence",
			config: `
defaults:
  job:
    mig:
      envs:
        FROM_OVERLAY: overlay_val
        SHARED: from_overlay
`,
			spec: `
steps:
  - image: docker.io/test/mig:latest
envs:
  FROM_SPEC: spec_val
  SHARED: from_spec
`,
			wantEnvs: map[string]any{
				"SHARED":       "from_spec",
				"FROM_OVERLAY": "overlay_val",
				"FROM_SPEC":    "spec_val",
			},
		},
		{
			name: "missing config.yaml is fine",
			spec: `
steps:
  - image: docker.io/test/mig:latest
envs:
  KEY: value
`,
			wantEnvs: map[string]any{"KEY": "value"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			configHome := t.TempDir()
			t.Setenv("PLOY_CONFIG_HOME", configHome)
			if tt.config != "" {
				writeFile(t, filepath.Join(configHome, "config.yaml"), tt.config)
			}

			result := runBuildSpecPayload(t, tt.spec, ".yaml", specPayloadOpts{})
			envs := mustDig(t, result, "envs")
			for k, v := range tt.wantEnvs {
				assertField(t, envs, k, v)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// applyConfigOverlayInPlace: phase routing for router/healing
// ---------------------------------------------------------------------------

func TestApplyConfigOverlayInPlace_PhaseRouting(t *testing.T) {
	tests := []struct {
		name    string
		config  string
		spec    map[string]any
		digPath []string
		wantEnv map[string]any
	}{
		{
			name: "router inherits pre_gate overlay",
			config: `
defaults:
  job:
    pre_gate:
      envs:
        GATE_KEY: gate_val
`,
			spec: map[string]any{
				"steps": []any{
					map[string]any{"image": "docker.io/test/mig:latest"},
				},
				"build_gate": map[string]any{
					"router": map[string]any{
						"image": "docker.io/test/router:latest",
					},
				},
			},
			digPath: []string{"build_gate", "router"},
			wantEnv: map[string]any{"GATE_KEY": "gate_val"},
		},
		{
			name: "healing gets heal overlay",
			config: `
defaults:
  job:
    heal:
      envs:
        HEAL_KEY: heal_val
`,
			spec: map[string]any{
				"steps": []any{
					map[string]any{"image": "docker.io/test/mig:latest"},
				},
				"build_gate": map[string]any{
					"healing": map[string]any{
						"by_error_kind": map[string]any{
							"infra": map[string]any{
								"image": "docker.io/test/healer:latest",
							},
						},
					},
				},
			},
			digPath: []string{"build_gate", "healing", "by_error_kind", "infra"},
			wantEnv: map[string]any{"HEAL_KEY": "heal_val"},
		},
		{
			name: "router derives active gate phase from spec",
			config: `
defaults:
  job:
    pre_gate:
      envs:
        PHASE_KEY: pre_gate_val
    post_gate:
      envs:
        PHASE_KEY: post_gate_val
`,
			spec: map[string]any{
				"steps": []any{
					map[string]any{"image": "docker.io/test/mig:latest"},
				},
				"build_gate": map[string]any{
					"post": map[string]any{"target": "unit"},
					"router": map[string]any{
						"image": "docker.io/test/router:latest",
					},
				},
			},
			digPath: []string{"build_gate", "router"},
			wantEnv: map[string]any{"PHASE_KEY": "post_gate_val"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			configHome := t.TempDir()
			t.Setenv("PLOY_CONFIG_HOME", configHome)
			writeFile(t, filepath.Join(configHome, "config.yaml"), tt.config)

			if err := applyConfigOverlayInPlace(tt.spec); err != nil {
				t.Fatalf("applyConfigOverlayInPlace: %v", err)
			}

			target := mustDig(t, tt.spec, tt.digPath...)
			envs := mustDig(t, target, "envs")
			for k, v := range tt.wantEnv {
				assertField(t, envs, k, v)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// deriveActiveGatePhase
// ---------------------------------------------------------------------------

func TestDeriveActiveGatePhase(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		spec map[string]any
		want string
	}{
		{name: "no build_gate", spec: map[string]any{}, want: "pre_gate"},
		{name: "pre configured", spec: map[string]any{"build_gate": map[string]any{"pre": map[string]any{"target": "build"}}}, want: "pre_gate"},
		{name: "only re", spec: map[string]any{"build_gate": map[string]any{"re": map[string]any{"target": "build"}}}, want: "re_gate"},
		{name: "only post", spec: map[string]any{"build_gate": map[string]any{"post": map[string]any{"target": "unit"}}}, want: "post_gate"},
		{name: "pre and re", spec: map[string]any{"build_gate": map[string]any{"pre": map[string]any{"target": "build"}, "re": map[string]any{"target": "build"}}}, want: "pre_gate"},
		{name: "re and post", spec: map[string]any{"build_gate": map[string]any{"re": map[string]any{"target": "build"}, "post": map[string]any{"target": "unit"}}}, want: "re_gate"},
		{name: "pre and post", spec: map[string]any{"build_gate": map[string]any{"pre": map[string]any{"target": "build"}, "post": map[string]any{"target": "unit"}}}, want: "pre_gate"},
		{name: "no pre/re/post", spec: map[string]any{"build_gate": map[string]any{"enabled": true}}, want: "pre_gate"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := deriveActiveGatePhase(tt.spec); got != tt.want {
				t.Errorf("deriveActiveGatePhase() = %q, want %q", got, tt.want)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// buildSpecPayload: deterministic canonical output
// ---------------------------------------------------------------------------

func TestBuildSpecPayload_DeterministicCanonicalSnapshot(t *testing.T) {
	configHome := t.TempDir()
	t.Setenv("PLOY_CONFIG_HOME", configHome)

	writeFile(t, filepath.Join(configHome, "config.yaml"), `
defaults:
  job:
    mig:
      envs:
        FROM_OVERLAY: overlay_val
        Z_KEY: z_val
        A_KEY: a_val
`)

	specDir := t.TempDir()
	specPath := filepath.Join(specDir, "test.yaml")
	writeFile(t, specPath, `
steps:
  - image: docker.io/test/mig:latest
envs:
  FROM_SPEC: spec_val
  M_KEY: m_val
`)

	var payloads [2][]byte
	for i := range payloads {
		payload, err := callBuildSpecPayload(t, specPath, specPayloadOpts{})
		if err != nil {
			t.Fatalf("run %d: %v", i, err)
		}
		payloads[i] = payload
	}

	if string(payloads[0]) != string(payloads[1]) {
		t.Errorf("non-deterministic output:\nrun 0: %s\nrun 1: %s", payloads[0], payloads[1])
	}

	result := unmarshalPayload(t, payloads[0])
	envs := mustDig(t, result, "envs")
	for k, v := range map[string]any{
		"FROM_SPEC":    "spec_val",
		"FROM_OVERLAY": "overlay_val",
		"Z_KEY":        "z_val",
		"A_KEY":        "a_val",
		"M_KEY":        "m_val",
	} {
		assertField(t, envs, k, v)
	}

	steps := mustSteps(t, result, 1)
	assertField(t, steps[0], "image", "docker.io/test/mig:latest")
}

// ---------------------------------------------------------------------------
// buildSpecPayload: three-layer merge (server < local < spec)
// ---------------------------------------------------------------------------

func TestBuildSpecPayload_ServerLocalSpecMergePrecedence(t *testing.T) {
	configHome := t.TempDir()
	t.Setenv("PLOY_CONFIG_HOME", configHome)

	writeFile(t, filepath.Join(configHome, "config.yaml"), `
defaults:
  job:
    mig:
      envs:
        LOCAL_KEY: local_val
        SHARED: from_local
    heal:
      envs:
        HEAL_LOCAL: heal_local_val
    pre_gate:
      envs:
        GATE_LOCAL: gate_local_val
`)

	result := runBuildSpecPayload(t, `
steps:
  - image: docker.io/test/mig:latest
    envs:
      STEP_SHARED: from_spec
envs:
  SPEC_KEY: spec_val
  SHARED: from_spec
build_gate:
  pre:
    target: build
  router:
    image: docker.io/test/router:latest
  healing:
    by_error_kind:
      infra:
        image: docker.io/test/healer:latest
`, ".yaml", specPayloadOpts{})

	// Top-level envs: spec wins for SHARED, local key preserved.
	envs := mustDig(t, result, "envs")
	assertField(t, envs, "SHARED", "from_spec")
	assertField(t, envs, "LOCAL_KEY", "local_val")
	assertField(t, envs, "SPEC_KEY", "spec_val")

	// Step-level envs preserved.
	steps := mustSteps(t, result, 1)
	stepEnvs := mustDig(t, steps[0], "envs")
	assertField(t, stepEnvs, "STEP_SHARED", "from_spec")

	// Router: pre_gate overlay applied.
	routerEnvs := mustDig(t, result, "build_gate", "router", "envs")
	assertField(t, routerEnvs, "GATE_LOCAL", "gate_local_val")

	// Healing: heal overlay applied.
	healEnvs := mustDig(t, result, "build_gate", "healing", "by_error_kind", "infra", "envs")
	assertField(t, healEnvs, "HEAL_LOCAL", "heal_local_val")
}
