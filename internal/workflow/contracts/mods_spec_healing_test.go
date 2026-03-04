package contracts

import (
	"strings"
	"testing"
)

func TestParseModsSpecJSON_HealingValidation(t *testing.T) {
	// Healing with image but no router.
	input := `{
		"steps": [{"image": "test:latest"}],
		"build_gate": {"healing": {"by_error_kind":{"infra":{"retries": 1, "image": "codex:latest", "command": "fix"}}}}
	}`
	_, err := ParseModsSpecJSON([]byte(input))
	if err == nil {
		t.Fatal("expected validation error for healing without router")
	}
}

func TestParseModsSpecJSON_HealingRequiresImage(t *testing.T) {
	// Healing configured without an image.
	input := `{
		"steps": [{"image": "test:latest"}],
		"build_gate": {
			"healing": {"by_error_kind":{"infra":{"retries": 1}}},
			"router": {"image": "router:latest"}
		}
	}`
	_, err := ParseModsSpecJSON([]byte(input))
	if err == nil {
		t.Fatal("expected validation error for healing without image")
	}
	if want := "build_gate.healing.by_error_kind.infra.image: required"; err.Error() != want {
		t.Fatalf("error = %q, want %q", err.Error(), want)
	}
}

func TestParseModsSpecJSON_HealingRetriesCoercion(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		want    int
		wantErr string
	}{
		{
			name: "int value",
			input: `{
				"steps": [{"image":"test:latest"}],
				"build_gate": {"healing":{"by_error_kind":{"infra":{"retries": 3, "image":"codex:latest"}}}, "router":{"image":"router:latest"}}
			}`,
			want: 3,
		},
		{
			name: "float rejected",
			input: `{
				"steps": [{"image":"test:latest"}],
				"build_gate": {"healing":{"by_error_kind":{"infra":{"retries": 1.9, "image":"codex:latest"}}}, "router":{"image":"router:latest"}}
			}`,
			wantErr: "parse migs spec json",
		},
		{
			name: "non-number rejected",
			input: `{
				"steps": [{"image":"test:latest"}],
				"build_gate": {"healing":{"by_error_kind":{"infra":{"retries": "nope", "image":"codex:latest"}}}, "router":{"image":"router:latest"}}
			}`,
			wantErr: "parse migs spec json",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			spec, err := ParseModsSpecJSON([]byte(tt.input))
			if tt.wantErr != "" {
				if err == nil {
					t.Fatal("expected error")
				}
				if !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("error = %q, want to contain %q", err.Error(), tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if spec.BuildGate == nil || spec.BuildGate.Healing == nil {
				t.Fatal("build_gate.healing is nil")
			}
			infra, ok := spec.BuildGate.Healing.ByErrorKind["infra"]
			if !ok {
				t.Fatal("missing build_gate.healing.by_error_kind.infra")
			}
			if infra.Retries != tt.want {
				t.Fatalf("retries = %d, want %d", infra.Retries, tt.want)
			}
		})
	}
}

// TestModsSpec_RoundTrip tests round-trip conversion via json.Marshal → ParseModsSpecJSON.
