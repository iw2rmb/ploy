package contracts

import (
	"strings"
	"testing"
)

func TestParseMigSpecJSON_HealValidation(t *testing.T) {
	// Heal without image.
	input := `{
		"steps": [{"image": "test:latest"}],
		"build_gate": {"heal": {"retries": 1}}
	}`
	_, err := ParseMigSpecJSON([]byte(input))
	if err == nil {
		t.Fatal("expected validation error for heal without image")
	}
	if want := "build_gate.heal.image: required"; err.Error() != want {
		t.Fatalf("error = %q, want %q", err.Error(), want)
	}
}

func TestParseMigSpecJSON_HealRetriesCoercion(t *testing.T) {
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
				"build_gate": {"heal":{"retries": 3, "image":"amata:latest"}}
			}`,
			want: 3,
		},
		{
			name: "float rejected",
			input: `{
				"steps": [{"image":"test:latest"}],
				"build_gate": {"heal":{"retries": 1.9, "image":"amata:latest"}}
			}`,
			wantErr: "parse migs spec json",
		},
		{
			name: "non-number rejected",
			input: `{
				"steps": [{"image":"test:latest"}],
				"build_gate": {"heal":{"retries": "nope", "image":"amata:latest"}}
			}`,
			wantErr: "parse migs spec json",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			spec, err := ParseMigSpecJSON([]byte(tt.input))
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
			if spec.BuildGate == nil || spec.BuildGate.Heal == nil {
				t.Fatal("build_gate.heal is nil")
			}
			if spec.BuildGate.Heal.Retries != tt.want {
				t.Fatalf("retries = %d, want %d", spec.BuildGate.Heal.Retries, tt.want)
			}
		})
	}
}

func TestParseMigSpecJSON_HealRetriesDefault(t *testing.T) {
	input := `{
		"steps": [{"image":"test:latest"}],
		"build_gate": {"heal":{"image":"amata:latest"}}
	}`
	spec, err := ParseMigSpecJSON([]byte(input))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if spec.BuildGate.Heal.Retries != 1 {
		t.Fatalf("retries = %d, want 1 (default)", spec.BuildGate.Heal.Retries)
	}
}

func TestParseMigSpecJSON_RouterForbidden(t *testing.T) {
	input := `{
		"steps": [{"image": "test:latest"}],
		"build_gate": {"router": {"image": "router:latest"}}
	}`
	_, err := ParseMigSpecJSON([]byte(input))
	if err == nil {
		t.Fatal("expected validation error for router field")
	}
	if !strings.Contains(err.Error(), "build_gate.router: forbidden") {
		t.Fatalf("error = %q, want to contain build_gate.router: forbidden", err.Error())
	}
}

func TestParseMigSpecJSON_HealingForbidden(t *testing.T) {
	input := `{
		"steps": [{"image": "test:latest"}],
		"build_gate": {"healing": {"by_error_kind": {"infra": {"image": "heal:latest"}}}}
	}`
	_, err := ParseMigSpecJSON([]byte(input))
	if err == nil {
		t.Fatal("expected validation error for healing field")
	}
	if !strings.Contains(err.Error(), "build_gate.healing: forbidden") {
		t.Fatalf("error = %q, want to contain build_gate.healing: forbidden", err.Error())
	}
}

// TestMigSpec_RoundTrip tests round-trip conversion via json.Marshal → ParseMigSpecJSON.
