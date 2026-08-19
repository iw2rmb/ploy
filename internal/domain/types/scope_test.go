package types

import (
	"strings"
	"testing"
)

func TestGlobalEnvTargetParsingAndValidation(t *testing.T) {
	tests := []struct {
		name      string
		input     string
		want      GlobalEnvTarget
		errSubstr string
	}{
		{name: "server", input: "server", want: GlobalEnvTargetServer},
		{name: "nodes", input: "nodes", want: GlobalEnvTargetNodes},
		{name: "gates", input: "gates", want: GlobalEnvTargetGates},
		{name: "steps", input: "steps", want: GlobalEnvTargetSteps},
		{name: "trims spaces", input: "  server  ", want: GlobalEnvTargetServer},
		{name: "trims tabs", input: "\tgates\t", want: GlobalEnvTargetGates},
		{name: "empty", input: "", errSubstr: "target is required"},
		{name: "whitespace", input: "   ", errSubstr: "target is required"},
		{name: "unknown", input: "unknown", errSubstr: "invalid target"},
		{name: "retired all", input: "all", errSubstr: "invalid target"},
		{name: "retired migs", input: "migs", errSubstr: "invalid target"},
		{name: "retired gate", input: "gate", errSubstr: "invalid target"},
		{name: "case sensitive Server", input: "Server", errSubstr: "invalid target"},
		{name: "case sensitive GATES", input: "GATES", errSubstr: "invalid target"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			validateErr := GlobalEnvTarget(tt.input).Validate()
			got, parseErr := ParseGlobalEnvTarget(tt.input)
			if tt.errSubstr == "" {
				if validateErr != nil || parseErr != nil {
					t.Fatalf("validation errors = (%v, %v), want nil", validateErr, parseErr)
				}
				if got != tt.want {
					t.Errorf("ParseGlobalEnvTarget() = %q, want %q", got, tt.want)
				}
				return
			}
			for name, err := range map[string]error{"Validate": validateErr, "ParseGlobalEnvTarget": parseErr} {
				if err == nil || !strings.Contains(err.Error(), tt.errSubstr) {
					t.Errorf("%s error = %v, want error containing %q", name, err, tt.errSubstr)
				}
			}
		})
	}
}

func TestGlobalEnvTargetMatchesJobType(t *testing.T) {
	tests := []struct {
		name    string
		target  GlobalEnvTarget
		jobType JobType
		want    bool
	}{
		{name: "gates matches pre-gate", target: GlobalEnvTargetGates, jobType: JobTypePreGate, want: true},
		{name: "gates matches post-gate", target: GlobalEnvTargetGates, jobType: JobTypePostGate, want: true},
		{name: "gates excludes mig", target: GlobalEnvTargetGates, jobType: JobTypeMig},
		{name: "steps matches mig", target: GlobalEnvTargetSteps, jobType: JobTypeMig, want: true},
		{name: "steps excludes pre-gate", target: GlobalEnvTargetSteps, jobType: JobTypePreGate},
		{name: "steps excludes post-gate", target: GlobalEnvTargetSteps, jobType: JobTypePostGate},
		{name: "server is not job-routed", target: GlobalEnvTargetServer, jobType: JobTypeMig},
		{name: "nodes is not job-routed", target: GlobalEnvTargetNodes, jobType: JobTypeMig},
		{name: "unknown does not match", target: "unknown", jobType: JobTypeMig},
		{name: "empty does not match", target: "", jobType: JobTypeMig},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.target.MatchesJobType(tt.jobType); got != tt.want {
				t.Errorf("MatchesJobType(%q) = %v, want %v", tt.jobType, got, tt.want)
			}
		})
	}
}
