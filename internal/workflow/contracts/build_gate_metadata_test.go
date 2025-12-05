package contracts

import "testing"

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
