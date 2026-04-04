package contracts

import (
	"strings"
	"testing"
)

func TestMigSpecValidate_HydraFieldsStep(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		wantErr string
	}{
		{
			name: "valid in entry",
			input: `{
				"steps": [{"image": "img:latest", "in": ["abcdef0:/in/config.json"]}]
			}`,
		},
		{
			name: "valid out entry",
			input: `{
				"steps": [{"image": "img:latest", "out": ["abcdef0:/out/results"]}]
			}`,
		},
		{
			name: "valid home entry rw",
			input: `{
				"steps": [{"image": "img:latest", "home": ["abcdef0:.codex/auth.json"]}]
			}`,
		},
		{
			name: "valid home entry ro",
			input: `{
				"steps": [{"image": "img:latest", "home": ["abcdef0:.codex/auth.json:ro"]}]
			}`,
		},
		{
			name: "valid ca entry",
			input: `{
				"steps": [{"image": "img:latest", "ca": ["abcdef0123456"]}]
			}`,
		},
		{
			name: "in entry wrong domain",
			input: `{
				"steps": [{"image": "img:latest", "in": ["abcdef0:/tmp/config.json"]}]
			}`,
			wantErr: "steps[0].in[0]",
		},
		{
			name: "out entry wrong domain",
			input: `{
				"steps": [{"image": "img:latest", "out": ["abcdef0:/in/results"]}]
			}`,
			wantErr: "steps[0].out[0]",
		},
		{
			name: "home entry absolute path rejected",
			input: `{
				"steps": [{"image": "img:latest", "home": ["abcdef0:/etc/config:ro"]}]
			}`,
			wantErr: "steps[0].home[0]",
		},
		{
			name: "home entry traversal rejected",
			input: `{
				"steps": [{"image": "img:latest", "home": ["abcdef0:../../etc/passwd"]}]
			}`,
			wantErr: "steps[0].home[0]",
		},
		{
			name: "in entry path traversal rejected",
			input: `{
				"steps": [{"image": "img:latest", "in": ["abcdef0:/in/../etc/passwd"]}]
			}`,
			wantErr: "steps[0].in[0]",
		},
		{
			name: "ca entry invalid hash",
			input: `{
				"steps": [{"image": "img:latest", "ca": ["not-hex!"]}]
			}`,
			wantErr: "steps[0].ca[0]",
		},
		{
			name: "duplicate in destinations rejected",
			input: `{
				"steps": [{"image": "img:latest", "in": ["abcdef0:/in/a", "bbbbbbb:/in/a"]}]
			}`,
			wantErr: "steps[0].in[1]",
		},
		{
			name: "no hydra fields is valid",
			input: `{
				"steps": [{"image": "img:latest"}]
			}`,
		},
		// Legacy fields superseded by Hydra are forbidden at step level.
		{
			name:    "env forbidden in step",
			input:   `{"steps": [{"image": "img:latest", "env": {"FOO": "bar"}}]}`,
			wantErr: "forbidden",
		},
		{
			name:    "env_from_file forbidden in step",
			input:   `{"steps": [{"image": "img:latest", "env_from_file": {}}]}`,
			wantErr: "forbidden",
		},
		{
			name:    "tmp_dir forbidden in step",
			input:   `{"steps": [{"image": "img:latest", "tmp_dir": []}]}`,
			wantErr: "forbidden",
		},
		{
			name:    "tmp_bundle forbidden in step",
			input:   `{"steps": [{"image": "img:latest", "tmp_bundle": {}}]}`,
			wantErr: "forbidden",
		},
		// Legacy fields superseded by Hydra are forbidden at root level.
		{
			name:    "env forbidden at root",
			input:   `{"steps": [{"image": "img:latest"}], "env": {"FOO": "bar"}}`,
			wantErr: "env: forbidden",
		},
		{
			name:    "env_from_file forbidden at root",
			input:   `{"steps": [{"image": "img:latest"}], "env_from_file": {}}`,
			wantErr: "env_from_file: forbidden",
		},
		{
			name:    "tmp_dir forbidden at root",
			input:   `{"steps": [{"image": "img:latest"}], "tmp_dir": []}`,
			wantErr: "tmp_dir: forbidden",
		},
		{
			name:    "tmp_bundle forbidden at root",
			input:   `{"steps": [{"image": "img:latest"}], "tmp_bundle": {}}`,
			wantErr: "tmp_bundle: forbidden",
		},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			_, err := ParseMigSpecJSON([]byte(tc.input))
			if tc.wantErr == "" {
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
				return
			}
			if err == nil {
				t.Fatalf("expected error containing %q, got nil", tc.wantErr)
			}
			if !strings.Contains(err.Error(), tc.wantErr) {
				t.Fatalf("error %q does not contain %q", err.Error(), tc.wantErr)
			}
		})
	}
}

func TestMigSpecValidate_HydraFieldsHealing(t *testing.T) {
	base := `{
		"steps": [{"image": "img:latest"}],
		"build_gate": {
			"router": {"image": "router:latest"},
			"healing": {"by_error_kind": {"infra": {"retries": 1, "image": "healer:latest", %s}}}
		}
	}`

	tests := []struct {
		name    string
		field   string
		wantErr string
	}{
		{
			name:  "valid in entry in healing action",
			field: `"in": ["abcdef0:/in/patch.diff"]`,
		},
		{
			name:  "valid home entry in healing action",
			field: `"home": ["abcdef0:.config/app.json:ro"]`,
		},
		{
			name:    "invalid in entry in healing action",
			field:   `"in": ["abcdef0:/tmp/bad"]`,
			wantErr: "build_gate.healing.by_error_kind.infra.in[0]",
		},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			input := strings.ReplaceAll(base, "%s", tc.field)
			_, err := ParseMigSpecJSON([]byte(input))
			if tc.wantErr == "" {
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
				return
			}
			if err == nil {
				t.Fatalf("expected error containing %q, got nil", tc.wantErr)
			}
			if !strings.Contains(err.Error(), tc.wantErr) {
				t.Fatalf("error %q does not contain %q", err.Error(), tc.wantErr)
			}
		})
	}
}

func TestMigSpecValidate_HydraFieldsRouter(t *testing.T) {
	base := `{
		"steps": [{"image": "img:latest"}],
		"build_gate": {"router": {"image": "router:latest", %s}}
	}`

	tests := []struct {
		name    string
		field   string
		wantErr string
	}{
		{
			name:    "env forbidden in router",
			field:   `"env": {"FOO": "bar"}`,
			wantErr: "build_gate.router.env: forbidden",
		},
		{
			name:  "valid ca entry in router",
			field: `"ca": ["abcdef0123456"]`,
		},
		{
			name:    "invalid ca in router",
			field:   `"ca": ["NOT_HEX"]`,
			wantErr: "build_gate.router.ca[0]",
		},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			input := strings.ReplaceAll(base, "%s", tc.field)
			_, err := ParseMigSpecJSON([]byte(input))
			if tc.wantErr == "" {
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
				return
			}
			if err == nil {
				t.Fatalf("expected error containing %q, got nil", tc.wantErr)
			}
			if !strings.Contains(err.Error(), tc.wantErr) {
				t.Fatalf("error %q does not contain %q", err.Error(), tc.wantErr)
			}
		})
	}
}

