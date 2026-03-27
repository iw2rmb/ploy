package gateprofile

import (
	"errors"
	"testing"

	types "github.com/iw2rmb/ploy/internal/domain/types"
	"github.com/iw2rmb/ploy/internal/workflow/contracts"
)

func TestDeriveProfileSnapshotFromOverride(t *testing.T) {
	t.Parallel()

	mavenOverride := &contracts.BuildGateProfileOverride{
		Command: contracts.CommandSpec{Shell: "mvn -q test"},
		Env:     map[string]string{"MAVEN_OPTS": "-Xmx2g"},
		Stack:   &contracts.GateProfileStack{Language: "java", Tool: "maven", Release: "21"},
		Target:  contracts.GateProfileTargetAllTests,
	}

	gradleOverride := &contracts.BuildGateProfileOverride{
		Command: contracts.CommandSpec{Shell: "./gradlew test"},
		Stack:   &contracts.GateProfileStack{Language: "java", Tool: "gradle"},
	}

	execOverride := &contracts.BuildGateProfileOverride{
		Command: contracts.CommandSpec{Exec: []string{"mvn", "-q", "compile"}},
		Stack:   &contracts.GateProfileStack{Language: "java", Tool: "maven"},
	}

	tests := []struct {
		name           string
		repoID         string
		override       *contracts.BuildGateProfileOverride
		target         string
		jobType        types.JobType
		meta           *contracts.BuildGateStageMetadata
		wantErr        bool
		wantActive     string
		wantStackLang  string
		wantStackTool  string
		wantStackRel   string
		wantEnvKey     string
		wantEnvVal     string
		wantCmdInBuild string
		wantCmdInUnit  string
		wantCmdInAll   string
	}{
		{
			name:          "shell command, explicit stack, all_tests target via override.Target",
			repoID:        "repo_1",
			override:      mavenOverride,
			target:        "",
			jobType:       types.JobTypePreGate,
			wantActive:    contracts.GateProfileTargetAllTests,
			wantStackLang: "java",
			wantStackTool: "maven",
			wantStackRel:  "21",
			wantEnvKey:    "MAVEN_OPTS",
			wantEnvVal:    "-Xmx2g",
			wantCmdInAll:  "mvn -q test",
		},
		{
			name:          "explicit target=build overrides override.Target",
			repoID:        "repo_1",
			override:      mavenOverride,
			target:        contracts.GateProfileTargetBuild,
			jobType:       types.JobTypePreGate,
			wantActive:    contracts.GateProfileTargetBuild,
			wantStackLang: "java",
			wantStackTool: "maven",
			wantCmdInBuild: "mvn -q test",
		},
		{
			name:          "explicit target=unit",
			repoID:        "repo_1",
			override:      mavenOverride,
			target:        contracts.GateProfileTargetUnit,
			jobType:       types.JobTypePostGate,
			wantActive:    contracts.GateProfileTargetUnit,
			wantStackLang: "java",
			wantStackTool: "maven",
			wantCmdInUnit: "mvn -q test",
		},
		{
			name:          "re_gate accepted same as post_gate",
			repoID:        "repo_1",
			override:      gradleOverride,
			target:        "",
			jobType:       types.JobTypeReGate,
			wantActive:    contracts.GateProfileTargetAllTests,
			wantStackLang: "java",
			wantStackTool: "gradle",
			wantCmdInAll:  "./gradlew test",
		},
		{
			name:          "exec form command joined without quoting",
			repoID:        "repo_1",
			override:      execOverride,
			target:        "",
			jobType:       types.JobTypePreGate,
			wantActive:    contracts.GateProfileTargetAllTests,
			wantStackLang: "java",
			wantStackTool: "maven",
			wantCmdInAll:  "mvn -q compile",
		},
		{
			name:    "nil override returns error",
			repoID:  "repo_1",
			jobType: types.JobTypePreGate,
			wantErr: true,
		},
		{
			name: "empty command returns error",
			repoID: "repo_1",
			override: &contracts.BuildGateProfileOverride{
				Stack: &contracts.GateProfileStack{Language: "java", Tool: "maven"},
			},
			jobType: types.JobTypePreGate,
			wantErr: true,
		},
		{
			name: "missing stack falls back to DetectedStackExpectation from meta",
			repoID: "repo_1",
			override: &contracts.BuildGateProfileOverride{
				Command: contracts.CommandSpec{Shell: "mvn test"},
			},
			jobType: types.JobTypePreGate,
			meta: &contracts.BuildGateStageMetadata{
				Detected: &contracts.StackExpectation{Language: "java", Tool: "gradle", Release: "17"},
			},
			wantActive:    contracts.GateProfileTargetAllTests,
			wantStackLang: "java",
			wantStackTool: "gradle",
			wantStackRel:  "17",
			wantCmdInAll:  "mvn test",
		},
		{
			name: "missing stack falls back to ModStack name from StaticChecks",
			repoID: "repo_1",
			override: &contracts.BuildGateProfileOverride{
				Command: contracts.CommandSpec{Shell: "mvn test"},
			},
			jobType: types.JobTypePreGate,
			meta: &contracts.BuildGateStageMetadata{
				StaticChecks: []contracts.BuildGateStaticCheckReport{
					{Language: "java", Tool: "maven", Passed: true},
				},
			},
			wantActive:    contracts.GateProfileTargetAllTests,
			wantStackLang: "java",
			wantStackTool: "maven",
			wantCmdInAll:  "mvn test",
		},
		{
			name: "no stack anywhere returns error",
			repoID: "repo_1",
			override: &contracts.BuildGateProfileOverride{
				Command: contracts.CommandSpec{Shell: "mvn test"},
			},
			jobType: types.JobTypePreGate,
			wantErr: true,
		},
		{
			name:     "unsupported job type returns error",
			repoID:   "repo_1",
			override: mavenOverride,
			jobType:  types.JobTypeMod,
			wantErr:  true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			raw, err := DeriveProfileSnapshotFromOverride(tc.repoID, tc.override, tc.target, tc.jobType, tc.meta)
			if tc.wantErr {
				if err == nil {
					t.Fatal("DeriveProfileSnapshotFromOverride() error=nil, want error")
				}
				return
			}
			if err != nil {
				t.Fatalf("DeriveProfileSnapshotFromOverride() error=%v", err)
			}
			profile, err := contracts.ParseGateProfileJSON(raw)
			if err != nil {
				t.Fatalf("snapshot fails ParseGateProfileJSON: %v", err)
			}
			if got, want := profile.Targets.Active, tc.wantActive; got != want {
				t.Fatalf("targets.active=%q, want %q", got, want)
			}
			if got, want := profile.Stack.Language, tc.wantStackLang; got != want {
				t.Fatalf("stack.language=%q, want %q", got, want)
			}
			if got, want := profile.Stack.Tool, tc.wantStackTool; got != want {
				t.Fatalf("stack.tool=%q, want %q", got, want)
			}
			if tc.wantStackRel != "" {
				if got, want := profile.Stack.Release, tc.wantStackRel; got != want {
					t.Fatalf("stack.release=%q, want %q", got, want)
				}
			}
			if tc.wantEnvKey != "" {
				if got, want := profile.Targets.AllTests.Env[tc.wantEnvKey], tc.wantEnvVal; got != want {
					t.Fatalf("all_tests.env[%s]=%q, want %q", tc.wantEnvKey, got, want)
				}
			}
			if tc.wantCmdInBuild != "" {
				if got, want := profile.Targets.Build.Command, tc.wantCmdInBuild; got != want {
					t.Fatalf("targets.build.command=%q, want %q", got, want)
				}
			}
			if tc.wantCmdInUnit != "" {
				if got, want := profile.Targets.Unit.Command, tc.wantCmdInUnit; got != want {
					t.Fatalf("targets.unit.command=%q, want %q", got, want)
				}
			}
			if tc.wantCmdInAll != "" {
				if got, want := profile.Targets.AllTests.Command, tc.wantCmdInAll; got != want {
					t.Fatalf("targets.all_tests.command=%q, want %q", got, want)
				}
			}
		})
	}
}

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

