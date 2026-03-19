package contracts

import (
	"strings"
	"testing"
)

// TestParseModsSpecJSON_RouterAmata covers amata parsing and validation for router.
func TestParseModsSpecJSON_RouterAmata(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		wantErr string
		check   func(t *testing.T, spec *ModsSpec)
	}{
		{
			name: "router amata with spec and set",
			input: `{
				"steps": [{"image": "test:latest"}],
				"build_gate": {
					"router": {
						"image": "router:latest",
						"amata": {
							"spec": "version: 1\nprompt: fix it",
							"set": [
								{"param": "REPO", "value": "my-repo"},
								{"param": "BRANCH", "value": "main"}
							]
						}
					}
				}
			}`,
			check: func(t *testing.T, spec *ModsSpec) {
				r := spec.BuildGate.Router
				if r.Amata == nil {
					t.Fatal("router.amata is nil")
				}
				if r.Amata.Spec != "version: 1\nprompt: fix it" {
					t.Errorf("router.amata.spec = %q, want %q", r.Amata.Spec, "version: 1\nprompt: fix it")
				}
				if len(r.Amata.Set) != 2 {
					t.Fatalf("router.amata.set len = %d, want 2", len(r.Amata.Set))
				}
				if r.Amata.Set[0].Param != "REPO" || r.Amata.Set[0].Value != "my-repo" {
					t.Errorf("router.amata.set[0] = %+v, want {REPO my-repo}", r.Amata.Set[0])
				}
				if r.Amata.Set[1].Param != "BRANCH" || r.Amata.Set[1].Value != "main" {
					t.Errorf("router.amata.set[1] = %+v, want {BRANCH main}", r.Amata.Set[1])
				}
			},
		},
		{
			name: "router amata with spec only (no set)",
			input: `{
				"steps": [{"image": "test:latest"}],
				"build_gate": {
					"router": {
						"image": "router:latest",
						"amata": {"spec": "prompt: fix"}
					}
				}
			}`,
			check: func(t *testing.T, spec *ModsSpec) {
				r := spec.BuildGate.Router
				if r.Amata == nil {
					t.Fatal("router.amata is nil")
				}
				if len(r.Amata.Set) != 0 {
					t.Errorf("router.amata.set len = %d, want 0", len(r.Amata.Set))
				}
			},
		},
		{
			name: "router without amata (direct codex mode)",
			input: `{
				"steps": [{"image": "test:latest"}],
				"build_gate": {
					"router": {"image": "router:latest"}
				}
			}`,
			check: func(t *testing.T, spec *ModsSpec) {
				if spec.BuildGate.Router.Amata != nil {
					t.Errorf("router.amata = %+v, want nil (direct codex mode)", spec.BuildGate.Router.Amata)
				}
			},
		},
		{
			name: "router amata with empty spec",
			input: `{
				"steps": [{"image": "test:latest"}],
				"build_gate": {
					"router": {
						"image": "router:latest",
						"amata": {"spec": ""}
					}
				}
			}`,
			wantErr: "build_gate.router.amata.spec: required",
		},
		{
			name: "router amata with whitespace-only spec",
			input: `{
				"steps": [{"image": "test:latest"}],
				"build_gate": {
					"router": {
						"image": "router:latest",
						"amata": {"spec": "   "}
					}
				}
			}`,
			wantErr: "build_gate.router.amata.spec: required",
		},
		{
			name: "router amata set entry with empty param",
			input: `{
				"steps": [{"image": "test:latest"}],
				"build_gate": {
					"router": {
						"image": "router:latest",
						"amata": {
							"spec": "prompt: fix",
							"set": [{"param": "", "value": "v"}]
						}
					}
				}
			}`,
			wantErr: "build_gate.router.amata.set[0].param: required",
		},
		{
			name: "router amata set entry with whitespace param",
			input: `{
				"steps": [{"image": "test:latest"}],
				"build_gate": {
					"router": {
						"image": "router:latest",
						"amata": {
							"spec": "prompt: fix",
							"set": [{"param": "  ", "value": "v"}]
						}
					}
				}
			}`,
			wantErr: "build_gate.router.amata.set[0].param: required",
		},
		{
			name: "router amata set entry with empty value is allowed",
			input: `{
				"steps": [{"image": "test:latest"}],
				"build_gate": {
					"router": {
						"image": "router:latest",
						"amata": {
							"spec": "prompt: fix",
							"set": [{"param": "KEY", "value": ""}]
						}
					}
				}
			}`,
			check: func(t *testing.T, spec *ModsSpec) {
				p := spec.BuildGate.Router.Amata.Set[0]
				if p.Param != "KEY" || p.Value != "" {
					t.Errorf("set[0] = %+v, want {KEY }", p)
				}
			},
		},
		{
			name: "router flat spec key forbidden",
			input: `{
				"steps": [{"image": "test:latest"}],
				"build_gate": {
					"router": {"image": "router:latest", "spec": "bad"}
				}
			}`,
			wantErr: "build_gate.router.spec: forbidden",
		},
		{
			name: "router flat set key forbidden",
			input: `{
				"steps": [{"image": "test:latest"}],
				"build_gate": {
					"router": {"image": "router:latest", "set": []}
				}
			}`,
			wantErr: "build_gate.router.set: forbidden",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			spec, err := ParseModsSpecJSON([]byte(tt.input))
			if tt.wantErr != "" {
				if err == nil {
					t.Fatal("expected error, got nil")
				}
				if !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("error = %q, want to contain %q", err.Error(), tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if tt.check != nil {
				tt.check(t, spec)
			}
		})
	}
}

