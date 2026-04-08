package types

import "testing"

// TestGlobalEnvTarget_Validate tests that Validate() correctly accepts known
// targets and rejects unknown/empty values.
func TestGlobalEnvTarget_Validate(t *testing.T) {
	tests := []struct {
		name    string
		target  GlobalEnvTarget
		wantErr bool
	}{
		// Valid targets.
		{name: "server is valid", target: GlobalEnvTargetServer, wantErr: false},
		{name: "nodes is valid", target: GlobalEnvTargetNodes, wantErr: false},
		{name: "gates is valid", target: GlobalEnvTargetGates, wantErr: false},
		{name: "steps is valid", target: GlobalEnvTargetSteps, wantErr: false},

		// Valid targets with surrounding whitespace (should be accepted after normalization).
		{name: "server with spaces", target: "  server  ", wantErr: false},
		{name: "gates with tabs", target: "\tgates\t", wantErr: false},

		// Invalid targets.
		{name: "empty string is invalid", target: "", wantErr: true},
		{name: "whitespace only is invalid", target: "   ", wantErr: true},
		{name: "unknown target is invalid", target: "unknown", wantErr: true},

		// Old scope values are invalid (hard cut).
		{name: "old scope all is invalid", target: "all", wantErr: true},
		{name: "old scope migs is invalid", target: "migs", wantErr: true},
		{name: "old scope heal is invalid", target: "heal", wantErr: true},
		{name: "old scope gate is invalid", target: "gate", wantErr: true},

		// Case sensitivity.
		{name: "mixed case Server is invalid", target: "Server", wantErr: true},
		{name: "mixed case GATES is invalid", target: "GATES", wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.target.Validate()
			if (err != nil) != tt.wantErr {
				t.Errorf("GlobalEnvTarget(%q).Validate() error = %v, wantErr %v",
					tt.target, err, tt.wantErr)
			}
		})
	}
}

// TestParseGlobalEnvTarget tests that ParseGlobalEnvTarget correctly parses valid
// targets and returns errors for invalid values.
func TestParseGlobalEnvTarget(t *testing.T) {
	tests := []struct {
		name      string
		input     string
		want      GlobalEnvTarget
		wantErr   bool
		errSubstr string
	}{
		// Valid inputs.
		{name: "server", input: "server", want: GlobalEnvTargetServer, wantErr: false},
		{name: "nodes", input: "nodes", want: GlobalEnvTargetNodes, wantErr: false},
		{name: "gates", input: "gates", want: GlobalEnvTargetGates, wantErr: false},
		{name: "steps", input: "steps", want: GlobalEnvTargetSteps, wantErr: false},

		// Empty is rejected (no default).
		{name: "empty rejected", input: "", wantErr: true, errSubstr: "target is required"},
		{name: "whitespace rejected", input: "   ", wantErr: true, errSubstr: "target is required"},

		// Valid with whitespace normalization.
		{name: "server with spaces", input: "  server  ", want: GlobalEnvTargetServer, wantErr: false},

		// Old scope values are invalid.
		{name: "old scope all", input: "all", want: "", wantErr: true, errSubstr: "invalid target"},
		{name: "old scope migs", input: "migs", want: "", wantErr: true, errSubstr: "invalid target"},

		// Invalid inputs.
		{name: "unknown target", input: "unknown", want: "", wantErr: true, errSubstr: "invalid target"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ParseGlobalEnvTarget(tt.input)
			if (err != nil) != tt.wantErr {
				t.Errorf("ParseGlobalEnvTarget(%q) error = %v, wantErr %v",
					tt.input, err, tt.wantErr)
				return
			}
			if !tt.wantErr && got != tt.want {
				t.Errorf("ParseGlobalEnvTarget(%q) = %q, want %q", tt.input, got, tt.want)
			}
			if tt.wantErr && tt.errSubstr != "" && err != nil {
				if !contains(err.Error(), tt.errSubstr) {
					t.Errorf("ParseGlobalEnvTarget(%q) error = %q, want error containing %q",
						tt.input, err.Error(), tt.errSubstr)
				}
			}
		})
	}
}

