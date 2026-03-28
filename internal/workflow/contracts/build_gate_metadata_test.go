package contracts

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"
)

func confidencePtr(v float64) *float64 {
	return &v
}

func boolPtr(v bool) *bool {
	return &v
}

// TestBuildGateStageMetadata_DetectedStack verifies that DetectedStack correctly
// derives the MigStack from the first static check's tool name.
func TestBuildGateStageMetadata_DetectedStack(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		meta BuildGateStageMetadata
		want MigStack
	}{
		{
			name: "maven tool detected",
			meta: BuildGateStageMetadata{
				StaticChecks: []BuildGateStaticCheckReport{
					{Language: "java", Tool: "maven", Passed: true},
				},
			},
			want: MigStackJavaMaven,
		},
		{
			name: "gradle tool detected",
			meta: BuildGateStageMetadata{
				StaticChecks: []BuildGateStaticCheckReport{
					{Language: "java", Tool: "gradle", Passed: false},
				},
			},
			want: MigStackJavaGradle,
		},
		{
			name: "java tool detected",
			meta: BuildGateStageMetadata{
				StaticChecks: []BuildGateStaticCheckReport{
					{Language: "java", Tool: "java", Passed: true},
				},
			},
			want: MigStackJava,
		},
		{
			name: "none tool (gate skipped)",
			meta: BuildGateStageMetadata{
				StaticChecks: []BuildGateStaticCheckReport{
					{Tool: "none", Passed: true},
				},
			},
			want: MigStackUnknown,
		},
		{
			name: "empty static checks",
			meta: BuildGateStageMetadata{
				StaticChecks: []BuildGateStaticCheckReport{},
			},
			want: MigStackUnknown,
		},
		{
			name: "nil static checks",
			meta: BuildGateStageMetadata{},
			want: MigStackUnknown,
		},
		{
			name: "multiple checks uses first",
			meta: BuildGateStageMetadata{
				StaticChecks: []BuildGateStaticCheckReport{
					{Language: "java", Tool: "maven", Passed: true},
					{Language: "java", Tool: "gradle", Passed: true},
				},
			},
			want: MigStackJavaMaven,
		},
		{
			name: "detected_stack tool takes precedence",
			meta: BuildGateStageMetadata{
				Detected: &StackExpectation{
					Language: "java",
					Tool:     "gradle",
					Release:  "11",
				},
				StaticChecks: []BuildGateStaticCheckReport{
					{Language: "java", Tool: "maven", Passed: true},
				},
			},
			want: MigStackJavaGradle,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := tt.meta.DetectedStack()
			if got != tt.want {
				t.Errorf("DetectedStack() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestBuildGateStageMetadata_DetectedStackExpectation(t *testing.T) {
	t.Parallel()

	t.Run("from detected_stack", func(t *testing.T) {
		t.Parallel()
		meta := BuildGateStageMetadata{
			Detected: &StackExpectation{
				Language: "java",
				Tool:     "gradle",
				Release:  "11",
			},
		}
		got := meta.DetectedStackExpectation()
		if got == nil {
			t.Fatal("DetectedStackExpectation() = nil, want non-nil")
		}
		if got.Language != "java" || got.Tool != "gradle" || got.Release != "11" {
			t.Fatalf("DetectedStackExpectation() = %+v, want java/gradle/11", *got)
		}
	})

	t.Run("fallback to static checks", func(t *testing.T) {
		t.Parallel()
		meta := BuildGateStageMetadata{
			StaticChecks: []BuildGateStaticCheckReport{
				{Language: "java", Tool: "maven", Passed: true},
			},
		}
		got := meta.DetectedStackExpectation()
		if got == nil {
			t.Fatal("DetectedStackExpectation() = nil, want non-nil")
		}
		if got.Language != "java" || got.Tool != "maven" || got.Release != "" {
			t.Fatalf("DetectedStackExpectation() = %+v, want java/maven/\"\"", *got)
		}
	})
}

func TestBuildGateStageMetadata_Validate_DetectedStack(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		meta    BuildGateStageMetadata
		wantErr string
	}{
		{
			name: "valid detected stack",
			meta: BuildGateStageMetadata{
				Detected: &StackExpectation{Language: "java", Tool: "gradle", Release: "11"},
			},
		},
		{
			name: "missing detected language",
			meta: BuildGateStageMetadata{
				Detected: &StackExpectation{Tool: "gradle", Release: "11"},
			},
			wantErr: "detected_stack.language",
		},
		{
			name: "missing detected tool",
			meta: BuildGateStageMetadata{
				Detected: &StackExpectation{Language: "java", Release: "11"},
			},
			wantErr: "detected_stack.tool",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			requireValidationErr(t, tt.meta.Validate(), tt.wantErr)
		})
	}
}

// TestStackGateResult_Validate verifies StackGateResult validation logic.
func TestStackGateResult_Validate(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		result    StackGateResult
		errSubstr string
	}{
		{name: "nil stack gate is valid", result: StackGateResult{}},
		{name: "disabled with no result is valid", result: StackGateResult{Enabled: false}},
		{name: "disabled with any result is valid", result: StackGateResult{Enabled: false, Result: "anything"}},
		{name: "enabled with pass result is valid", result: StackGateResult{Enabled: true, Result: "pass"}},
		{name: "enabled with mismatch result is valid", result: StackGateResult{Enabled: true, Result: "mismatch"}},
		{name: "enabled with unknown result is valid", result: StackGateResult{Enabled: true, Result: "unknown"}},
		{name: "enabled with empty result is invalid", result: StackGateResult{Enabled: true, Result: ""}, errSubstr: "result required"},
		{name: "enabled with invalid result is invalid", result: StackGateResult{Enabled: true, Result: "invalid"}, errSubstr: "result invalid"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			requireValidationErr(t, tt.result.Validate(), tt.errSubstr)
		})
	}
}

