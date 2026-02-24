package contracts

import (
	"encoding/json"
	"testing"
)

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
			exp:       StackExpectation{Language: "java", Tool: "maven", Release: "17"},
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
			a:    StackExpectation{Language: "java", Tool: "maven", Release: "17"},
			b:    StackExpectation{Language: "java", Tool: "maven", Release: "17"},
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

// TestParseStackGateSpec_FullSpec tests parsing full inbound/outbound configuration.
func TestParseStackGateSpec_FullSpec(t *testing.T) {
	input := `{
		"steps": [{
			"image": "docker.io/user/mod:latest",
			"stack": {
				"inbound": {
					"enabled": true,
					"expect": {"language": "java", "tool": "maven", "release": "11"}
				},
				"outbound": {
					"enabled": true,
					"expect": {"language": "java", "tool": "maven", "release": "17"}
				}
			}
		}]
	}`

	spec, err := ParseModsSpecJSON([]byte(input))
	if err != nil {
		t.Fatalf("ParseModsSpecJSON failed: %v", err)
	}

	if spec.Steps[0].Stack == nil {
		t.Fatal("stack is nil")
	}

	stack := spec.Steps[0].Stack

	// Verify inbound.
	if stack.Inbound == nil {
		t.Fatal("inbound is nil")
	}
	if !stack.Inbound.Enabled {
		t.Error("inbound.enabled = false, want true")
	}
	if stack.Inbound.Expect == nil {
		t.Fatal("inbound.expect is nil")
	}
	if stack.Inbound.Expect.Language != "java" {
		t.Errorf("inbound.expect.language = %q, want %q", stack.Inbound.Expect.Language, "java")
	}
	if stack.Inbound.Expect.Tool != "maven" {
		t.Errorf("inbound.expect.tool = %q, want %q", stack.Inbound.Expect.Tool, "maven")
	}
	if stack.Inbound.Expect.Release != "11" {
		t.Errorf("inbound.expect.release = %q, want %q", stack.Inbound.Expect.Release, "11")
	}

	// Verify outbound.
	if stack.Outbound == nil {
		t.Fatal("outbound is nil")
	}
	if !stack.Outbound.Enabled {
		t.Error("outbound.enabled = false, want true")
	}
	if stack.Outbound.Expect == nil {
		t.Fatal("outbound.expect is nil")
	}
	if stack.Outbound.Expect.Release != "17" {
		t.Errorf("outbound.expect.release = %q, want %q", stack.Outbound.Expect.Release, "17")
	}
}

// TestParseStackExpectation_NumericRelease tests string/int/float release handling.
func TestParseStackExpectation_NumericRelease(t *testing.T) {
	tests := []struct {
		name        string
		input       string
		wantRelease string
	}{
		{
			name:        "string release",
			input:       `{"steps": [{"image": "test:latest", "stack": {"inbound": {"enabled": true, "expect": {"release": "11"}}}}]}`,
			wantRelease: "11",
		},
		{
			name:        "integer release (JSON float64)",
			input:       `{"steps": [{"image": "test:latest", "stack": {"inbound": {"enabled": true, "expect": {"release": 11}}}}]}`,
			wantRelease: "11",
		},
		{
			name:        "float release",
			input:       `{"steps": [{"image": "test:latest", "stack": {"inbound": {"enabled": true, "expect": {"release": 3.9}}}}]}`,
			wantRelease: "3.9",
		},
		{
			name:        "integer 17",
			input:       `{"steps": [{"image": "test:latest", "stack": {"inbound": {"enabled": true, "expect": {"release": 17}}}}]}`,
			wantRelease: "17",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			spec, err := ParseModsSpecJSON([]byte(tt.input))
			if err != nil {
				t.Fatalf("ParseModsSpecJSON failed: %v", err)
			}

			if spec.Steps[0].Stack == nil || spec.Steps[0].Stack.Inbound == nil ||
				spec.Steps[0].Stack.Inbound.Expect == nil {
				t.Fatal("stack/inbound/expect is nil")
			}

			got := spec.Steps[0].Stack.Inbound.Expect.Release
			if got != tt.wantRelease {
				t.Errorf("release = %q, want %q", got, tt.wantRelease)
			}
		})
	}
}