// TestGlobalEnvTarget_MatchesJobType tests the target matching logic that determines
// whether a global env var should be injected based on job type and env var target.
func TestGlobalEnvTarget_MatchesJobType(t *testing.T) {
	tests := []struct {
		name    string
		target  GlobalEnvTarget
		jobType JobType
		want    bool
	}{
		// "gates" target matches gate-related jobs.
		{name: "gates matches pre_gate", target: GlobalEnvTargetGates, jobType: JobTypePreGate, want: true},
		{name: "gates matches re_gate", target: GlobalEnvTargetGates, jobType: JobTypeReGate, want: true},
		{name: "gates matches post_gate", target: GlobalEnvTargetGates, jobType: JobTypePostGate, want: true},
		{name: "gates does not match mig", target: GlobalEnvTargetGates, jobType: JobTypeMig, want: false},
		{name: "gates does not match heal", target: GlobalEnvTargetGates, jobType: JobTypeHeal, want: false},

		// "steps" target matches step jobs.
		{name: "steps matches mig", target: GlobalEnvTargetSteps, jobType: JobTypeMig, want: true},
		{name: "steps matches heal", target: GlobalEnvTargetSteps, jobType: JobTypeHeal, want: true},
		{name: "steps does not match pre_gate", target: GlobalEnvTargetSteps, jobType: JobTypePreGate, want: false},
		{name: "steps does not match re_gate", target: GlobalEnvTargetSteps, jobType: JobTypeReGate, want: false},
		{name: "steps does not match post_gate", target: GlobalEnvTargetSteps, jobType: JobTypePostGate, want: false},

		// "server" target does not match any job type (not job-routed).
		{name: "server does not match mig", target: GlobalEnvTargetServer, jobType: JobTypeMig, want: false},
		{name: "server does not match heal", target: GlobalEnvTargetServer, jobType: JobTypeHeal, want: false},
		{name: "server does not match pre_gate", target: GlobalEnvTargetServer, jobType: JobTypePreGate, want: false},

		// "nodes" target does not match any job type (not job-routed).
		{name: "nodes does not match mig", target: GlobalEnvTargetNodes, jobType: JobTypeMig, want: false},
		{name: "nodes does not match heal", target: GlobalEnvTargetNodes, jobType: JobTypeHeal, want: false},
		{name: "nodes does not match pre_gate", target: GlobalEnvTargetNodes, jobType: JobTypePreGate, want: false},

		// Unknown/empty targets should not match.
		{name: "unknown target does not match", target: "unknown", jobType: JobTypeMig, want: false},
		{name: "empty target does not match", target: "", jobType: JobTypeMig, want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.target.MatchesJobType(tt.jobType)
			if got != tt.want {
				t.Errorf("GlobalEnvTarget(%q).MatchesJobType(%q) = %v, want %v",
					tt.target, tt.jobType, got, tt.want)
			}
		})
	}
}

// TestGlobalEnvTarget_String tests that String() returns the underlying value.
func TestGlobalEnvTarget_String(t *testing.T) {
	tests := []struct {
		target GlobalEnvTarget
		want   string
	}{
		{GlobalEnvTargetServer, "server"},
		{GlobalEnvTargetNodes, "nodes"},
		{GlobalEnvTargetGates, "gates"},
		{GlobalEnvTargetSteps, "steps"},
	}
	for _, tt := range tests {
		if got := tt.target.String(); got != tt.want {
			t.Errorf("GlobalEnvTarget(%q).String() = %q, want %q", tt.target, got, tt.want)
		}
	}
}

// TestGlobalEnvTarget_IsZero tests that IsZero() correctly identifies empty values.
func TestGlobalEnvTarget_IsZero(t *testing.T) {
	tests := []struct {
		target GlobalEnvTarget
		want   bool
	}{
		{"", true},
		{"   ", true},
		{"\t\n", true},
		{"server", false},
		{"gates", false},
	}
	for _, tt := range tests {
		if got := tt.target.IsZero(); got != tt.want {
			t.Errorf("GlobalEnvTarget(%q).IsZero() = %v, want %v", tt.target, got, tt.want)
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
