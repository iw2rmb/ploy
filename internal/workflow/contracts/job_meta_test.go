package contracts

import (
	"encoding/json"
	"testing"

	types "github.com/iw2rmb/ploy/internal/domain/types"
)

const (
	testDigestA = "sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	testDigestB = "sha256:bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"
)

func TestJobKind_Valid(t *testing.T) {
	tests := []struct {
		kind JobKind
		want bool
	}{
		{JobKindMig, true},
		{JobKindGate, true},
		{JobKindBuild, true},
		{"", false},
		{"unknown", false},
		{"MOD", false}, // case-sensitive
	}
	for _, tc := range tests {
		got := tc.kind.Valid()
		if got != tc.want {
			t.Errorf("JobKind(%q).Valid() = %v, want %v", tc.kind, got, tc.want)
		}
	}
}

func TestJobMeta_Validate(t *testing.T) {
	tests := []struct {
		name    string
		meta    JobMeta
		wantErr bool
	}{
		{
			name:    "valid mig job",
			meta:    JobMeta{Kind: JobKindMig},
			wantErr: false,
		},
		{
			name: "valid gate job with metadata",
			meta: JobMeta{
				Kind: JobKindGate,
				GateMetadata: &BuildGateStageMetadata{
					LogDigest: types.Sha256Digest(testDigestA),
					StaticChecks: []BuildGateStaticCheckReport{
						{Tool: "maven", Passed: true},
					},
				},
			},
			wantErr: false,
		},
		{
			name: "valid gate job with recovery metadata",
			meta: JobMeta{
				Kind: JobKindGate,
				RecoveryMetadata: &RecoveryJobMetadata{
					LoopKind:  "healing",
					ErrorKind: "infra",
				},
			},
			wantErr: false,
		},
		{
			name: "valid mig job with recovery metadata",
			meta: JobMeta{
				Kind: JobKindMig,
				RecoveryMetadata: &RecoveryJobMetadata{
					LoopKind:  "healing",
					ErrorKind: "code",
				},
			},
			wantErr: false,
		},
		{
			name: "valid build job with metadata",
			meta: JobMeta{
				Kind: JobKindBuild,
				Build: &BuildMeta{
					Tool:    "maven",
					Command: "mvn clean install",
				},
			},
			wantErr: false,
		},
		{
			name:    "invalid kind",
			meta:    JobMeta{Kind: "invalid"},
			wantErr: true,
		},
		{
			name: "gate metadata on mig job",
			meta: JobMeta{
				Kind:         JobKindMig,
				GateMetadata: &BuildGateStageMetadata{},
			},
			wantErr: true,
		},
		{
			name: "build metadata on mig job",
			meta: JobMeta{
				Kind:  JobKindMig,
				Build: &BuildMeta{Tool: "maven"},
			},
			wantErr: true,
		},
		{
			name: "gate metadata on build job",
			meta: JobMeta{
				Kind:         JobKindBuild,
				GateMetadata: &BuildGateStageMetadata{},
			},
			wantErr: true,
		},
		{
			name: "recovery metadata on build job",
			meta: JobMeta{
				Kind: JobKindBuild,
				RecoveryMetadata: &RecoveryJobMetadata{
					LoopKind:  "healing",
					ErrorKind: "infra",
				},
			},
			wantErr: true,
		},
		{
			name: "invalid recovery metadata",
			meta: JobMeta{
				Kind: JobKindGate,
				RecoveryMetadata: &RecoveryJobMetadata{
					LoopKind:  "healing",
					ErrorKind: "invalid",
				},
			},
			wantErr: true,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := tc.meta.Validate()
			if (err != nil) != tc.wantErr {
				t.Errorf("JobMeta.Validate() error = %v, wantErr %v", err, tc.wantErr)
			}
		})
	}
}