// TestValidateStackGatePhaseSpec_RejectDisabledWithExpect tests ambiguous config rejection.
func TestValidateStackGatePhaseSpec_RejectDisabledWithExpect(t *testing.T) {
	input := `{
		"steps": [{
			"image": "test:latest",
			"stack": {
				"inbound": {
					"enabled": false,
					"expect": {"language": "java"}
				}
			}
		}]
	}`

	_, err := ParseModsSpecJSON([]byte(input))
	if err == nil {
		t.Fatal("expected validation error for enabled=false with expect")
	}

	want := "steps[0].stack.inbound: enabled=false with expect is ambiguous; remove expect or set enabled=true"
	if err.Error() != want {
		t.Errorf("error = %q, want %q", err.Error(), want)
	}
}

// TestStackGateSpec_RoundTrip tests parse→wire→parse round-trip.
func TestStackGateSpec_RoundTrip(t *testing.T) {
	original := &ModsSpec{
		Steps: []ModStep{{
			Image: JobImage{Universal: "docker.io/user/mod:latest"},
			Stack: &StackGateSpec{
				Inbound: &StackGatePhaseSpec{
					Enabled: true,
					Expect:  &StackExpectation{Language: "java", Tool: "maven", Release: "11"},
				},
				Outbound: &StackGatePhaseSpec{
					Enabled: true,
					Expect:  &StackExpectation{Language: "java", Tool: "maven", Release: "17"},
				},
			},
		}},
	}

	// Convert to map.
	m := original.ToMap()

	// Marshal to JSON and parse back.
	data, err := json.Marshal(m)
	if err != nil {
		t.Fatalf("json.Marshal failed: %v", err)
	}

	parsed, err := ParseModsSpecJSON(data)
	if err != nil {
		t.Fatalf("ParseModsSpecJSON failed: %v", err)
	}

	// Verify round-trip.
	if parsed.Steps[0].Stack == nil {
		t.Fatal("stack is nil after round-trip")
	}

	// Compare original and parsed.
	if !original.Steps[0].Stack.Equal(*parsed.Steps[0].Stack) {
		t.Errorf("round-trip mismatch: original != parsed")
	}

	// Verify specific values.
	if parsed.Steps[0].Stack.Inbound.Expect.Release != "11" {
		t.Errorf("inbound.expect.release = %q, want %q",
			parsed.Steps[0].Stack.Inbound.Expect.Release, "11")
	}
	if parsed.Steps[0].Stack.Outbound.Expect.Release != "17" {
		t.Errorf("outbound.expect.release = %q, want %q",
			parsed.Steps[0].Stack.Outbound.Expect.Release, "17")
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

// TestParseModsSpecJSON_StackGate tests parsing full stack spec via ModsSpec.
func TestParseModsSpecJSON_StackGate(t *testing.T) {
	input := `{
		"steps": [
			{
				"name": "java11-to-17",
				"image": "docker.io/user/mods-orw:latest",
				"stack": {
					"inbound": {
						"enabled": true,
						"expect": {"language": "java", "tool": "maven", "release": "11"}
					},
					"outbound": {
						"enabled": true,
						"expect": {"language": "java", "tool": "maven", "release": "17"}
					}
				}
			}
		]
	}`

	spec, err := ParseModsSpecJSON([]byte(input))
	if err != nil {
		t.Fatalf("ParseModsSpecJSON failed: %v", err)
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
	if step.Stack.Inbound == nil || step.Stack.Outbound == nil {
		t.Fatal("inbound or outbound is nil")
	}

	// Verify inbound.
	if step.Stack.Inbound.Expect.Language != "java" {
		t.Errorf("inbound.language = %q, want java", step.Stack.Inbound.Expect.Language)
	}
	if step.Stack.Inbound.Expect.Tool != "maven" {
		t.Errorf("inbound.tool = %q, want maven", step.Stack.Inbound.Expect.Tool)
	}
	if step.Stack.Inbound.Expect.Release != "11" {
		t.Errorf("inbound.release = %q, want 11", step.Stack.Inbound.Expect.Release)
	}

	// Verify outbound.
	if step.Stack.Outbound.Expect.Release != "17" {
		t.Errorf("outbound.release = %q, want 17", step.Stack.Outbound.Expect.Release)
	}
}

// TestParseModsSpecJSON_StackGate_RejectDisabledWithExpect tests validation error.
func TestParseModsSpecJSON_StackGate_RejectDisabledWithExpect(t *testing.T) {
	input := `{
		"steps": [{
			"image": "test:latest",
			"stack": {
				"outbound": {
					"enabled": false,
					"expect": {"release": "17"}
				}
			}
		}]
	}`

	_, err := ParseModsSpecJSON([]byte(input))
	if err == nil {
		t.Fatal("expected validation error")
	}

	want := "steps[0].stack.outbound: enabled=false with expect is ambiguous; remove expect or set enabled=true"
	if err.Error() != want {
		t.Errorf("error = %q, want %q", err.Error(), want)
	}
}

// TestValidateStackGatePhaseSpec_RejectEnabledWithoutExpect tests that enabled:true
// without expect is rejected as incomplete configuration.
func TestValidateStackGatePhaseSpec_RejectEnabledWithoutExpect(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		wantErr string
	}{
		{
			name: "inbound enabled without expect",
			input: `{
				"steps": [{
					"image": "test:latest",
					"stack": {
						"inbound": {
							"enabled": true
						}
					}
				}]
			}`,
			wantErr: "steps[0].stack.inbound: enabled=true requires expect; add expect or set enabled=false",
		},
		{
			name: "outbound enabled without expect",
			input: `{
				"steps": [{
					"image": "test:latest",
					"stack": {
						"outbound": {
							"enabled": true
						}
					}
				}]
			}`,
			wantErr: "steps[0].stack.outbound: enabled=true requires expect; add expect or set enabled=false",
		},
		{
			name: "enabled with empty expect",
			input: `{
				"steps": [{
					"image": "test:latest",
					"stack": {
						"inbound": {
							"enabled": true,
							"expect": {}
						}
					}
				}]
			}`,
			wantErr: "steps[0].stack.inbound: enabled=true requires expect; add expect or set enabled=false",
		},
		{
			name: "enabled with valid expect passes",
			input: `{
				"steps": [{
					"image": "test:latest",
					"stack": {
						"inbound": {
							"enabled": true,
							"expect": {"language": "java"}
						}
					}
				}]
			}`,
			wantErr: "",
		},
		{
			name: "disabled without expect passes",
			input: `{
				"steps": [{
					"image": "test:latest",
					"stack": {
						"inbound": {
							"enabled": false
						}
					}
				}]
			}`,
			wantErr: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := ParseModsSpecJSON([]byte(tt.input))
			if tt.wantErr == "" {
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
				return
			}
			if err == nil {
				t.Fatal("expected validation error")
			}
			if err.Error() != tt.wantErr {
				t.Errorf("error = %q, want %q", err.Error(), tt.wantErr)
			}
		})
	}
}

// TestStackGateWire_EmptyOmitted tests that empty stack is omitted in wire format.
func TestStackGateWire_EmptyOmitted(t *testing.T) {
	spec := &ModsSpec{
		Steps: []ModStep{{
			Image: JobImage{Universal: "test:latest"},
			Stack: nil, // No stack config.
		}},
	}

	m := spec.ToMap()
	stepsRaw, ok := m["steps"].([]any)
	if !ok {
		t.Fatal("steps not in expected format")
	}

	step0, ok := stepsRaw[0].(map[string]any)
	if !ok {
		t.Fatal("step[0] not in expected format")
	}
	if _, exists := step0["stack"]; exists {
		t.Error("stack should be omitted when nil")
	}
}

// TestStackGateWire_DisabledOmitted tests that disabled phases are omitted.
func TestStackGateWire_DisabledOmitted(t *testing.T) {
	spec := &ModsSpec{
		Steps: []ModStep{{
			Image: JobImage{Universal: "test:latest"},
			Stack: &StackGateSpec{
				Inbound: &StackGatePhaseSpec{Enabled: false}, // Disabled, should be omitted.
			},
		}},
	}

	m := spec.ToMap()
	stepsRaw, ok := m["steps"].([]any)
	if !ok {
		t.Fatal("steps not in expected format")
	}

	step0, ok := stepsRaw[0].(map[string]any)
	if !ok {
		t.Fatal("step[0] not in expected format")
	}
	// Stack should be omitted or null when all phases are disabled.
	if v, exists := step0["stack"]; exists && v != nil {
		t.Errorf("stack should be omitted or null when all phases are disabled, got %v", v)
	}
}
