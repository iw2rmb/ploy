package gateprofile

import (
	"errors"
	"testing"

	types "github.com/iw2rmb/ploy/internal/domain/types"
	"github.com/iw2rmb/ploy/internal/workflow/contracts"
)

func TestDeriveProfileSnapshotFromOverride(t *testing.T) {
	t.Parallel()

	baseOverride := &contracts.BuildGateProfileOverride{
		Command: contracts.CommandSpec{Shell: "mvn -q test"},
		Env:     map[string]string{"MAVEN_OPTS": "-Xmx2g"},
		Stack:   &contracts.GateProfileStack{Language: "java", Tool: "maven", Release: "21"},
		Target:  contracts.GateProfileTargetAllTests,
	}

	tests := []struct {
		name     string
		override *contracts.BuildGateProfileOverride
		target   string
		jobType  types.JobType
		meta     *contracts.BuildGateStageMetadata
		wantErr  bool
		assert   func(t *testing.T, p *contracts.GateProfile)
	}{
		{
			name:     "override target defaults to all_tests",
			override: baseOverride,
			jobType:  types.JobTypePreGate,
			assert: func(t *testing.T, p *contracts.GateProfile) {
				t.Helper()
				if p.Targets.Active != contracts.GateProfileTargetAllTests {
					t.Fatalf("active=%q", p.Targets.Active)
				}
				if p.Targets.AllTests.Command != "mvn -q test" {
					t.Fatalf("all_tests.command=%q", p.Targets.AllTests.Command)
				}
				if p.Stack.Release != "21" {
					t.Fatalf("stack.release=%q", p.Stack.Release)
				}
			},
		},
		{
			name:     "explicit target overrides override target",
			override: baseOverride,
			target:   contracts.GateProfileTargetBuild,
			jobType:  types.JobTypePreGate,
			assert: func(t *testing.T, p *contracts.GateProfile) {
				t.Helper()
				if p.Targets.Active != contracts.GateProfileTargetBuild {
					t.Fatalf("active=%q", p.Targets.Active)
				}
				if p.Targets.Build.Command != "mvn -q test" {
					t.Fatalf("build.command=%q", p.Targets.Build.Command)
				}
			},
		},
		{
			name: "exec command is normalized",
			override: &contracts.BuildGateProfileOverride{
				Command: contracts.CommandSpec{Exec: []string{"mvn", "-q", "compile"}},
				Stack:   &contracts.GateProfileStack{Language: "java", Tool: "maven"},
			},
			jobType: types.JobTypePostGate,
			assert: func(t *testing.T, p *contracts.GateProfile) {
				t.Helper()
				if p.Targets.AllTests.Command != "mvn -q compile" {
					t.Fatalf("all_tests.command=%q", p.Targets.AllTests.Command)
				}
			},
		},
		{
			name: "stack falls back to detected expectation",
			override: &contracts.BuildGateProfileOverride{
				Command: contracts.CommandSpec{Shell: "mvn test"},
			},
			jobType: types.JobTypePreGate,
			meta: &contracts.BuildGateStageMetadata{
				Detected: &contracts.StackExpectation{Language: "java", Tool: "gradle", Release: "17"},
			},
			assert: func(t *testing.T, p *contracts.GateProfile) {
				t.Helper()
				if p.Stack.Tool != "gradle" || p.Stack.Release != "17" {
					t.Fatalf("stack=%+v", p.Stack)
				}
			},
		},
		{
			name: "regate is accepted",
			override: &contracts.BuildGateProfileOverride{
				Command: contracts.CommandSpec{Shell: "./gradlew test"},
				Stack:   &contracts.GateProfileStack{Language: "java", Tool: "gradle"},
			},
			jobType: types.JobTypeReGate,
			assert: func(t *testing.T, p *contracts.GateProfile) {
				t.Helper()
				if p.Targets.Active != contracts.GateProfileTargetAllTests {
					t.Fatalf("active=%q", p.Targets.Active)
				}
			},
		},
		{
			name:    "nil override fails",
			jobType: types.JobTypePreGate,
			wantErr: true,
		},
		{
			name: "empty command fails",
			override: &contracts.BuildGateProfileOverride{
				Stack: &contracts.GateProfileStack{Language: "java", Tool: "maven"},
			},
			jobType: types.JobTypePreGate,
			wantErr: true,
		},
		{
			name: "no stack source fails",
			override: &contracts.BuildGateProfileOverride{
				Command: contracts.CommandSpec{Shell: "mvn test"},
			},
			jobType: types.JobTypePreGate,
			wantErr: true,
		},
		{
			name:     "unsupported job type fails",
			override: baseOverride,
			jobType:  types.JobTypeMig,
			wantErr:  true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			raw, err := DeriveProfileSnapshotFromOverride("repo_1", tc.override, tc.target, tc.jobType, tc.meta)
			if tc.wantErr {
				if err == nil {
					t.Fatal("error=nil, want non-nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("DeriveProfileSnapshotFromOverride() error=%v", err)
			}
			profile, err := contracts.ParseGateProfileJSON(raw)
			if err != nil {
				t.Fatalf("ParseGateProfileJSON() error=%v", err)
			}
			if tc.assert != nil {
				tc.assert(t, profile)
			}
		})
	}
}

func TestProfilePrecedenceOrder(t *testing.T) {
	t.Parallel()
	if !(ProfilePrecedenceExact > ProfilePrecedenceLatest) {
		t.Fatalf("exact=%d latest=%d", ProfilePrecedenceExact, ProfilePrecedenceLatest)
	}
	if !(ProfilePrecedenceLatest > ProfilePrecedenceDefault) {
		t.Fatalf("latest=%d default=%d", ProfilePrecedenceLatest, ProfilePrecedenceDefault)
	}
}

func TestSelectProfile(t *testing.T) {
	t.Parallel()
	exact := &ProfileCandidate{ID: 1, ObjectKey: "exact", Precedence: ProfilePrecedenceExact}
	latest := &ProfileCandidate{ID: 2, ObjectKey: "latest", Precedence: ProfilePrecedenceLatest}
	def := &ProfileCandidate{ID: 3, ObjectKey: "default", Precedence: ProfilePrecedenceDefault}

	cases := []struct {
		name string
		ex   *ProfileCandidate
		la   *ProfileCandidate
		de   *ProfileCandidate
		want *ProfileCandidate
	}{
		{name: "exact wins", ex: exact, la: latest, de: def, want: exact},
		{name: "latest fallback", la: latest, de: def, want: latest},
		{name: "default fallback", de: def, want: def},
		{name: "all nil", want: nil},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := SelectProfile(tc.ex, tc.la, tc.de)
			if got != tc.want {
				t.Fatalf("SelectProfile()=%v, want %v", got, tc.want)
			}
		})
	}
}