func TestMarshalJobMeta(t *testing.T) {
	tests := []struct {
		name    string
		meta    *JobMeta
		want    string
		wantErr bool
	}{
		{
			name:    "nil meta returns error",
			meta:    nil,
			wantErr: true,
		},
		{
			name:    "invalid kind returns error",
			meta:    &JobMeta{Kind: ""},
			wantErr: true,
		},
		{
			name: "gate metadata on mig job returns error",
			meta: &JobMeta{
				Kind:         JobKindMig,
				GateMetadata: &BuildGateStageMetadata{},
			},
			wantErr: true,
		},
		{
			name: "mig job",
			meta: &JobMeta{Kind: JobKindMig},
			want: `{"kind":"mig"}`,
		},
		{
			name: "gate job with metadata",
			meta: &JobMeta{
				Kind: JobKindGate,
				GateMetadata: &BuildGateStageMetadata{
					LogDigest: types.Sha256Digest(testDigestA),
				},
			},
			want: `{"kind":"gate","gate":{"log_digest":"` + testDigestA + `"}}`,
		},
		{
			name: "build job with metadata",
			meta: &JobMeta{
				Kind: JobKindBuild,
				Build: &BuildMeta{
					Tool:    "gradle",
					Command: "gradle build",
				},
			},
			want: `{"kind":"build","build":{"tool":"gradle","command":"gradle build"}}`,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := MarshalJobMeta(tc.meta)
			if (err != nil) != tc.wantErr {
				t.Errorf("MarshalJobMeta() error = %v, wantErr %v", err, tc.wantErr)
				return
			}
			if !tc.wantErr && string(got) != tc.want {
				t.Errorf("MarshalJobMeta() = %s, want %s", got, tc.want)
			}
		})
	}
}

func TestUnmarshalJobMeta(t *testing.T) {
	tests := []struct {
		name     string
		data     []byte
		wantKind JobKind
		wantErr  bool
	}{
		// Legacy shapes are now rejected - structured metadata is required.
		{
			name:    "empty bytes returns error",
			data:    []byte{},
			wantErr: true,
		},
		{
			name:    "empty object returns error",
			data:    []byte("{}"),
			wantErr: true,
		},
		{
			name:    "null returns error",
			data:    []byte("null"),
			wantErr: true,
		},
		{
			name:    "missing kind field returns error",
			data:    []byte(`{"gate":{"log_digest":"` + testDigestA + `"}}`),
			wantErr: true,
		},
		{
			name:    "invalid kind returns error",
			data:    []byte(`{"kind":"unknown"}`),
			wantErr: true,
		},
		{
			name:    "invalid json returns error",
			data:    []byte(`{invalid json`),
			wantErr: true,
		},
		{
			name:    "gate metadata on mig job returns error",
			data:    []byte(`{"kind":"mig","gate":{"log_digest":"` + testDigestA + `"}}`),
			wantErr: true,
		},
		// Valid structured metadata.
		{
			name:     "mig job",
			data:     []byte(`{"kind":"mig"}`),
			wantKind: JobKindMig,
		},
		{
			name:     "gate job",
			data:     []byte(`{"kind":"gate","gate":{"log_digest":"` + testDigestA + `"}}`),
			wantKind: JobKindGate,
		},
		{
			name:     "build job",
			data:     []byte(`{"kind":"build","build":{"tool":"maven"}}`),
			wantKind: JobKindBuild,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := UnmarshalJobMeta(tc.data)
			if (err != nil) != tc.wantErr {
				t.Errorf("UnmarshalJobMeta() error = %v, wantErr %v", err, tc.wantErr)
				return
			}
			if tc.wantErr {
				// Verify error message is descriptive for debugging.
				if err != nil && len(err.Error()) < 10 {
					t.Errorf("UnmarshalJobMeta() error message too short: %v", err)
				}
				return
			}
			if got == nil {
				t.Error("UnmarshalJobMeta() = nil, want non-nil")
				return
			}
			if got.Kind != tc.wantKind {
				t.Errorf("UnmarshalJobMeta().Kind = %v, want %v", got.Kind, tc.wantKind)
			}
		})
	}
}

