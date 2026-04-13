package hook

import (
	"strings"
	"testing"
)

func TestLoadSpecYAML_HydraEntriesValidation(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		doc     string
		wantErr string
	}{
		{
			name: "accepts canonical hydra entries",
			doc: `
id: canonical
steps:
  - image: ghcr.io/example/hook:latest
    ca:
      - abcdef0
    in:
      - abcdef0:/in/amata.yaml
    out:
      - abcdef0:/out/sbom.spdx.json
    home:
      - abcdef0:.codex/auth.json:ro
`,
		},
		{
			name: "accepts stack-specific image map",
			doc: `
id: stack-image
steps:
  - image:
      default: ghcr.io/example/hook-default:latest
      java-gradle: ghcr.io/example/hook-gradle:latest
`,
		},
		{
			name: "rejects authoring in entry",
			doc: `
id: invalid-in
steps:
  - image: ghcr.io/example/hook:latest
    in:
      - ./amata.yaml:amata.yaml
`,
			wantErr: "invalid short hash",
		},
		{
			name: "rejects absolute home destination",
			doc: `
id: invalid-home
steps:
  - image: ghcr.io/example/hook:latest
    home:
      - abcdef0:/root/.codex/auth.json
`,
			wantErr: "destination must be relative",
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			_, err := LoadSpecYAML([]byte(strings.TrimSpace(tt.doc)), "test")
			if tt.wantErr == "" {
				if err != nil {
					t.Fatalf("LoadSpecYAML() unexpected error: %v", err)
				}
				return
			}
			if err == nil {
				t.Fatalf("LoadSpecYAML() expected error containing %q", tt.wantErr)
			}
			if !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("LoadSpecYAML() error=%q, want substring %q", err.Error(), tt.wantErr)
			}
		})
	}
}