func TestSelectProfileLazy(t *testing.T) {
	t.Parallel()
	exact := &ProfileCandidate{ID: 1}
	latest := &ProfileCandidate{ID: 2}
	def := &ProfileCandidate{ID: 3}
	sentinel := errors.New("fetch failed")

	hit := func(c *ProfileCandidate) func() (*ProfileCandidate, error) {
		return func() (*ProfileCandidate, error) { return c, nil }
	}
	miss := func() (*ProfileCandidate, error) { return nil, nil }
	boom := func() (*ProfileCandidate, error) { return nil, sentinel }

	cases := []struct {
		name        string
		fetchExact  func() (*ProfileCandidate, error)
		fetchLatest func() (*ProfileCandidate, error)
		fetchDef    func() (*ProfileCandidate, error)
		want        *ProfileCandidate
		wantErr     bool
	}{
		{name: "exact short-circuits", fetchExact: hit(exact), fetchLatest: boom, fetchDef: boom, want: exact},
		{name: "latest short-circuits default", fetchExact: miss, fetchLatest: hit(latest), fetchDef: boom, want: latest},
		{name: "default fallback", fetchExact: miss, fetchLatest: miss, fetchDef: hit(def), want: def},
		{name: "all miss", fetchExact: miss, fetchLatest: miss, fetchDef: miss, want: nil},
		{name: "error propagates", fetchExact: boom, fetchLatest: miss, fetchDef: miss, wantErr: true},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := SelectProfileLazy(tc.fetchExact, tc.fetchLatest, tc.fetchDef)
			if tc.wantErr {
				if !errors.Is(err, sentinel) {
					t.Fatalf("error=%v, want sentinel", err)
				}
				return
			}
			if err != nil {
				t.Fatalf("error=%v", err)
			}
			if got != tc.want {
				t.Fatalf("got=%v want=%v", got, tc.want)
			}
		})
	}
}