func TestJobMeta_RoundTrip(t *testing.T) {
	// Test that marshaling and unmarshaling preserves data.
	original := &JobMeta{
		Kind: JobKindGate,
		GateMetadata: &BuildGateStageMetadata{
			LogDigest: types.Sha256Digest(testDigestB),
			StaticChecks: []BuildGateStaticCheckReport{
				{
					Tool:   "maven",
					Passed: false,
					Failures: []BuildGateStaticCheckFailure{
						{
							File:     "src/Main.java",
							Line:     42,
							Severity: "error",
							Message:  "compilation failed",
						},
					},
				},
			},
			LogFindings: []BuildGateLogFinding{
				{
					Code:     "E001",
					Severity: "warning",
					Message:  "deprecated API usage",
				},
			},
		},
	}

	// Marshal to JSON.
	data, err := MarshalJobMeta(original)
	if err != nil {
		t.Fatalf("MarshalJobMeta() error = %v", err)
	}

	// Unmarshal back.
	got, err := UnmarshalJobMeta(data)
	if err != nil {
		t.Fatalf("UnmarshalJobMeta() error = %v", err)
	}

	// Verify key fields.
	if got.Kind != original.Kind {
		t.Errorf("Kind = %v, want %v", got.Kind, original.Kind)
	}
	if got.GateMetadata == nil {
		t.Fatal("GateMetadata = nil, want non-nil")
	}
	if got.GateMetadata.LogDigest != original.GateMetadata.LogDigest {
		t.Errorf("GateMetadata.LogDigest = %v, want %v", got.GateMetadata.LogDigest, original.GateMetadata.LogDigest)
	}
	if len(got.GateMetadata.StaticChecks) != len(original.GateMetadata.StaticChecks) {
		t.Errorf("GateMetadata.StaticChecks len = %d, want %d", len(got.GateMetadata.StaticChecks), len(original.GateMetadata.StaticChecks))
	}
	if len(got.GateMetadata.LogFindings) != len(original.GateMetadata.LogFindings) {
		t.Errorf("GateMetadata.LogFindings len = %d, want %d", len(got.GateMetadata.LogFindings), len(original.GateMetadata.LogFindings))
	}
}

func TestNewJobMetaConstructors(t *testing.T) {
	t.Run("NewMigJobMeta", func(t *testing.T) {
		m := NewMigJobMeta()
		if m.Kind != JobKindMig {
			t.Errorf("Kind = %v, want %v", m.Kind, JobKindMig)
		}
		if m.GateMetadata != nil {
			t.Error("GateMetadata should be nil")
		}
		if m.Build != nil {
			t.Error("Build should be nil")
		}
	})

	t.Run("NewGateJobMeta", func(t *testing.T) {
		gate := &BuildGateStageMetadata{LogDigest: types.Sha256Digest(testDigestA)}
		m := NewGateJobMeta(gate)
		if m.Kind != JobKindGate {
			t.Errorf("Kind = %v, want %v", m.Kind, JobKindGate)
		}
		if m.GateMetadata != gate {
			t.Error("GateMetadata should be the provided metadata")
		}
		if m.Build != nil {
			t.Error("Build should be nil")
		}
	})

	t.Run("NewBuildJobMeta", func(t *testing.T) {
		build := &BuildMeta{Tool: "maven", Command: "mvn clean"}
		m := NewBuildJobMeta(build)
		if m.Kind != JobKindBuild {
			t.Errorf("Kind = %v, want %v", m.Kind, JobKindBuild)
		}
		if m.Build != build {
			t.Error("Build should be the provided metadata")
		}
		if m.GateMetadata != nil {
			t.Error("GateMetadata should be nil")
		}
	})
}

func TestBuildMeta_JSON(t *testing.T) {
	// Test BuildMeta with metrics map.
	bm := &BuildMeta{
		Tool:          "gradle",
		Command:       "gradle build",
		StatusDetails: "build succeeded",
		Metrics: map[string]interface{}{
			"compilation_time_ms": 1234.0,
			"test_count":          42.0,
			"passed":              true,
		},
	}

	data, err := json.Marshal(bm)
	if err != nil {
		t.Fatalf("json.Marshal() error = %v", err)
	}

	var got BuildMeta
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatalf("json.Unmarshal() error = %v", err)
	}

	if got.Tool != bm.Tool {
		t.Errorf("Tool = %v, want %v", got.Tool, bm.Tool)
	}
	if got.Command != bm.Command {
		t.Errorf("Command = %v, want %v", got.Command, bm.Command)
	}
	if got.StatusDetails != bm.StatusDetails {
		t.Errorf("StatusDetails = %v, want %v", got.StatusDetails, bm.StatusDetails)
	}
	if len(got.Metrics) != len(bm.Metrics) {
		t.Errorf("Metrics len = %d, want %d", len(got.Metrics), len(bm.Metrics))
	}
}

