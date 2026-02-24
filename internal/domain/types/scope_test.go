package types

import "testing"

// TestGlobalEnvScope_Validate tests that Validate() correctly accepts known
// scopes and rejects unknown/empty values.
func TestGlobalEnvScope_Validate(t *testing.T) {
	tests := []struct {
		name    string
		scope   GlobalEnvScope
		wantErr bool
	}{
		// Valid scopes.
		{name: "all is valid", scope: GlobalEnvScopeAll, wantErr: false},
		{name: "mods is valid", scope: GlobalEnvScopeMods, wantErr: false},
		{name: "heal is valid", scope: GlobalEnvScopeHeal, wantErr: false},
		{name: "gate is valid", scope: GlobalEnvScopeGate, wantErr: false},

		// Valid scopes with surrounding whitespace (should be accepted after normalization).
		{name: "all with spaces", scope: "  all  ", wantErr: false},
		{name: "mods with tabs", scope: "\tmods\t", wantErr: false},

		// Invalid scopes.
		{name: "empty string is invalid", scope: "", wantErr: true},
		{name: "whitespace only is invalid", scope: "   ", wantErr: true},
		{name: "unknown scope is invalid", scope: "unknown", wantErr: true},
		{name: "typo mod (not mods) is invalid", scope: "mod", wantErr: true},
		{name: "typo gates is invalid", scope: "gates", wantErr: true},
		{name: "mixed case ALL is invalid", scope: "ALL", wantErr: true},
		{name: "mixed case Mods is invalid", scope: "Mods", wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.scope.Validate()
			if (err != nil) != tt.wantErr {
				t.Errorf("GlobalEnvScope(%q).Validate() error = %v, wantErr %v",
					tt.scope, err, tt.wantErr)
			}
		})
	}
}

// TestParseGlobalEnvScope tests that ParseGlobalEnvScope correctly parses valid
// scopes and returns errors for invalid values.
func TestParseGlobalEnvScope(t *testing.T) {
	tests := []struct {
		name      string
		input     string
		want      GlobalEnvScope
		wantErr   bool
		errSubstr string
	}{
		// Valid inputs.
		{name: "all", input: "all", want: GlobalEnvScopeAll, wantErr: false},
		{name: "mods", input: "mods", want: GlobalEnvScopeMods, wantErr: false},
		{name: "heal", input: "heal", want: GlobalEnvScopeHeal, wantErr: false},
		{name: "gate", input: "gate", want: GlobalEnvScopeGate, wantErr: false},

		// Empty defaults to "all".
		{name: "empty defaults to all", input: "", want: GlobalEnvScopeAll, wantErr: false},
		{name: "whitespace defaults to all", input: "   ", want: GlobalEnvScopeAll, wantErr: false},

		// Valid with whitespace normalization.
		{name: "all with spaces", input: "  all  ", want: GlobalEnvScopeAll, wantErr: false},

		// Invalid inputs.
		{name: "unknown scope", input: "unknown", want: "", wantErr: true, errSubstr: "invalid scope"},
		{name: "typo mod", input: "mod", want: "", wantErr: true, errSubstr: "invalid scope"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ParseGlobalEnvScope(tt.input)
			if (err != nil) != tt.wantErr {
				t.Errorf("ParseGlobalEnvScope(%q) error = %v, wantErr %v",
					tt.input, err, tt.wantErr)
				return
			}
			if !tt.wantErr && got != tt.want {
				t.Errorf("ParseGlobalEnvScope(%q) = %q, want %q", tt.input, got, tt.want)
			}
			if tt.wantErr && tt.errSubstr != "" && err != nil {
				if !contains(err.Error(), tt.errSubstr) {
					t.Errorf("ParseGlobalEnvScope(%q) error = %q, want error containing %q",
						tt.input, err.Error(), tt.errSubstr)
				}
			}
		})
	}
}