func TestGateOverrideForJobType(t *testing.T) {
	t.Parallel()

	base := testGateProfile()
	base.Targets.Active = contracts.GateProfileTargetBuild
	base.Targets.Build.Status = contracts.PrepTargetStatusPassed
	base.Targets.Build.Command = "go test ./..."
	base.Targets.Build.Env = map[string]string{"GOFLAGS": "-mod=readonly"}

	unsupported := testGateProfile()
	unsupported.Targets.Active = contracts.GateProfileTargetUnsupported

	hostSocket := testGateProfile()
	hostSocket.Targets.Active = contracts.GateProfileTargetBuild
	hostSocket.Targets.Build.Command = "go test ./..."
	hostSocket.Runtime = &contracts.GateProfileRuntime{
		Docker: &contracts.GateProfileRuntimeDocker{Mode: contracts.GateProfileDockerModeHostSocket, APIVersion: "1.45"},
	}

	tcp := testGateProfile()
	tcp.Targets.Active = contracts.GateProfileTargetBuild
	tcp.Targets.Build.Command = "go test ./..."
	tcp.Runtime = &contracts.GateProfileRuntime{
		Docker: &contracts.GateProfileRuntimeDocker{Mode: contracts.GateProfileDockerModeTCP, Host: "tcp://dind:2375"},
	}

	failedUnit := testGateProfile()
	failedUnit.Targets.Active = contracts.GateProfileTargetUnit
	failedUnit.Targets.Unit = &contracts.GateProfileTarget{Status: contracts.PrepTargetStatusFailed, Command: "go test ./... -run Unit", Env: map[string]string{}}

	cases := []struct {
		name      string
		profile   *contracts.GateProfile
		jobType   types.JobType
		wantPhase contracts.BuildGateProfilePhase
		wantNil   bool
		assert    func(t *testing.T, o *contracts.BuildGateProfileOverride)
	}{
		{name: "pre maps to pre", profile: base, jobType: types.JobTypePreGate, wantPhase: contracts.BuildGateProfilePhasePre,
			assert: func(t *testing.T, o *contracts.BuildGateProfileOverride) {
				t.Helper()
				if o.Command.Shell != "go test ./..." || o.Env["GOFLAGS"] != "-mod=readonly" {
					t.Fatalf("override=%+v", o)
				}
			}},
		{name: "post maps to post", profile: base, jobType: types.JobTypePostGate, wantPhase: contracts.BuildGateProfilePhasePost},
		{name: "regate maps to post", profile: base, jobType: types.JobTypeReGate, wantPhase: contracts.BuildGateProfilePhasePost},
		{name: "non gate job returns nil", profile: base, jobType: types.JobTypeMig, wantNil: true},
		{name: "nil profile returns nil", profile: nil, jobType: types.JobTypePreGate, wantNil: true},
		{name: "unsupported active target returns nil override", profile: unsupported, jobType: types.JobTypePreGate, wantPhase: contracts.BuildGateProfilePhasePre, wantNil: true},
		{name: "host_socket injects docker env", profile: hostSocket, jobType: types.JobTypePreGate, wantPhase: contracts.BuildGateProfilePhasePre,
			assert: func(t *testing.T, o *contracts.BuildGateProfileOverride) {
				t.Helper()
				if o.Env[contracts.GateProfileDockerHostEnv] != defaultDockerHostSocket {
					t.Fatalf("DOCKER_HOST=%q", o.Env[contracts.GateProfileDockerHostEnv])
				}
				if o.Env[contracts.GateProfileDockerAPIVersionEnv] != "1.45" {
					t.Fatalf("DOCKER_API_VERSION=%q", o.Env[contracts.GateProfileDockerAPIVersionEnv])
				}
			}},
		{name: "tcp runtime injects host", profile: tcp, jobType: types.JobTypePreGate, wantPhase: contracts.BuildGateProfilePhasePre,
			assert: func(t *testing.T, o *contracts.BuildGateProfileOverride) {
				t.Helper()
				if o.Env[contracts.GateProfileDockerHostEnv] != "tcp://dind:2375" {
					t.Fatalf("DOCKER_HOST=%q", o.Env[contracts.GateProfileDockerHostEnv])
				}
			}},
		{name: "failed active target command is still used", profile: failedUnit, jobType: types.JobTypePreGate, wantPhase: contracts.BuildGateProfilePhasePre,
			assert: func(t *testing.T, o *contracts.BuildGateProfileOverride) {
				t.Helper()
				if o.Command.Shell != "go test ./... -run Unit" {
					t.Fatalf("command=%q", o.Command.Shell)
				}
			}},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			phase, override, err := GateOverrideForJobType(tc.profile, tc.jobType)
			if err != nil {
				t.Fatalf("error=%v", err)
			}
			if tc.wantNil {
				if override != nil {
					t.Fatalf("override=%v, want nil", override)
				}
				if tc.wantPhase != "" && phase != tc.wantPhase {
					t.Fatalf("phase=%q want=%q", phase, tc.wantPhase)
				}
				return
			}
			if phase != tc.wantPhase {
				t.Fatalf("phase=%q want=%q", phase, tc.wantPhase)
			}
			if override == nil {
				t.Fatal("override=nil")
			}
			if tc.assert != nil {
				tc.assert(t, override)
			}
		})
	}
}

