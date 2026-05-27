package spec

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/iw2rmb/ploy/internal/workflow/contracts"
)

func TestHandleSpecSchemaPrintsEmbeddedSchema(t *testing.T) {
	t.Parallel()

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	if err := Handle([]string{"schema"}, &stdout, &stderr); err != nil {
		t.Fatalf("Handle(schema) error = %v", err)
	}
	want, err := contracts.MigSpecSchemaJSON()
	if err != nil {
		t.Fatalf("MigSpecSchemaJSON() error = %v", err)
	}
	if got := strings.TrimSpace(stdout.String()); got != strings.TrimSpace(string(want)) {
		t.Fatalf("schema output does not match embedded schema")
	}
	if stderr.Len() != 0 {
		t.Fatalf("stderr = %q, want empty", stderr.String())
	}
}

func TestHandleSpecValidate(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		spec    string
		wantErr string
	}{
		{
			name: "valid",
			spec: "steps:\n  - image: docker.io/test/mig:latest\n",
		},
		{
			name:    "unknown root key",
			spec:    "version: old\nsteps:\n  - image: docker.io/test/mig:latest\n",
			wantErr: "validate spec",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			path := filepath.Join(t.TempDir(), "spec.yaml")
			if err := os.WriteFile(path, []byte(tt.spec), 0o644); err != nil {
				t.Fatalf("write spec: %v", err)
			}
			var stdout bytes.Buffer
			var stderr bytes.Buffer
			err := Handle([]string{"validate", path}, &stdout, &stderr)
			if tt.wantErr == "" {
				if err != nil {
					t.Fatalf("Handle(validate) error = %v", err)
				}
				if !strings.Contains(stderr.String(), "Validated spec "+path) {
					t.Fatalf("stderr = %q, want validation message", stderr.String())
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("Handle(validate) error = %v, want containing %q", err, tt.wantErr)
			}
		})
	}
}
