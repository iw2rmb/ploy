package contracts

import (
	"encoding/json"
	"testing"
)

func copyStackExp(e StackExpectation) *StackExpectation { return &e }

// Shared test expectation values used across multiple test cases.
var (
	javaMaven17Exp = StackExpectation{Language: "java", Tool: "maven", Release: "17"}
	javaMaven11Exp = StackExpectation{Language: "java", Tool: "maven", Release: "11"}
)

// stackGateJSON wraps a stack JSON fragment in a minimal MigSpec envelope.
func stackGateJSON(stackFragment string) string {
	return `{"steps":[{"image":"test:latest","stack":` + stackFragment + `}]}`
}

// marshalStep0Map marshals a MigSpec to JSON and returns step[0] as a
// map[string]any, failing the test on any error.
func marshalStep0Map(t *testing.T, spec *MigSpec) map[string]any {
	t.Helper()
	data, err := json.Marshal(spec)
	if err != nil {
		t.Fatalf("json.Marshal failed: %v", err)
	}
	var m map[string]any
	if err := json.Unmarshal(data, &m); err != nil {
		t.Fatalf("json.Unmarshal failed: %v", err)
	}
	stepsRaw, ok := m["steps"].([]any)
	if !ok {
		t.Fatal("steps not in expected format")
	}
	step0, ok := stepsRaw[0].(map[string]any)
	if !ok {
		t.Fatal("step[0] not in expected format")
	}
	return step0
}

// TestStackExpectation_IsEmpty tests empty detection.
func TestStackExpectation_IsEmpty(t *testing.T) {
	tests := []struct {
		name      string
		exp       StackExpectation
		wantEmpty bool
	}{
		{
			name:      "all empty",
			exp:       StackExpectation{},
			wantEmpty: true,
		},
		{
			name:      "language set",
			exp:       StackExpectation{Language: "java"},
			wantEmpty: false,
		},
		{
			name:      "tool set",
			exp:       StackExpectation{Tool: "maven"},
			wantEmpty: false,
		},
		{
			name:      "release set",
			exp:       StackExpectation{Release: "11"},
			wantEmpty: false,
		},
		{
			name:      "all set",
			exp:       javaMaven17Exp,
			wantEmpty: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.exp.IsEmpty(); got != tt.wantEmpty {
				t.Errorf("IsEmpty() = %v, want %v", got, tt.wantEmpty)
			}
		})
	}
}

// TestStackExpectation_Equal tests equality comparison.
func TestStackExpectation_Equal(t *testing.T) {
	tests := []struct {
		name string
		a    StackExpectation
		b    StackExpectation
		want bool
	}{
		{
			name: "both empty",
			a:    StackExpectation{},
			b:    StackExpectation{},
			want: true,
		},
		{
			name: "identical full",
			a:    javaMaven17Exp,
			b:    javaMaven17Exp,
			want: true,
		},
		{
			name: "different language",
			a:    StackExpectation{Language: "java"},
			b:    StackExpectation{Language: "go"},
			want: false,
		},
		{
			name: "different tool",
			a:    StackExpectation{Tool: "maven"},
			b:    StackExpectation{Tool: "gradle"},
			want: false,
		},
		{
			name: "different release",
			a:    StackExpectation{Release: "11"},
			b:    StackExpectation{Release: "17"},
			want: false,
		},
		{
			name: "partial vs full",
			a:    StackExpectation{Language: "java"},
			b:    StackExpectation{Language: "java", Tool: "maven"},
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.a.Equal(tt.b); got != tt.want {
				t.Errorf("Equal() = %v, want %v", got, tt.want)
			}
		})
	}
}

