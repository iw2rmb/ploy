package contracts

import (
	"fmt"
	"testing"
)

// amataPlacement describes where an AmataRunSpec can appear in a MigSpec,
// parameterizing JSON construction and result extraction so behavioral tests
// are written once and run for every valid placement site.
type amataPlacement struct {
	name     string
	wrapJSON func(amataFragment string) string           // build full spec JSON given an amata JSON object
	baseJSON string                                      // valid spec JSON without amata (direct codex mode)
	getAmata func(*MigSpec) *AmataRunSpec               // extract amata from parsed result
	errPfx   string                                      // error path prefix, e.g. "build_gate.heal.amata"
	forbid   []struct{ name, input, wantErr string }     // site-specific flat-key forbidden cases
}

func amataplacements() []amataPlacement {
	return []amataPlacement{
		{
			name: "Step",
			wrapJSON: func(frag string) string {
				return fmt.Sprintf(`{
					"steps": [{"image": "test:latest", "amata": %s}]
				}`, frag)
			},
			baseJSON: `{"steps": [{"image": "test:latest"}]}`,
			getAmata: func(s *MigSpec) *AmataRunSpec { return s.Steps[0].Amata },
			errPfx:   "steps[0].amata",
		},
		{
			name: "Heal",
			wrapJSON: func(frag string) string {
				return fmt.Sprintf(`{
					"steps": [{"image": "test:latest"}],
					"build_gate": {
						"heal": {"image": "heal:latest", "amata": %s}
					}
				}`, frag)
			},
			baseJSON: `{
				"steps": [{"image": "test:latest"}],
				"build_gate": {
					"heal": {"image": "heal:latest"}
				}
			}`,
			getAmata: func(s *MigSpec) *AmataRunSpec {
				return s.BuildGate.Heal.Amata
			},
			errPfx: "build_gate.heal.amata",
			forbid: []struct{ name, input, wantErr string }{
				{
					name: "flat_spec_forbidden",
					input: `{
						"steps": [{"image": "test:latest"}],
						"build_gate": {"heal": {"image": "heal:latest", "spec": "bad"}}
					}`,
					wantErr: "build_gate.heal.spec: forbidden",
				},
				{
					name: "flat_set_forbidden",
					input: `{
						"steps": [{"image": "test:latest"}],
						"build_gate": {"heal": {"image": "heal:latest", "set": []}}
					}`,
					wantErr: "build_gate.heal.set: forbidden",
				},
			},
		},
	}
}

// TestParseMigSpecJSON_Amata covers amata parsing and validation for every
// valid placement site: step and heal action.
func TestParseMigSpecJSON_Amata(t *testing.T) {
	for _, p := range amataplacements() {
		t.Run(p.name, func(t *testing.T) {
			t.Run("spec_and_set", func(t *testing.T) {
				input := p.wrapJSON(`{
					"spec": "version: 1\nprompt: fix it",
					"set": [
						{"param": "REPO", "value": "my-repo"},
						{"param": "BRANCH", "value": "main"}
					]
				}`)
				spec, err := ParseMigSpecJSON([]byte(input))
				requireValidationErr(t, err, "")

				a := p.getAmata(spec)
				if a == nil {
					t.Fatal("amata is nil")
				}
				if a.Spec != "version: 1\nprompt: fix it" {
					t.Errorf("spec = %q, want %q", a.Spec, "version: 1\nprompt: fix it")
				}
				if len(a.Set) != 2 {
					t.Fatalf("set len = %d, want 2", len(a.Set))
				}
				if a.Set[0].Param != "REPO" || a.Set[0].Value != "my-repo" {
					t.Errorf("set[0] = %+v, want {REPO my-repo}", a.Set[0])
				}
				if a.Set[1].Param != "BRANCH" || a.Set[1].Value != "main" {
					t.Errorf("set[1] = %+v, want {BRANCH main}", a.Set[1])
				}
			})

			t.Run("spec_only_no_set", func(t *testing.T) {
				input := p.wrapJSON(`{"spec": "prompt: fix"}`)
				spec, err := ParseMigSpecJSON([]byte(input))
				requireValidationErr(t, err, "")

				a := p.getAmata(spec)
				if a == nil {
					t.Fatal("amata is nil")
				}
				if len(a.Set) != 0 {
					t.Errorf("set len = %d, want 0", len(a.Set))
				}
			})

			t.Run("no_amata_direct_codex", func(t *testing.T) {
				spec, err := ParseMigSpecJSON([]byte(p.baseJSON))
				requireValidationErr(t, err, "")

				if p.getAmata(spec) != nil {
					t.Errorf("amata = %+v, want nil (direct codex mode)", p.getAmata(spec))
				}
			})

			t.Run("empty_spec_required", func(t *testing.T) {
				input := p.wrapJSON(`{"spec": ""}`)
				_, err := ParseMigSpecJSON([]byte(input))
				requireValidationErr(t, err, p.errPfx+".spec: required")
			})

			t.Run("whitespace_spec_required", func(t *testing.T) {
				input := p.wrapJSON(`{"spec": "   "}`)
				_, err := ParseMigSpecJSON([]byte(input))
				requireValidationErr(t, err, p.errPfx+".spec: required")
			})

			t.Run("empty_param_required", func(t *testing.T) {
				input := p.wrapJSON(`{"spec": "prompt: fix", "set": [{"param": "", "value": "v"}]}`)
				_, err := ParseMigSpecJSON([]byte(input))
				requireValidationErr(t, err, p.errPfx+".set[0].param: required")
			})

			t.Run("empty_value_allowed", func(t *testing.T) {
				input := p.wrapJSON(`{"spec": "prompt: fix", "set": [{"param": "KEY", "value": ""}]}`)
				spec, err := ParseMigSpecJSON([]byte(input))
				requireValidationErr(t, err, "")

				s := p.getAmata(spec).Set[0]
				if s.Param != "KEY" || s.Value != "" {
					t.Errorf("set[0] = %+v, want {KEY }", s)
				}
			})

			for _, fb := range p.forbid {
				t.Run(fb.name, func(t *testing.T) {
					_, err := ParseMigSpecJSON([]byte(fb.input))
					requireValidationErr(t, err, fb.wantErr)
				})
			}
		})
	}
}