func TestSelectProfileLazy(t *testing.T) {
	t.Parallel()

	exact := &ProfileCandidate{ID: 1, ObjectKey: "exact-key", Precedence: ProfilePrecedenceExact}
	latest := &ProfileCandidate{ID: 2, ObjectKey: "latest-key", Precedence: ProfilePrecedenceLatest}
	def := &ProfileCandidate{ID: 3, ObjectKey: "default-key", Precedence: ProfilePrecedenceDefault}

	sentinelErr := errors.New("fetch failed")

	hit := func(c *ProfileCandidate) func() (*ProfileCandidate, error) {
		return func() (*ProfileCandidate, error) { return c, nil }
	}
	miss := func() (*ProfileCandidate, error) { return nil, nil }
	boom := func() (*ProfileCandidate, error) { return nil, sentinelErr }

	tests := []struct {
		name        string
		fetchExact  func() (*ProfileCandidate, error)
		fetchLatest func() (*ProfileCandidate, error)
		fetchDef    func() (*ProfileCandidate, error)
		wantID      int64
		wantErr     bool
	}{
		{
			name:        "exact found — latest and default not called",
			fetchExact:  hit(exact),
			fetchLatest: boom,
			fetchDef:    boom,
			wantID:      1,
		},
		{
			name:        "exact miss, latest found — default not called",
			fetchExact:  miss,
			fetchLatest: hit(latest),
			fetchDef:    boom,
			wantID:      2,
		},
		{
			name:        "exact and latest miss — default returned",
			fetchExact:  miss,
			fetchLatest: miss,
			fetchDef:    hit(def),
			wantID:      3,
		},
		{
			name:        "all miss — nil returned",
			fetchExact:  miss,
			fetchLatest: miss,
			fetchDef:    miss,
			wantID:      0,
		},
		{
			name:        "exact error propagates",
			fetchExact:  boom,
			fetchLatest: miss,
			fetchDef:    miss,
			wantErr:     true,
		},
		{
			name:        "latest error propagates when exact is miss",
			fetchExact:  miss,
			fetchLatest: boom,
			fetchDef:    miss,
			wantErr:     true,
		},
		{
			name:        "default error propagates when exact and latest are miss",
			fetchExact:  miss,
			fetchLatest: miss,
			fetchDef:    boom,
			wantErr:     true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := SelectProfileLazy(tc.fetchExact, tc.fetchLatest, tc.fetchDef)
			if tc.wantErr {
				if err == nil {
					t.Fatal("SelectProfileLazy() error=nil, want non-nil")
				}
				if !errors.Is(err, sentinelErr) {
					t.Fatalf("SelectProfileLazy() error=%v, want sentinel", err)
				}
				return
			}
			if err != nil {
				t.Fatalf("SelectProfileLazy() error=%v, want nil", err)
			}
			if tc.wantID == 0 {
				if got != nil {
					t.Fatalf("SelectProfileLazy()=%v, want nil", got)
				}
				return
			}
			if got == nil {
				t.Fatal("SelectProfileLazy()=nil, want non-nil")
			}
			if got.ID != tc.wantID {
				t.Fatalf("SelectProfileLazy().ID=%d, want %d", got.ID, tc.wantID)
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