// TestParseMigSpecJSON_StackGate tests parsing full stack spec via MigSpec.
func TestParseMigSpecJSON_StackGate(t *testing.T) {
	input := `{
		"steps": [{
			"name": "java11-to-17",
			"image": "docker.io/user/migs-orw:latest",
			"stack": {
				"inbound":  {"enabled": true, "expect": {"language": "java", "tool": "maven", "release": "11"}},
				"outbound": {"enabled": true, "expect": {"language": "java", "tool": "maven", "release": "17"}}
			}
		}]
	}`

	spec, err := ParseMigSpecJSON([]byte(input))
	if err != nil {
		t.Fatalf("ParseMigSpecJSON failed: %v", err)
	}
	if len(spec.Steps) != 1 {
		t.Fatalf("len(steps) = %d, want 1", len(spec.Steps))
	}

	step := spec.Steps[0]
	if step.Name != "java11-to-17" {
		t.Errorf("name = %q, want %q", step.Name, "java11-to-17")
	}
	if step.Stack == nil {
		t.Fatal("stack is nil")
	}

	wantStack := StackGateSpec{
		Inbound:  &StackGatePhaseSpec{Enabled: true, Expect: copyStackExp(javaMaven11Exp)},
		Outbound: &StackGatePhaseSpec{Enabled: true, Expect: copyStackExp(javaMaven17Exp)},
	}
	if !step.Stack.Equal(wantStack) {
		t.Errorf("stack mismatch:\n  got  inbound=%+v outbound=%+v\n  want inbound=%+v outbound=%+v",
			step.Stack.Inbound, step.Stack.Outbound, wantStack.Inbound, wantStack.Outbound)
	}
}

// TestParseStackExpectation_NumericRelease tests string/int/float release handling.
func TestParseStackExpectation_NumericRelease(t *testing.T) {
	tests := []struct {
		name        string
		stack       string
		wantRelease string
	}{
		{"string release", `{"inbound":{"enabled":true,"expect":{"release":"11"}}}`, "11"},
		{"integer release", `{"inbound":{"enabled":true,"expect":{"release":11}}}`, "11"},
		{"float release", `{"inbound":{"enabled":true,"expect":{"release":3.9}}}`, "3.9"},
		{"integer 17", `{"inbound":{"enabled":true,"expect":{"release":17}}}`, "17"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			spec, err := ParseMigSpecJSON([]byte(stackGateJSON(tt.stack)))
			if err != nil {
				t.Fatalf("ParseMigSpecJSON failed: %v", err)
			}
			got := spec.Steps[0].Stack.Inbound.Expect.Release
			if got != tt.wantRelease {
				t.Errorf("release = %q, want %q", got, tt.wantRelease)
			}
		})
	}
}

// TestValidateStackGatePhaseSpec tests validation of phase spec configurations.
func TestValidateStackGatePhaseSpec(t *testing.T) {
	tests := []struct {
		name    string
		stack   string
		wantErr string
	}{
		{
			name:    "inbound disabled with expect rejected",
			stack:   `{"inbound":{"enabled":false,"expect":{"language":"java"}}}`,
			wantErr: "steps[0].stack.inbound: enabled=false with expect is ambiguous",
		},
		{
			name:    "outbound disabled with expect rejected",
			stack:   `{"outbound":{"enabled":false,"expect":{"release":"17"}}}`,
			wantErr: "steps[0].stack.outbound: enabled=false with expect is ambiguous",
		},
		{
			name:    "inbound enabled without expect rejected",
			stack:   `{"inbound":{"enabled":true}}`,
			wantErr: "steps[0].stack.inbound: enabled=true requires expect",
		},
		{
			name:    "outbound enabled without expect rejected",
			stack:   `{"outbound":{"enabled":true}}`,
			wantErr: "steps[0].stack.outbound: enabled=true requires expect",
		},
		{
			name:    "enabled with empty expect rejected",
			stack:   `{"inbound":{"enabled":true,"expect":{}}}`,
			wantErr: "steps[0].stack.inbound: enabled=true requires expect",
		},
		{
			name:    "enabled with valid expect passes",
			stack:   `{"inbound":{"enabled":true,"expect":{"language":"java"}}}`,
			wantErr: "",
		},
		{
			name:    "disabled without expect passes",
			stack:   `{"inbound":{"enabled":false}}`,
			wantErr: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := ParseMigSpecJSON([]byte(stackGateJSON(tt.stack)))
			requireValidationErr(t, err, tt.wantErr)
		})
	}
}

// TestStackGateSpec_RoundTrip tests parse-wire-parse round-trip.
func TestStackGateSpec_RoundTrip(t *testing.T) {
	original := &MigSpec{
		Steps: []MigStep{{
			Image: JobImage{Universal: "docker.io/user/mig:latest"},
			Stack: &StackGateSpec{
				Inbound:  &StackGatePhaseSpec{Enabled: true, Expect: copyStackExp(javaMaven11Exp)},
				Outbound: &StackGatePhaseSpec{Enabled: true, Expect: copyStackExp(javaMaven17Exp)},
			},
		}},
	}

	data, err := json.Marshal(original)
	if err != nil {
		t.Fatalf("json.Marshal failed: %v", err)
	}

	parsed, err := ParseMigSpecJSON(data)
	if err != nil {
		t.Fatalf("ParseMigSpecJSON failed: %v", err)
	}

	if parsed.Steps[0].Stack == nil {
		t.Fatal("stack is nil after round-trip")
	}
	if !original.Steps[0].Stack.Equal(*parsed.Steps[0].Stack) {
		t.Errorf("round-trip mismatch:\n  original=%+v\n  parsed  =%+v",
			original.Steps[0].Stack, parsed.Steps[0].Stack)
	}
}