// TestGlobalEnvScope_MatchesJobType tests the scope matching logic that determines
// whether a global env var should be injected based on job type and env var scope.
func TestGlobalEnvScope_MatchesJobType(t *testing.T) {
	tests := []struct {
		name    string
		scope   GlobalEnvScope
		modType JobType
		want    bool
	}{
		// "all" scope matches every job type.
		{name: "all matches mod", scope: GlobalEnvScopeAll, modType: JobTypeMod, want: true},
		{name: "all matches heal", scope: GlobalEnvScopeAll, modType: JobTypeHeal, want: true},
		{name: "all matches pre_gate", scope: GlobalEnvScopeAll, modType: JobTypePreGate, want: true},
		{name: "all matches re_gate", scope: GlobalEnvScopeAll, modType: JobTypeReGate, want: true},
		{name: "all matches post_gate", scope: GlobalEnvScopeAll, modType: JobTypePostGate, want: true},
		{name: "all matches mr", scope: GlobalEnvScopeAll, modType: JobTypeMR, want: true},

		// "mods" scope matches mod and post_gate jobs.
		{name: "mods matches mod", scope: GlobalEnvScopeMods, modType: JobTypeMod, want: true},
		{name: "mods matches post_gate", scope: GlobalEnvScopeMods, modType: JobTypePostGate, want: true},
		{name: "mods does not match heal", scope: GlobalEnvScopeMods, modType: JobTypeHeal, want: false},
		{name: "mods does not match pre_gate", scope: GlobalEnvScopeMods, modType: JobTypePreGate, want: false},
		{name: "mods does not match re_gate", scope: GlobalEnvScopeMods, modType: JobTypeReGate, want: false},
		{name: "mods does not match mr", scope: GlobalEnvScopeMods, modType: JobTypeMR, want: false},

		// "heal" scope matches heal and re_gate jobs.
		{name: "heal matches heal", scope: GlobalEnvScopeHeal, modType: JobTypeHeal, want: true},
		{name: "heal matches re_gate", scope: GlobalEnvScopeHeal, modType: JobTypeReGate, want: true},
		{name: "heal does not match mod", scope: GlobalEnvScopeHeal, modType: JobTypeMod, want: false},
		{name: "heal does not match pre_gate", scope: GlobalEnvScopeHeal, modType: JobTypePreGate, want: false},
		{name: "heal does not match post_gate", scope: GlobalEnvScopeHeal, modType: JobTypePostGate, want: false},
		{name: "heal does not match mr", scope: GlobalEnvScopeHeal, modType: JobTypeMR, want: false},

		// "gate" scope matches all gate-related jobs.
		{name: "gate matches pre_gate", scope: GlobalEnvScopeGate, modType: JobTypePreGate, want: true},
		{name: "gate matches re_gate", scope: GlobalEnvScopeGate, modType: JobTypeReGate, want: true},
		{name: "gate matches post_gate", scope: GlobalEnvScopeGate, modType: JobTypePostGate, want: true},
		{name: "gate does not match mod", scope: GlobalEnvScopeGate, modType: JobTypeMod, want: false},
		{name: "gate does not match heal", scope: GlobalEnvScopeGate, modType: JobTypeHeal, want: false},
		{name: "gate does not match mr", scope: GlobalEnvScopeGate, modType: JobTypeMR, want: false},

		// Unknown/empty scopes should not match.
		{name: "unknown scope does not match", scope: "unknown", modType: JobTypeMod, want: false},
		{name: "empty scope does not match", scope: "", modType: JobTypeMod, want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.scope.MatchesJobType(tt.modType)
			if got != tt.want {
				t.Errorf("GlobalEnvScope(%q).MatchesJobType(%q) = %v, want %v",
					tt.scope, tt.modType, got, tt.want)
			}
		})
	}
}

// TestGlobalEnvScope_String tests that String() returns the underlying value.
func TestGlobalEnvScope_String(t *testing.T) {
	tests := []struct {
		scope GlobalEnvScope
		want  string
	}{
		{GlobalEnvScopeAll, "all"},
		{GlobalEnvScopeMods, "mods"},
		{GlobalEnvScopeHeal, "heal"},
		{GlobalEnvScopeGate, "gate"},
	}
	for _, tt := range tests {
		if got := tt.scope.String(); got != tt.want {
			t.Errorf("GlobalEnvScope(%q).String() = %q, want %q", tt.scope, got, tt.want)
		}
	}
}

// TestGlobalEnvScope_IsZero tests that IsZero() correctly identifies empty values.
func TestGlobalEnvScope_IsZero(t *testing.T) {
	tests := []struct {
		scope GlobalEnvScope
		want  bool
	}{
		{"", true},
		{"   ", true},
		{"\t\n", true},
		{"all", false},
		{"mods", false},
	}
	for _, tt := range tests {
		if got := tt.scope.IsZero(); got != tt.want {
			t.Errorf("GlobalEnvScope(%q).IsZero() = %v, want %v", tt.scope, got, tt.want)
		}
	}
}

// contains is a helper function that checks if s contains substr.
func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(substr) == 0 ||
		(len(s) > 0 && len(substr) > 0 && findSubstring(s, substr) >= 0))
}

func findSubstring(s, substr string) int {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return i
		}
	}
	return -1
}
