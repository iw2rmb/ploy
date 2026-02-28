package contracts

import (
	"encoding/json"
	"strings"
	"testing"
)

func confidencePtr(v float64) *float64 {
	return &v
}

// TestBuildGateStageMetadata_DetectedStack verifies that DetectedStack correctly
// derives the ModStack from the first static check's tool name.
func TestBuildGateStageMetadata_DetectedStack(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		meta BuildGateStageMetadata
		want ModStack
	}{
		{
			name: "maven tool detected",
			meta: BuildGateStageMetadata{
				StaticChecks: []BuildGateStaticCheckReport{
					{Language: "java", Tool: "maven", Passed: true},
				},
			},
			want: ModStackJavaMaven,
		},
		{
			name: "gradle tool detected",
			meta: BuildGateStageMetadata{
				StaticChecks: []BuildGateStaticCheckReport{
					{Language: "java", Tool: "gradle", Passed: false},
				},
			},
			want: ModStackJavaGradle,
		},
		{
			name: "java tool detected",
			meta: BuildGateStageMetadata{
				StaticChecks: []BuildGateStaticCheckReport{
					{Language: "java", Tool: "java", Passed: true},
				},
			},
			want: ModStackJava,
		},
		{
			name: "none tool (gate skipped)",
			meta: BuildGateStageMetadata{
				StaticChecks: []BuildGateStaticCheckReport{
					{Tool: "none", Passed: true},
				},
			},
			want: ModStackUnknown,
		},
		{
			name: "empty static checks",
			meta: BuildGateStageMetadata{
				StaticChecks: []BuildGateStaticCheckReport{},
			},
			want: ModStackUnknown,
		},
		{
			name: "nil static checks",
			meta: BuildGateStageMetadata{},
			want: ModStackUnknown,
		},
		{
			name: "multiple checks uses first",
			meta: BuildGateStageMetadata{
				StaticChecks: []BuildGateStaticCheckReport{
					{Language: "java", Tool: "maven", Passed: true},
					{Language: "java", Tool: "gradle", Passed: true},
				},
			},
			want: ModStackJavaMaven, // First check's tool is used.
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

// TestBuildGateStageMetadata_DetectedStack_Stability verifies that the same
// metadata always produces the same stack, ensuring determinism for re-gates.
func TestBuildGateStageMetadata_DetectedStack_Stability(t *testing.T) {
	t.Parallel()

	meta := BuildGateStageMetadata{
		StaticChecks: []BuildGateStaticCheckReport{
			{Language: "java", Tool: "maven", Passed: true},
		},
	}

	// Call DetectedStack multiple times to verify stability.
	for i := 0; i < 10; i++ {
		got := meta.DetectedStack()
		if got != ModStackJavaMaven {
			t.Errorf("DetectedStack() call %d = %q, want %q", i, got, ModStackJavaMaven)
		}
	}
}

// TestStackGateResult_Validate verifies StackGateResult validation logic.
func TestStackGateResult_Validate(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		result    StackGateResult
		wantErr   bool
		errSubstr string
	}{
		{
			name:    "nil stack gate is valid",
			result:  StackGateResult{},
			wantErr: false,
		},
		{
			name:    "disabled with no result is valid",
			result:  StackGateResult{Enabled: false},
			wantErr: false,
		},
		{
			name:    "disabled with any result is valid",
			result:  StackGateResult{Enabled: false, Result: "anything"},
			wantErr: false,
		},
		{
			name:    "enabled with pass result is valid",
			result:  StackGateResult{Enabled: true, Result: "pass"},
			wantErr: false,
		},
		{
			name:    "enabled with mismatch result is valid",
			result:  StackGateResult{Enabled: true, Result: "mismatch"},
			wantErr: false,
		},
		{
			name:    "enabled with unknown result is valid",
			result:  StackGateResult{Enabled: true, Result: "unknown"},
			wantErr: false,
		},
		{
			name:      "enabled with empty result is invalid",
			result:    StackGateResult{Enabled: true, Result: ""},
			wantErr:   true,
			errSubstr: "result required",
		},
		{
			name:      "enabled with invalid result is invalid",
			result:    StackGateResult{Enabled: true, Result: "invalid"},
			wantErr:   true,
			errSubstr: "result invalid",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			err := tt.result.Validate()
			if tt.wantErr {
				if err == nil {
					t.Errorf("Validate() expected error containing %q, got nil", tt.errSubstr)
				} else if !strings.Contains(err.Error(), tt.errSubstr) {
					t.Errorf("Validate() error = %q, want substring %q", err.Error(), tt.errSubstr)
				}
			} else if err != nil {
				t.Errorf("Validate() unexpected error: %v", err)
			}
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
		wantErr   bool
		errSubstr string
	}{
		{
			name: "nil stack gate is valid",
			meta: BuildGateStageMetadata{
				StaticChecks: []BuildGateStaticCheckReport{{Tool: "maven", Passed: true}},
			},
			wantErr: false,
		},
		{
			name: "valid stack gate result is valid",
			meta: BuildGateStageMetadata{
				StaticChecks: []BuildGateStaticCheckReport{{Tool: "stack-gate", Passed: true}},
				StackGate:    &StackGateResult{Enabled: true, Result: "pass"},
			},
			wantErr: false,
		},
		{
			name: "invalid stack gate result causes validation failure",
			meta: BuildGateStageMetadata{
				StaticChecks: []BuildGateStaticCheckReport{{Tool: "stack-gate", Passed: false}},
				StackGate:    &StackGateResult{Enabled: true, Result: ""},
			},
			wantErr:   true,
			errSubstr: "stack_gate invalid",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			err := tt.meta.Validate()
			if tt.wantErr {
				if err == nil {
					t.Errorf("Validate() expected error containing %q, got nil", tt.errSubstr)
				} else if !strings.Contains(err.Error(), tt.errSubstr) {
					t.Errorf("Validate() error = %q, want substring %q", err.Error(), tt.errSubstr)
				}
			} else if err != nil {
				t.Errorf("Validate() unexpected error: %v", err)
			}
		})
	}
}

// TestBuildGateStageMetadata_StackGate_JSONRoundtrip verifies JSON serialization.
func TestBuildGateStageMetadata_StackGate_JSONRoundtrip(t *testing.T) {
	t.Parallel()

	original := BuildGateStageMetadata{
		StaticChecks: []BuildGateStaticCheckReport{{
			Language: "java",
			Tool:     "stack-gate",
			Passed:   true,
		}},
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

	// Serialize to JSON.
	data, err := json.Marshal(original)
	if err != nil {
		t.Fatalf("Marshal() error: %v", err)
	}

	// Verify expected fields are present.
	jsonStr := string(data)
	expectedFields := []string{
		`"enabled":true`,
		`"expected":{`,
		`"detected":{`,
		`"runtime_image":"maven:3-eclipse-temurin-17"`,
		`"result":"pass"`,
	}
	for _, field := range expectedFields {
		if !strings.Contains(jsonStr, field) {
			t.Errorf("JSON missing expected field: %s\nGot: %s", field, jsonStr)
		}
	}

	// Deserialize back.
	var decoded BuildGateStageMetadata
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("Unmarshal() error: %v", err)
	}

	// Verify roundtrip.
	if decoded.StackGate == nil {
		t.Fatal("Roundtrip: StackGate is nil")
	}
	if decoded.StackGate.Enabled != original.StackGate.Enabled {
		t.Errorf("Enabled: got %v, want %v", decoded.StackGate.Enabled, original.StackGate.Enabled)
	}
	if decoded.StackGate.Result != original.StackGate.Result {
		t.Errorf("Result: got %q, want %q", decoded.StackGate.Result, original.StackGate.Result)
	}
	if decoded.StackGate.RuntimeImage != original.StackGate.RuntimeImage {
		t.Errorf("RuntimeImage: got %q, want %q", decoded.StackGate.RuntimeImage, original.StackGate.RuntimeImage)
	}
	if decoded.StackGate.Expected == nil || decoded.StackGate.Expected.Release != "17" {
		t.Errorf("Expected.Release: got %v, want 17", decoded.StackGate.Expected)
	}
}

func TestBuildGateStageMetadata_BugSummary_Valid(t *testing.T) {
	t.Parallel()
	meta := BuildGateStageMetadata{
		BugSummary: "Missing semicolon on line 42 of Main.java",
	}
	if err := meta.Validate(); err != nil {
		t.Errorf("Validate() unexpected error: %v", err)
	}
}

func TestBuildGateStageMetadata_BugSummary_TooLong(t *testing.T) {
	t.Parallel()
	meta := BuildGateStageMetadata{
		BugSummary: strings.Repeat("x", 201),
	}
	err := meta.Validate()
	if err == nil {
		t.Fatal("expected validation error for >200 char bug_summary")
	}
	if !strings.Contains(err.Error(), "bug_summary") {
		t.Errorf("error = %q, want substring 'bug_summary'", err.Error())
	}
}

func TestBuildGateStageMetadata_BugSummary_Multiline(t *testing.T) {
	t.Parallel()
	meta := BuildGateStageMetadata{
		BugSummary: "line one\nline two",
	}
	err := meta.Validate()
	if err == nil {
		t.Fatal("expected validation error for multiline bug_summary")
	}
	if !strings.Contains(err.Error(), "single-line") {
		t.Errorf("error = %q, want substring 'single-line'", err.Error())
	}
}

func TestBuildGateStageMetadata_Recovery_Valid(t *testing.T) {
	t.Parallel()
	meta := BuildGateStageMetadata{
		Recovery: &BuildGateRecoveryMetadata{
			LoopKind:   "healing",
			ErrorKind:  "infra",
			StrategyID: "infra-default",
			Confidence: confidencePtr(0.8),
			Reason:     "pre_gate network timeout",
		},
	}
	if err := meta.Validate(); err != nil {
		t.Fatalf("Validate() unexpected error: %v", err)
	}
}

func TestBuildGateStageMetadata_Recovery_InvalidLoopKind(t *testing.T) {
	t.Parallel()
	meta := BuildGateStageMetadata{
		Recovery: &BuildGateRecoveryMetadata{
			LoopKind:  "prepare",
			ErrorKind: "infra",
		},
	}
	err := meta.Validate()
	if err == nil {
		t.Fatal("expected validation error for invalid loop_kind")
	}
	if !strings.Contains(err.Error(), "loop_kind") {
		t.Fatalf("error = %q, want substring %q", err.Error(), "loop_kind")
	}
}

func TestBuildGateStageMetadata_Recovery_InvalidErrorKind(t *testing.T) {
	t.Parallel()
	meta := BuildGateStageMetadata{
		Recovery: &BuildGateRecoveryMetadata{
			LoopKind:  "healing",
			ErrorKind: "routing",
		},
	}
	err := meta.Validate()
	if err == nil {
		t.Fatal("expected validation error for invalid error_kind")
	}
	if !strings.Contains(err.Error(), "error_kind") {
		t.Fatalf("error = %q, want substring %q", err.Error(), "error_kind")
	}
}

func TestBuildGateStageMetadata_Recovery_InvalidConfidence(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name       string
		confidence float64
	}{
		{name: "below range", confidence: -0.1},
		{name: "above range", confidence: 1.1},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			meta := BuildGateStageMetadata{
				Recovery: &BuildGateRecoveryMetadata{
					LoopKind:   "healing",
					ErrorKind:  "code",
					Confidence: confidencePtr(tt.confidence),
				},
			}
			err := meta.Validate()
			if err == nil {
				t.Fatal("expected validation error for invalid confidence")
			}
			if !strings.Contains(err.Error(), "confidence") {
				t.Fatalf("error = %q, want substring %q", err.Error(), "confidence")
			}
		})
	}
}

