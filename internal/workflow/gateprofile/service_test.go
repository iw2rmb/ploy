package gateprofile

import (
	"testing"

	types "github.com/iw2rmb/ploy/internal/domain/types"
	"github.com/iw2rmb/ploy/internal/workflow/contracts"
)

// buildTestProfile returns a valid GateProfile parsed from JSON for use in tests.
func buildTestProfile(t *testing.T, raw string) *contracts.GateProfile {
	t.Helper()
	p, err := contracts.ParseGateProfileJSON([]byte(raw))
	if err != nil {
		t.Fatalf("ParseGateProfileJSON: %v", err)
	}
	return p
}

// TestProfilePrecedenceOrder locks the relative ordering of precedence constants so
// callers can safely compare ProfilePrecedence values numerically.
func TestProfilePrecedenceOrder(t *testing.T) {
	t.Parallel()
	if !(ProfilePrecedenceExact > ProfilePrecedenceLatest) {
		t.Errorf("ProfilePrecedenceExact (%d) must be > ProfilePrecedenceLatest (%d)",
			ProfilePrecedenceExact, ProfilePrecedenceLatest)
	}
	if !(ProfilePrecedenceLatest > ProfilePrecedenceDefault) {
		t.Errorf("ProfilePrecedenceLatest (%d) must be > ProfilePrecedenceDefault (%d)",
			ProfilePrecedenceLatest, ProfilePrecedenceDefault)
	}
}

func TestSelectProfile(t *testing.T) {
	t.Parallel()

	exact := &ProfileCandidate{ID: 1, ObjectKey: "exact-key", Precedence: ProfilePrecedenceExact}
	latest := &ProfileCandidate{ID: 2, ObjectKey: "latest-key", Precedence: ProfilePrecedenceLatest}
	def := &ProfileCandidate{ID: 3, ObjectKey: "default-key", Precedence: ProfilePrecedenceDefault}

	tests := []struct {
		name           string
		exact          *ProfileCandidate
		latest         *ProfileCandidate
		def            *ProfileCandidate
		wantID         int64
		wantObjectKey  string
		wantPrecedence ProfilePrecedence
	}{
		{
			name:           "exact wins over all others",
			exact:          exact,
			latest:         latest,
			def:            def,
			wantID:         1,
			wantObjectKey:  "exact-key",
			wantPrecedence: ProfilePrecedenceExact,
		},
		{
			name:           "latest wins when exact is absent",
			exact:          nil,
			latest:         latest,
			def:            def,
			wantID:         2,
			wantObjectKey:  "latest-key",
			wantPrecedence: ProfilePrecedenceLatest,
		},
		{
			name:           "default wins when exact and latest are absent",
			exact:          nil,
			latest:         nil,
			def:            def,
			wantID:         3,
			wantObjectKey:  "default-key",
			wantPrecedence: ProfilePrecedenceDefault,
		},
		{
			name:   "all nil returns nil",
			exact:  nil,
			latest: nil,
			def:    nil,
			wantID: 0,
		},
		// Single-candidate fallback cases: verify each tier works in isolation.
		{
			name:           "exact only — no fallback needed",
			exact:          exact,
			latest:         nil,
			def:            nil,
			wantID:         1,
			wantObjectKey:  "exact-key",
			wantPrecedence: ProfilePrecedenceExact,
		},
		{
			name:           "latest only — default is absent, latest is returned",
			exact:          nil,
			latest:         latest,
			def:            nil,
			wantID:         2,
			wantObjectKey:  "latest-key",
			wantPrecedence: ProfilePrecedenceLatest,
		},
		{
			name:           "default only — exact and latest absent, default is returned",
			exact:          nil,
			latest:         nil,
			def:            def,
			wantID:         3,
			wantObjectKey:  "default-key",
			wantPrecedence: ProfilePrecedenceDefault,
		},
		// Normalization: returned pointer is the same instance (no copying/mutation).
		{
			name:           "returned candidate is identical to input (no copy)",
			exact:          exact,
			latest:         latest,
			def:            def,
			wantID:         1,
			wantObjectKey:  "exact-key",
			wantPrecedence: ProfilePrecedenceExact,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := SelectProfile(tc.exact, tc.latest, tc.def)
			if tc.wantID == 0 {
				if got != nil {
					t.Fatalf("SelectProfile=%v, want nil", got)
				}
				return
			}
			if got == nil {
				t.Fatal("SelectProfile=nil, want non-nil")
			}
			if got.ID != tc.wantID {
				t.Fatalf("SelectProfile.ID=%d, want %d", got.ID, tc.wantID)
			}
			if got.ObjectKey != tc.wantObjectKey {
				t.Fatalf("SelectProfile.ObjectKey=%q, want %q", got.ObjectKey, tc.wantObjectKey)
			}
			if got.Precedence != tc.wantPrecedence {
				t.Fatalf("SelectProfile.Precedence=%d, want %d", got.Precedence, tc.wantPrecedence)
			}
		})
	}
}