// TestParseMigSpecJSON_AmataForbiddenPlacements verifies that amata is rejected
// outside of steps[].amata and build_gate.heal.amata.
func TestParseMigSpecJSON_AmataForbiddenPlacements(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		wantErr string
	}{
		{
			name: "build_gate amata forbidden",
			input: `{
				"steps": [{"image": "test:latest"}],
				"build_gate": {
					"amata": {"spec": "bad"}
				}
			}`,
			wantErr: "build_gate.amata: forbidden",
		},
		{
			name: "build_gate pre amata forbidden",
			input: `{
				"steps": [{"image": "test:latest"}],
				"build_gate": {
					"pre": {
						"amata": {"spec": "bad"}
					}
				}
			}`,
			wantErr: "build_gate.pre.amata: forbidden",
		},
		{
			name: "build_gate pre stack nested amata forbidden",
			input: `{
				"steps": [{"image": "test:latest"}],
				"build_gate": {
					"pre": {
						"stack": {
							"enabled": true,
							"amata": {"spec": "bad"}
						}
					}
				}
			}`,
			wantErr: "build_gate.pre.stack.amata: forbidden",
		},
		{
			name: "build_gate pre stack flat spec forbidden",
			input: `{
				"steps": [{"image": "test:latest"}],
				"build_gate": {
					"pre": {
						"stack": {
							"enabled": true,
							"spec": "bad"
						}
					}
				}
			}`,
			wantErr: "build_gate.pre.stack.spec: forbidden",
		},
		{
			name: "build_gate pre stack flat set forbidden",
			input: `{
				"steps": [{"image": "test:latest"}],
				"build_gate": {
					"pre": {
						"stack": {
							"enabled": true,
							"set": []
						}
					}
				}
			}`,
			wantErr: "build_gate.pre.stack.set: forbidden",
		},
		{
			name: "build_gate post nested amata forbidden",
			input: `{
				"steps": [{"image": "test:latest"}],
				"build_gate": {
					"post": {
						"stack": {
							"enabled": true,
							"amata": {"spec": "bad"}
						}
					}
				}
			}`,
			wantErr: "build_gate.post.stack.amata: forbidden",
		},
		{
			name: "build_gate post stack flat spec forbidden",
			input: `{
				"steps": [{"image": "test:latest"}],
				"build_gate": {
					"post": {
						"stack": {
							"enabled": true,
							"spec": "bad"
						}
					}
				}
			}`,
			wantErr: "build_gate.post.stack.spec: forbidden",
		},
		{
			name: "build_gate post stack flat set forbidden",
			input: `{
				"steps": [{"image": "test:latest"}],
				"build_gate": {
					"post": {
						"stack": {
							"enabled": true,
							"set": []
						}
					}
				}
			}`,
			wantErr: "build_gate.post.stack.set: forbidden",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := ParseMigSpecJSON([]byte(tt.input))
			requireValidationErr(t, err, tt.wantErr)
		})
	}
}