// TestBuildGateStageMetadata_Validate_StackGate verifies that metadata validation
// includes stack gate result validation.
func TestBuildGateStageMetadata_Validate_StackGate(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		meta      BuildGateStageMetadata
		errSubstr string
	}{
		{
			name: "nil stack gate is valid",
			meta: BuildGateStageMetadata{
				StaticChecks: []BuildGateStaticCheckReport{{Tool: "maven", Passed: true}},
			},
		},
		{
			name: "valid stack gate result is valid",
			meta: BuildGateStageMetadata{
				StaticChecks: []BuildGateStaticCheckReport{{Tool: "stack-gate", Passed: true}},
				StackGate:    &StackGateResult{Enabled: true, Result: "pass"},
			},
		},
		{
			name: "invalid stack gate result causes validation failure",
			meta: BuildGateStageMetadata{
				StaticChecks: []BuildGateStaticCheckReport{{Tool: "stack-gate", Passed: false}},
				StackGate:    &StackGateResult{Enabled: true, Result: ""},
			},
			errSubstr: "stack_gate invalid",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			requireValidationErr(t, tt.meta.Validate(), tt.errSubstr)
		})
	}
}

func TestBuildGateStageMetadata_JSONRoundTrip(t *testing.T) {
	t.Parallel()

	t.Run("stack gate fields", func(t *testing.T) {
		t.Parallel()
		original := BuildGateStageMetadata{
			StaticChecks: []BuildGateStaticCheckReport{{
				Language: "java",
				Tool:     "stack-gate",
				Passed:   true,
			}},
			Detected: &StackExpectation{
				Language: "java",
				Tool:     "maven",
				Release:  "17",
			},
			StackGate: &StackGateResult{
				Enabled: true,
				Expected: &StackExpectation{
					Language: "java",
					Tool:     "maven",
					Release:  "17",
				},
				Detected: &StackExpectation{
					Language: "java",
					Tool:     "maven",
					Release:  "17",
				},
				RuntimeImage: "maven:3-eclipse-temurin-17",
				Result:       "pass",
			},
		}
		requireJSONRoundTrip(t, original)
	})

	t.Run("recovery fields", func(t *testing.T) {
		t.Parallel()
		original := BuildGateStageMetadata{
			BugSummary: "Missing semicolon on line 42",
			Recovery: &BuildGateRecoveryMetadata{
				LoopKind:     "healing",
				ErrorKind:    "infra",
				StrategyID:   "infra-default",
				Confidence:   confidencePtr(0.75),
				Reason:       "docker daemon unavailable",
				Expectations: json.RawMessage(`{"artifacts":[{"path":"/out/gate-profile-candidate.json","schema":"gate_profile_v1"}]}`),
			},
		}
		requireJSONRoundTrip(t, original)
	})
}

func requireJSONRoundTrip(t *testing.T, original BuildGateStageMetadata) {
	t.Helper()
	data, err := json.Marshal(original)
	if err != nil {
		t.Fatalf("Marshal() error: %v", err)
	}
	var decoded BuildGateStageMetadata
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("Unmarshal() error: %v", err)
	}
	if !reflect.DeepEqual(decoded, original) {
		t.Fatalf("roundtrip mismatch:\n got: %+v\nwant: %+v", decoded, original)
	}
}

func TestBuildGateStageMetadata_BugSummary_Validation(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		bugSummary string
		wantSubstr string
	}{
		{name: "valid", bugSummary: "Missing semicolon on line 42 of Main.java"},
		{name: "too long", bugSummary: strings.Repeat("x", 201), wantSubstr: "bug_summary"},
		{name: "multiline", bugSummary: "line one\nline two", wantSubstr: "single-line"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			meta := BuildGateStageMetadata{BugSummary: tt.bugSummary}
			requireValidationErr(t, meta.Validate(), tt.wantSubstr)
		})
	}
}