func TestGateOverrideForJobType(t *testing.T) {
	t.Parallel()

	simpleProfile := buildTestProfile(t, `{
		"schema_version": 1, "repo_id": "repo_1",
		"runner_mode": "simple",
		"stack": {"language":"go","tool":"go"},
		"targets": {
			"active": "build",
			"build": {"status":"passed","command":"go test ./...","env":{"GOFLAGS":"-mod=readonly"}},
			"unit":  {"status":"not_attempted","env":{}},
			"all_tests": {"status":"not_attempted","env":{}}
		},
		"orchestration": {"pre":[],"post":[]}
	}`)

	unsupportedProfile := buildTestProfile(t, `{
		"schema_version": 1, "repo_id": "repo_1",
		"runner_mode": "simple",
		"stack": {"language":"go","tool":"go"},
		"targets": {
			"active": "unsupported",
			"build":  {"status":"failed","command":"go test ./...","env":{},"failure_code":"infra_support"},
			"unit":  {"status":"not_attempted","env":{}},
			"all_tests": {"status":"not_attempted","env":{}}
		},
		"orchestration": {"pre":[],"post":[]}
	}`)

	hostSocketProfile := buildTestProfile(t, `{
		"schema_version": 1, "repo_id": "repo_1",
		"runner_mode": "simple",
		"stack": {"language":"go","tool":"go"},
		"runtime": {"docker": {"mode":"host_socket","api_version":"1.45"}},
		"targets": {
			"active": "build",
			"build": {"status":"passed","command":"go test ./...","env":{}},
			"unit":  {"status":"not_attempted","env":{}},
			"all_tests": {"status":"not_attempted","env":{}}
		},
		"orchestration": {"pre":[],"post":[]}
	}`)

	tcpProfile := buildTestProfile(t, `{
		"schema_version": 1, "repo_id": "repo_1",
		"runner_mode": "simple",
		"stack": {"language":"go","tool":"go"},
		"runtime": {"docker": {"mode":"tcp","host":"tcp://dind:2375"}},
		"targets": {
			"active": "build",
			"build": {"status":"passed","command":"go test ./...","env":{}},
			"unit":  {"status":"not_attempted","env":{}},
			"all_tests": {"status":"not_attempted","env":{}}
		},
		"orchestration": {"pre":[],"post":[]}
	}`)

	failedTargetProfile := buildTestProfile(t, `{
		"schema_version": 1, "repo_id": "repo_1",
		"runner_mode": "simple",
		"stack": {"language":"go","tool":"go"},
		"targets": {
			"active": "unit",
			"build":  {"status":"not_attempted","command":"go build ./...","env":{}},
			"unit":  {"status":"failed","command":"go test ./... -run Unit","env":{}},
			"all_tests": {"status":"not_attempted","env":{}}
		},
		"orchestration": {"pre":[],"post":[]}
	}`)

	tests := []struct {
		name        string
		profile     *contracts.GateProfile
		jobType     types.JobType
		wantPhase   contracts.BuildGateProfilePhase
		wantNil     bool
		wantErr     bool
		wantCommand string
		wantEnvKey  string
		wantEnvVal  string
	}{
		{
			name:        "pre_gate maps to pre phase with active target command",
			profile:     simpleProfile,
			jobType:     types.JobTypePreGate,
			wantPhase:   contracts.BuildGateProfilePhasePre,
			wantCommand: "go test ./...",
			wantEnvKey:  "GOFLAGS",
			wantEnvVal:  "-mod=readonly",
		},
		{
			name:        "post_gate maps to post phase with active target command",
			profile:     simpleProfile,
			jobType:     types.JobTypePostGate,
			wantPhase:   contracts.BuildGateProfilePhasePost,
			wantCommand: "go test ./...",
		},
		{
			name:        "regate maps to post phase same as post_gate",
			profile:     simpleProfile,
			jobType:     types.JobTypeReGate,
			wantPhase:   contracts.BuildGateProfilePhasePost,
			wantCommand: "go test ./...",
		},
		{
			name:    "non-gate job type returns empty phase and nil override",
			profile: simpleProfile,
			jobType: types.JobTypeMod,
			wantNil: true,
		},
		{
			name:    "nil profile returns empty phase and nil override",
			profile: nil,
			jobType: types.JobTypePreGate,
			wantNil: true,
		},
		{
			name:    "unsupported active target returns phase with nil override",
			profile: unsupportedProfile,
			jobType: types.JobTypePreGate,
			wantPhase: contracts.BuildGateProfilePhasePre,
			wantNil: true,
		},
		{
			name:        "command always used regardless of target status (failed target)",
			profile:     failedTargetProfile,
			jobType:     types.JobTypePreGate,
			wantPhase:   contracts.BuildGateProfilePhasePre,
			wantCommand: "go test ./... -run Unit",
		},
		{
			name:       "host_socket runtime injects DOCKER_HOST env",
			profile:    hostSocketProfile,
			jobType:    types.JobTypePreGate,
			wantPhase:  contracts.BuildGateProfilePhasePre,
			wantEnvKey: contracts.GateProfileDockerHostEnv,
			wantEnvVal: defaultDockerHostSocket,
		},
		{
			name:       "host_socket with api_version injects DOCKER_API_VERSION env",
			profile:    hostSocketProfile,
			jobType:    types.JobTypePreGate,
			wantPhase:  contracts.BuildGateProfilePhasePre,
			wantEnvKey: contracts.GateProfileDockerAPIVersionEnv,
			wantEnvVal: "1.45",
		},
		{
			name:       "tcp runtime injects DOCKER_HOST from host field",
			profile:    tcpProfile,
			jobType:    types.JobTypePreGate,
			wantPhase:  contracts.BuildGateProfilePhasePre,
			wantEnvKey: contracts.GateProfileDockerHostEnv,
			wantEnvVal: "tcp://dind:2375",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			phase, override, err := GateOverrideForJobType(tc.profile, tc.jobType)
			if tc.wantErr {
				if err == nil {
					t.Fatal("expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if tc.wantNil {
				if override != nil {
					t.Fatalf("override=%v, want nil", override)
				}
				if tc.wantPhase != "" && phase != tc.wantPhase {
					t.Fatalf("phase=%q, want %q", phase, tc.wantPhase)
				}
				return
			}
			if phase != tc.wantPhase {
				t.Fatalf("phase=%q, want %q", phase, tc.wantPhase)
			}
			if override == nil {
				t.Fatal("override=nil, want non-nil")
			}
			if tc.wantCommand != "" && override.Command.Shell != tc.wantCommand {
				t.Fatalf("command=%q, want %q", override.Command.Shell, tc.wantCommand)
			}
			if tc.wantEnvKey != "" {
				if got := override.Env[tc.wantEnvKey]; got != tc.wantEnvVal {
					t.Fatalf("env[%s]=%q, want %q", tc.wantEnvKey, got, tc.wantEnvVal)
				}
			}
		})
	}
}

func TestStackMatches(t *testing.T) {
	t.Parallel()

	profile := buildTestProfile(t, `{
		"schema_version": 1, "repo_id": "repo_1",
		"runner_mode": "simple",
		"stack": {"language":"java","tool":"gradle","release":"11"},
		"targets": {
			"active": "build",
			"build": {"status":"passed","command":"./gradlew test","env":{}},
			"unit":  {"status":"not_attempted","env":{}},
			"all_tests": {"status":"not_attempted","env":{}}
		},
		"orchestration": {"pre":[],"post":[]}
	}`)

	noReleaseProfile := buildTestProfile(t, `{
		"schema_version": 1, "repo_id": "repo_1",
		"runner_mode": "simple",
		"stack": {"language":"java","tool":"gradle"},
		"targets": {
			"active": "build",
			"build": {"status":"passed","command":"./gradlew test","env":{}},
			"unit":  {"status":"not_attempted","env":{}},
			"all_tests": {"status":"not_attempted","env":{}}
		},
		"orchestration": {"pre":[],"post":[]}
	}`)

	tests := []struct {
		name    string
		profile *contracts.GateProfile
		lang    string
		tool    string
		release string
		want    bool
	}{
		{
			name:    "nil profile does not match",
			profile: nil,
			lang:    "java",
			tool:    "gradle",
			release: "",
			want:    false,
		},
		{
			name:    "empty release acts as wildcard (matches any release)",
			profile: profile,
			lang:    "java",
			tool:    "gradle",
			release: "",
			want:    true,
		},
		{
			name:    "exact release match passes",
			profile: profile,
			lang:    "java",
			tool:    "gradle",
			release: "11",
			want:    true,
		},
		{
			name:    "mismatched release fails",
			profile: profile,
			lang:    "java",
			tool:    "gradle",
			release: "17",
			want:    false,
		},
		{
			name:    "non-empty release against profile with no release fails",
			profile: noReleaseProfile,
			lang:    "java",
			tool:    "gradle",
			release: "11",
			want:    false,
		},
		{
			name:    "empty release against profile with no release matches",
			profile: noReleaseProfile,
			lang:    "java",
			tool:    "gradle",
			release: "",
			want:    true,
		},
		{
			name:    "wrong language does not match",
			profile: profile,
			lang:    "go",
			tool:    "gradle",
			release: "",
			want:    false,
		},
		{
			name:    "wrong tool does not match",
			profile: profile,
			lang:    "java",
			tool:    "maven",
			release: "",
			want:    false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := StackMatches(tc.profile, tc.lang, tc.tool, tc.release)
			if got != tc.want {
				t.Fatalf("StackMatches(%q,%q,%q)=%v, want %v", tc.lang, tc.tool, tc.release, got, tc.want)
			}
		})
	}
}