func TestStackMatches(t *testing.T) {
	t.Parallel()

	withRelease := testGateProfile()
	withRelease.Stack = contracts.GateProfileStack{Language: "java", Tool: "gradle", Release: "11"}
	noRelease := testGateProfile()
	noRelease.Stack = contracts.GateProfileStack{Language: "java", Tool: "gradle"}

	cases := []struct {
		name    string
		profile *contracts.GateProfile
		lang    string
		tool    string
		rel     string
		want    bool
	}{
		{name: "nil profile false", profile: nil, lang: "java", tool: "gradle", want: false},
		{name: "release wildcard", profile: withRelease, lang: "java", tool: "gradle", rel: "", want: true},
		{name: "exact release", profile: withRelease, lang: "java", tool: "gradle", rel: "11", want: true},
		{name: "release mismatch", profile: withRelease, lang: "java", tool: "gradle", rel: "17", want: false},
		{name: "non-empty release requires profile release", profile: noRelease, lang: "java", tool: "gradle", rel: "11", want: false},
		{name: "language mismatch", profile: withRelease, lang: "go", tool: "gradle", rel: "", want: false},
		{name: "tool mismatch", profile: withRelease, lang: "java", tool: "maven", rel: "", want: false},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := StackMatches(tc.profile, tc.lang, tc.tool, tc.rel); got != tc.want {
				t.Fatalf("StackMatches()=%v want=%v", got, tc.want)
			}
		})
	}
}

func testGateProfile() *contracts.GateProfile {
	return &contracts.GateProfile{
		SchemaVersion: 1,
		RepoID:        "repo_1",
		RunnerMode:    contracts.PrepRunnerModeSimple,
		Stack:         contracts.GateProfileStack{Language: "go", Tool: "go"},
		Targets: contracts.GateProfileTargets{
			Active:   contracts.GateProfileTargetBuild,
			Build:    &contracts.GateProfileTarget{Status: contracts.PrepTargetStatusNotAttempted, Env: map[string]string{}},
			Unit:     &contracts.GateProfileTarget{Status: contracts.PrepTargetStatusNotAttempted, Env: map[string]string{}},
			AllTests: &contracts.GateProfileTarget{Status: contracts.PrepTargetStatusNotAttempted, Env: map[string]string{}},
		},
		Orchestration: contracts.GateProfileOrchestration{},
	}
}