// TestParseModsSpecJSON_HealingAmata covers amata parsing and validation for healing actions.
func TestParseModsSpecJSON_HealingAmata(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		wantErr string
		check   func(t *testing.T, spec *ModsSpec)
	}{
		{
			name: "healing action with amata spec and set",
			input: `{
				"steps": [{"image": "test:latest"}],
				"build_gate": {
					"healing": {
						"by_error_kind": {
							"code": {
								"image": "heal:latest",
								"amata": {
									"spec": "prompt: heal the code",
									"set": [{"param": "ERR", "value": "compile"}]
								}
							}
						}
					},
					"router": {"image": "router:latest"}
				}
			}`,
			check: func(t *testing.T, spec *ModsSpec) {
				action := spec.BuildGate.Healing.ByErrorKind["code"]
				if action.Amata == nil {
					t.Fatal("healing.by_error_kind.code.amata is nil")
				}
				if action.Amata.Spec != "prompt: heal the code" {
					t.Errorf("amata.spec = %q, want %q", action.Amata.Spec, "prompt: heal the code")
				}
				if len(action.Amata.Set) != 1 || action.Amata.Set[0].Param != "ERR" {
					t.Errorf("amata.set = %+v, want [{ERR compile}]", action.Amata.Set)
				}
			},
		},
		{
			name: "healing action without amata (direct codex mode)",
			input: `{
				"steps": [{"image": "test:latest"}],
				"build_gate": {
					"healing": {
						"by_error_kind": {
							"infra": {"image": "heal:latest"}
						}
					},
					"router": {"image": "router:latest"}
				}
			}`,
			check: func(t *testing.T, spec *ModsSpec) {
				action := spec.BuildGate.Healing.ByErrorKind["infra"]
				if action.Amata != nil {
					t.Errorf("amata = %+v, want nil (direct codex mode)", action.Amata)
				}
			},
		},
		{
			name: "healing action amata with empty spec",
			input: `{
				"steps": [{"image": "test:latest"}],
				"build_gate": {
					"healing": {
						"by_error_kind": {
							"code": {
								"image": "heal:latest",
								"amata": {"spec": ""}
							}
						}
					},
					"router": {"image": "router:latest"}
				}
			}`,
			wantErr: "build_gate.healing.by_error_kind.code.amata.spec: required",
		},
		{
			name: "healing action amata set with empty param",
			input: `{
				"steps": [{"image": "test:latest"}],
				"build_gate": {
					"healing": {
						"by_error_kind": {
							"deps": {
								"image": "heal:latest",
								"amata": {
									"spec": "prompt: fix deps",
									"set": [{"param": "", "value": "v"}, {"param": "OK", "value": "1"}]
								}
							}
						}
					},
					"router": {"image": "router:latest"}
				}
			}`,
			wantErr: "build_gate.healing.by_error_kind.deps.amata.set[0].param: required",
		},
		{
			name: "healing action flat spec key forbidden",
			input: `{
				"steps": [{"image": "test:latest"}],
				"build_gate": {
					"healing": {
						"by_error_kind": {
							"infra": {"image": "heal:latest", "spec": "bad"}
						}
					},
					"router": {"image": "router:latest"}
				}
			}`,
			wantErr: "build_gate.healing.by_error_kind.infra.spec: forbidden",
		},
		{
			name: "healing action flat set key forbidden",
			input: `{
				"steps": [{"image": "test:latest"}],
				"build_gate": {
					"healing": {
						"by_error_kind": {
							"code": {"image": "heal:latest", "set": []}
						}
					},
					"router": {"image": "router:latest"}
				}
			}`,
			wantErr: "build_gate.healing.by_error_kind.code.set: forbidden",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			spec, err := ParseModsSpecJSON([]byte(tt.input))
			if tt.wantErr != "" {
				if err == nil {
					t.Fatal("expected error, got nil")
				}
				if !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("error = %q, want to contain %q", err.Error(), tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if tt.check != nil {
				tt.check(t, spec)
			}
		})
	}
}