func TestBuildGateStageMetadata_Recovery_Validation(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		recovery   BuildGateRecoveryMetadata
		wantSubstr string
	}{
		// Happy paths.
		{
			name: "valid with all optional fields",
			recovery: BuildGateRecoveryMetadata{
				LoopKind:     "healing",
				ErrorKind:    "infra",
				StrategyID:   "infra-default",
				Confidence:   confidencePtr(0.8),
				Reason:       "pre_gate network timeout",
				Expectations: json.RawMessage(`{"artifacts":[{"path":"/out/gate-profile-candidate.json","schema":"gate_profile_v1"}]}`),
			},
		},
		{
			name: "valid candidate promoted",
			recovery: BuildGateRecoveryMetadata{
				LoopKind:                  "healing",
				ErrorKind:                 "infra",
				CandidateSchemaID:         GateProfileCandidateSchemaID,
				CandidateArtifactPath:     GateProfileCandidateArtifactPath,
				CandidateValidationStatus: RecoveryCandidateStatusValid,
				CandidatePromoted:         boolPtr(true),
				CandidateGateProfile: json.RawMessage(`{
					"schema_version": 1,
					"repo_id": "repo_123",
					"runner_mode": "simple",
					"targets": {
						"active": "unit",
						"build": {"status":"passed","command":"go test ./...","env":{},"failure_code":null},
						"unit": {"status":"passed","command":"go test ./... -run Unit","env":{},"failure_code":null},
						"all_tests": {"status":"not_attempted","env":{}}
					},
					"orchestration": {"pre": [], "post": []}
				}`),
			},
		},
		// Enum validation.
		{
			name:       "invalid loop_kind",
			recovery:   BuildGateRecoveryMetadata{LoopKind: "prepare", ErrorKind: "infra"},
			wantSubstr: "loop_kind",
		},
		{
			name:       "invalid error_kind",
			recovery:   BuildGateRecoveryMetadata{LoopKind: "healing", ErrorKind: "routing"},
			wantSubstr: "error_kind",
		},
		{
			name:       "custom error_kind rejected",
			recovery:   BuildGateRecoveryMetadata{LoopKind: "healing", ErrorKind: "custom"},
			wantSubstr: "error_kind",
		},
		// Confidence bounds.
		{
			name:       "confidence below range",
			recovery:   BuildGateRecoveryMetadata{LoopKind: "healing", ErrorKind: "code", Confidence: confidencePtr(-0.1)},
			wantSubstr: "confidence",
		},
		{
			name:       "confidence above range",
			recovery:   BuildGateRecoveryMetadata{LoopKind: "healing", ErrorKind: "code", Confidence: confidencePtr(1.1)},
			wantSubstr: "confidence",
		},
		// String field constraints.
		{
			name:       "multiline strategy_id",
			recovery:   BuildGateRecoveryMetadata{LoopKind: "healing", ErrorKind: "code", StrategyID: "line1\nline2"},
			wantSubstr: "strategy_id",
		},
		{
			name:       "too long strategy_id",
			recovery:   BuildGateRecoveryMetadata{LoopKind: "healing", ErrorKind: "code", StrategyID: strings.Repeat("x", 201)},
			wantSubstr: "strategy_id",
		},
		{
			name:       "multiline reason",
			recovery:   BuildGateRecoveryMetadata{LoopKind: "healing", ErrorKind: "code", Reason: "line1\nline2"},
			wantSubstr: "reason",
		},
		{
			name:       "too long reason",
			recovery:   BuildGateRecoveryMetadata{LoopKind: "healing", ErrorKind: "code", Reason: strings.Repeat("x", 201)},
			wantSubstr: "reason",
		},
		// Expectations type.
		{
			name:       "invalid expectations type",
			recovery:   BuildGateRecoveryMetadata{LoopKind: "healing", ErrorKind: "infra", Expectations: json.RawMessage(`"scalar"`)},
			wantSubstr: "expectations",
		},
		// Candidate state machine.
		{
			name: "invalid candidate status",
			recovery: BuildGateRecoveryMetadata{
				LoopKind:                  "healing",
				ErrorKind:                 "infra",
				CandidateValidationStatus: "done",
			},
			wantSubstr: "candidate_validation_status invalid",
		},
		{
			name: "valid status missing payload",
			recovery: BuildGateRecoveryMetadata{
				LoopKind:                  "healing",
				ErrorKind:                 "infra",
				CandidateValidationStatus: RecoveryCandidateStatusValid,
			},
			wantSubstr: "candidate_gate_profile: required when",
		},
		{
			name: "non-valid status with payload",
			recovery: BuildGateRecoveryMetadata{
				LoopKind:                  "healing",
				ErrorKind:                 "infra",
				CandidateValidationStatus: RecoveryCandidateStatusInvalid,
				CandidateGateProfile:      json.RawMessage(`{"schema_version":1}`),
			},
			wantSubstr: "candidate_gate_profile: forbidden when",
		},
		{
			name: "promoted true with non-valid status",
			recovery: BuildGateRecoveryMetadata{
				LoopKind:                  "healing",
				ErrorKind:                 "infra",
				CandidateValidationStatus: RecoveryCandidateStatusInvalid,
				CandidatePromoted:         boolPtr(true),
			},
			wantSubstr: "candidate_promoted: true requires candidate_validation_status",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			meta := BuildGateStageMetadata{Recovery: &tt.recovery}
			requireValidationErr(t, meta.Validate(), tt.wantSubstr)
		})
	}
}
