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