func TestBuildGateStageMetadata_Recovery_MultilineOrTooLongRejected(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name     string
		recovery BuildGateRecoveryMetadata
	}{
		{
			name: "multiline strategy_id",
			recovery: BuildGateRecoveryMetadata{
				LoopKind:   "healing",
				ErrorKind:  "code",
				StrategyID: "line1\nline2",
			},
		},
		{
			name: "too long strategy_id",
			recovery: BuildGateRecoveryMetadata{
				LoopKind:   "healing",
				ErrorKind:  "code",
				StrategyID: strings.Repeat("x", 201),
			},
		},
		{
			name: "multiline reason",
			recovery: BuildGateRecoveryMetadata{
				LoopKind:  "healing",
				ErrorKind: "code",
				Reason:    "line1\nline2",
			},
		},
		{
			name: "too long reason",
			recovery: BuildGateRecoveryMetadata{
				LoopKind:  "healing",
				ErrorKind: "code",
				Reason:    strings.Repeat("x", 201),
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			meta := BuildGateStageMetadata{Recovery: &tt.recovery}
			if err := meta.Validate(); err == nil {
				t.Fatal("expected validation error")
			}
		})
	}
}

func TestBuildGateStageMetadata_Recovery_JSONRoundTrip(t *testing.T) {
	t.Parallel()
	original := BuildGateStageMetadata{
		BugSummary: "Missing semicolon on line 42",
		Recovery: &BuildGateRecoveryMetadata{
			LoopKind:   "healing",
			ErrorKind:  "infra",
			StrategyID: "infra-default",
			Confidence: confidencePtr(0.75),
			Reason:     "docker daemon unavailable",
		},
	}
	data, err := json.Marshal(original)
	if err != nil {
		t.Fatalf("Marshal() error: %v", err)
	}
	var decoded BuildGateStageMetadata
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("Unmarshal() error: %v", err)
	}
	if decoded.Recovery == nil {
		t.Fatal("Recovery is nil after round-trip")
	}
	if decoded.Recovery.LoopKind != "healing" {
		t.Fatalf("LoopKind = %q, want %q", decoded.Recovery.LoopKind, "healing")
	}
	if decoded.Recovery.ErrorKind != "infra" {
		t.Fatalf("ErrorKind = %q, want %q", decoded.Recovery.ErrorKind, "infra")
	}
	if decoded.Recovery.StrategyID != "infra-default" {
		t.Fatalf("StrategyID = %q, want %q", decoded.Recovery.StrategyID, "infra-default")
	}
	if decoded.Recovery.Confidence == nil || *decoded.Recovery.Confidence != 0.75 {
		t.Fatalf("Confidence = %#v, want %v", decoded.Recovery.Confidence, 0.75)
	}
	if decoded.Recovery.Reason != "docker daemon unavailable" {
		t.Fatalf("Reason = %q, want %q", decoded.Recovery.Reason, "docker daemon unavailable")
	}
}