// TestParseModsSpecJSON_AmataValidateRoundtrip confirms AmataRunSpec survives
// marshal → ParseModsSpecJSON intact.
func TestParseModsSpecJSON_AmataValidateRoundtrip(t *testing.T) {
	input := `{
		"steps": [{"image": "test:latest"}],
		"build_gate": {
			"healing": {
				"by_error_kind": {
					"code": {
						"image": "heal:latest",
						"amata": {
							"spec": "prompt: fix",
							"set": [{"param": "A", "value": "1"}, {"param": "B", "value": "2"}]
						}
					}
				}
			},
			"router": {
				"image": "router:latest",
				"amata": {
					"spec": "router-spec",
					"set": [{"param": "MODE", "value": "route"}]
				}
			}
		}
	}`

	spec, err := ParseModsSpecJSON([]byte(input))
	if err != nil {
		t.Fatalf("ParseModsSpecJSON failed: %v", err)
	}

	// Router amata.
	ra := spec.BuildGate.Router.Amata
	if ra == nil || ra.Spec != "router-spec" {
		t.Errorf("router.amata.spec = %v, want router-spec", ra)
	}
	if len(ra.Set) != 1 || ra.Set[0].Param != "MODE" || ra.Set[0].Value != "route" {
		t.Errorf("router.amata.set = %+v", ra.Set)
	}

	// Healing amata.
	ha := spec.BuildGate.Healing.ByErrorKind["code"].Amata
	if ha == nil || ha.Spec != "prompt: fix" {
		t.Errorf("healing.amata.spec = %v, want prompt: fix", ha)
	}
	if len(ha.Set) != 2 {
		t.Fatalf("healing.amata.set len = %d, want 2", len(ha.Set))
	}
	if ha.Set[0].Param != "A" || ha.Set[0].Value != "1" {
		t.Errorf("healing.amata.set[0] = %+v, want {A 1}", ha.Set[0])
	}
	if ha.Set[1].Param != "B" || ha.Set[1].Value != "2" {
		t.Errorf("healing.amata.set[1] = %+v, want {B 2}", ha.Set[1])
	}
}