func TestJobMeta_ActionSummary_Valid(t *testing.T) {
	t.Parallel()
	m := &JobMeta{
		Kind:          JobKindMig,
		ActionSummary: "Fixed missing import in Main.java",
	}
	if err := m.Validate(); err != nil {
		t.Errorf("Validate() unexpected error: %v", err)
	}

	// Verify round-trip.
	data, err := json.Marshal(m)
	if err != nil {
		t.Fatalf("Marshal() error: %v", err)
	}
	var decoded JobMeta
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("Unmarshal() error: %v", err)
	}
	if decoded.ActionSummary != m.ActionSummary {
		t.Errorf("ActionSummary round-trip: got %q, want %q", decoded.ActionSummary, m.ActionSummary)
	}
}

func TestJobMeta_ActionSummary_OnGateJob_Rejected(t *testing.T) {
	t.Parallel()
	m := &JobMeta{
		Kind:          JobKindGate,
		GateMetadata:  &BuildGateStageMetadata{},
		ActionSummary: "should not be here",
	}
	err := m.Validate()
	if err == nil {
		t.Fatal("expected validation error for action_summary on gate job")
	}
}

func TestJobMeta_ActionSummary_TooLong(t *testing.T) {
	t.Parallel()
	long := ""
	for i := 0; i < 201; i++ {
		long += "x"
	}
	m := &JobMeta{
		Kind:          JobKindMig,
		ActionSummary: long,
	}
	err := m.Validate()
	if err == nil {
		t.Fatal("expected validation error for >200 char action_summary")
	}
}

func TestJobMeta_Recovery_RoundTrip(t *testing.T) {
	t.Parallel()
	original := &JobMeta{
		Kind: JobKindMig,
		RecoveryMetadata: &RecoveryJobMetadata{
			LoopKind:   "healing",
			ErrorKind:  "code",
			StrategyID: "code-default",
			Reason:     "compile failure persisted after gate",
		},
	}
	data, err := MarshalJobMeta(original)
	if err != nil {
		t.Fatalf("MarshalJobMeta() error = %v", err)
	}
	got, err := UnmarshalJobMeta(data)
	if err != nil {
		t.Fatalf("UnmarshalJobMeta() error = %v", err)
	}
	if got.RecoveryMetadata == nil {
		t.Fatal("RecoveryMetadata = nil, want non-nil")
	}
	if got.RecoveryMetadata.LoopKind != original.RecoveryMetadata.LoopKind {
		t.Fatalf("RecoveryMetadata.LoopKind = %q, want %q", got.RecoveryMetadata.LoopKind, original.RecoveryMetadata.LoopKind)
	}
	if got.RecoveryMetadata.ErrorKind != original.RecoveryMetadata.ErrorKind {
		t.Fatalf("RecoveryMetadata.ErrorKind = %q, want %q", got.RecoveryMetadata.ErrorKind, original.RecoveryMetadata.ErrorKind)
	}
	if got.RecoveryMetadata.StrategyID != original.RecoveryMetadata.StrategyID {
		t.Fatalf("RecoveryMetadata.StrategyID = %q, want %q", got.RecoveryMetadata.StrategyID, original.RecoveryMetadata.StrategyID)
	}
}

func TestUnmarshalJobMeta_RecoveryOnBuildJobRejected(t *testing.T) {
	t.Parallel()
	_, err := UnmarshalJobMeta([]byte(`{"kind":"build","recovery":{"loop_kind":"healing","error_kind":"infra"}}`))
	if err == nil {
		t.Fatal("expected error for recovery metadata on build job")
	}
}