// TestStackGateSpec_IsEmpty tests empty detection for full spec.
func TestStackGateSpec_IsEmpty(t *testing.T) {
	tests := []struct {
		name      string
		spec      StackGateSpec
		wantEmpty bool
	}{
		{
			name:      "empty spec",
			spec:      StackGateSpec{},
			wantEmpty: true,
		},
		{
			name:      "nil phases",
			spec:      StackGateSpec{Inbound: nil, Outbound: nil},
			wantEmpty: true,
		},
		{
			name:      "disabled inbound only",
			spec:      StackGateSpec{Inbound: &StackGatePhaseSpec{Enabled: false}},
			wantEmpty: true,
		},
		{
			name:      "enabled inbound",
			spec:      StackGateSpec{Inbound: &StackGatePhaseSpec{Enabled: true}},
			wantEmpty: false,
		},
		{
			name:      "enabled outbound",
			spec:      StackGateSpec{Outbound: &StackGatePhaseSpec{Enabled: true}},
			wantEmpty: false,
		},
		{
			name: "full spec",
			spec: StackGateSpec{
				Inbound:  &StackGatePhaseSpec{Enabled: true, Expect: &StackExpectation{Language: "java"}},
				Outbound: &StackGatePhaseSpec{Enabled: true, Expect: &StackExpectation{Release: "17"}},
			},
			wantEmpty: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.spec.IsEmpty(); got != tt.wantEmpty {
				t.Errorf("IsEmpty() = %v, want %v", got, tt.wantEmpty)
			}
		})
	}
}

// TestStackGateSpec_Equal tests equality comparison for full specs.
func TestStackGateSpec_Equal(t *testing.T) {
	tests := []struct {
		name string
		a    StackGateSpec
		b    StackGateSpec
		want bool
	}{
		{
			name: "both empty",
			a:    StackGateSpec{},
			b:    StackGateSpec{},
			want: true,
		},
		{
			name: "identical full",
			a: StackGateSpec{
				Inbound:  &StackGatePhaseSpec{Enabled: true, Expect: &StackExpectation{Language: "java"}},
				Outbound: &StackGatePhaseSpec{Enabled: true, Expect: &StackExpectation{Release: "17"}},
			},
			b: StackGateSpec{
				Inbound:  &StackGatePhaseSpec{Enabled: true, Expect: &StackExpectation{Language: "java"}},
				Outbound: &StackGatePhaseSpec{Enabled: true, Expect: &StackExpectation{Release: "17"}},
			},
			want: true,
		},
		{
			name: "different inbound",
			a:    StackGateSpec{Inbound: &StackGatePhaseSpec{Enabled: true}},
			b:    StackGateSpec{Inbound: &StackGatePhaseSpec{Enabled: false}},
			want: false,
		},
		{
			name: "one nil inbound",
			a:    StackGateSpec{Inbound: &StackGatePhaseSpec{Enabled: true}},
			b:    StackGateSpec{Inbound: nil},
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.a.Equal(tt.b); got != tt.want {
				t.Errorf("Equal() = %v, want %v", got, tt.want)
			}
		})
	}
}

// TestStackGateWire_Omission tests that empty/disabled stack is omitted in wire format.
func TestStackGateWire_Omission(t *testing.T) {
	tests := []struct {
		name string
		spec *MigSpec
	}{
		{
			name: "nil stack omitted",
			spec: &MigSpec{Steps: []MigStep{{
				Image: JobImage{Universal: "test:latest"},
				Stack: nil,
			}}},
		},
		{
			name: "disabled-only stack omitted",
			spec: &MigSpec{Steps: []MigStep{{
				Image: JobImage{Universal: "test:latest"},
				Stack: &StackGateSpec{
					Inbound: &StackGatePhaseSpec{Enabled: false},
				},
			}}},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			step0 := marshalStep0Map(t, tt.spec)
			if v, exists := step0["stack"]; exists && v != nil {
				t.Errorf("stack should be omitted, got %v", v)
			}
		})
	}
}
