package contracts

import (
	"encoding/json"
	"testing"
)

func TestJobKind_Valid(t *testing.T) {
	tests := []struct {
		kind JobKind
		want bool
	}{
		{JobKindMod, true},
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
			name:    "valid mod job",
			meta:    JobMeta{Kind: JobKindMod},
			wantErr: false,
		},
		{
			name: "valid gate job with metadata",
			meta: JobMeta{
				Kind: JobKindGate,
				Gate: &BuildGateStageMetadata{
					LogDigest: "sha256:abc123",
					StaticChecks: []BuildGateStaticCheckReport{
						{Tool: "maven", Passed: true},
					},
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
			name: "gate metadata on mod job",
			meta: JobMeta{
				Kind: JobKindMod,
				Gate: &BuildGateStageMetadata{},
			},
			wantErr: true,
		},
		{
			name: "build metadata on mod job",
			meta: JobMeta{
				Kind:  JobKindMod,
				Build: &BuildMeta{Tool: "maven"},
			},
			wantErr: true,
		},
		{
			name: "gate metadata on build job",
			meta: JobMeta{
				Kind: JobKindBuild,
				Gate: &BuildGateStageMetadata{},
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
			name: "nil meta returns empty object",
			meta: nil,
			want: "{}",
		},
		{
			name: "mod job",
			meta: &JobMeta{Kind: JobKindMod},
			want: `{"kind":"mod"}`,
		},
		{
			name: "gate job with metadata",
			meta: &JobMeta{
				Kind: JobKindGate,
				Gate: &BuildGateStageMetadata{
					LogDigest: "sha256:abc",
				},
			},
			want: `{"kind":"gate","gate":{"log_digest":"sha256:abc"}}`,
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
			if string(got) != tc.want {
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
		wantNil  bool
		wantErr  bool
	}{
		{
			name:    "empty bytes returns nil",
			data:    []byte{},
			wantNil: true,
		},
		{
			name:    "empty object returns nil",
			data:    []byte("{}"),
			wantNil: true,
		},
		{
			name:    "null returns nil",
			data:    []byte("null"),
			wantNil: true,
		},
		{
			name:     "mod job",
			data:     []byte(`{"kind":"mod"}`),
			wantKind: JobKindMod,
		},
		{
			name:     "gate job",
			data:     []byte(`{"kind":"gate","gate":{"log_digest":"sha256:abc"}}`),
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
			if tc.wantNil {
				if got != nil {
					t.Errorf("UnmarshalJobMeta() = %v, want nil", got)
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
		Gate: &BuildGateStageMetadata{
			LogDigest: "sha256:abc123",
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
	if got.Gate == nil {
		t.Fatal("Gate = nil, want non-nil")
	}
	if got.Gate.LogDigest != original.Gate.LogDigest {
		t.Errorf("Gate.LogDigest = %v, want %v", got.Gate.LogDigest, original.Gate.LogDigest)
	}
	if len(got.Gate.StaticChecks) != len(original.Gate.StaticChecks) {
		t.Errorf("Gate.StaticChecks len = %d, want %d", len(got.Gate.StaticChecks), len(original.Gate.StaticChecks))
	}
	if len(got.Gate.LogFindings) != len(original.Gate.LogFindings) {
		t.Errorf("Gate.LogFindings len = %d, want %d", len(got.Gate.LogFindings), len(original.Gate.LogFindings))
	}
}

func TestNewJobMetaConstructors(t *testing.T) {
	t.Run("NewModJobMeta", func(t *testing.T) {
		m := NewModJobMeta()
		if m.Kind != JobKindMod {
			t.Errorf("Kind = %v, want %v", m.Kind, JobKindMod)
		}
		if m.Gate != nil {
			t.Error("Gate should be nil")
		}
		if m.Build != nil {
			t.Error("Build should be nil")
		}
	})

	t.Run("NewGateJobMeta", func(t *testing.T) {
		gate := &BuildGateStageMetadata{LogDigest: "sha256:test"}
		m := NewGateJobMeta(gate)
		if m.Kind != JobKindGate {
			t.Errorf("Kind = %v, want %v", m.Kind, JobKindGate)
		}
		if m.Gate != gate {
			t.Error("Gate should be the provided metadata")
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
		if m.Gate != nil {
			t.Error("Gate should be nil")
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
