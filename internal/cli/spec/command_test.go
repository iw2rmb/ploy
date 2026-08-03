package spec

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	domainapi "github.com/iw2rmb/ploy/internal/domain/api"
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
		t.Fatal("schema output does not match embedded schema")
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
		files   map[string]string
		wantErr string
	}{
		{name: "valid", spec: "steps:\n  - image: docker.io/test/mig:latest\n"},
		{name: "unknown root key accepted", spec: "version: old\nsteps:\n  - image: docker.io/test/mig:latest\n"},
		{name: "unknown nested build gate key accepted", spec: "steps:\n  - image: docker.io/test/mig:latest\nbuild_gate:\n  enabled: true\n"},
		{name: "missing hydra input file", spec: "steps:\n  - image: docker.io/test/mig:latest\n    in:\n      - ./missing.yaml:missing.yaml\n", wantErr: "validate local file records"},
		{
			name: "amata include not mounted",
			spec: "steps:\n  - image: docker.io/test/mig:latest\n    in:\n      - ./amata.yaml:amata.yaml\n",
			files: map[string]string{
				"amata.yaml":           "flows:\n  audit: !include ./gradle-assemble.yaml#/flows/audit\n",
				"gradle-assemble.yaml": "flows:\n  audit:\n    steps: []\n",
			},
			wantErr: "target /in/gradle-assemble.yaml is not mounted",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			path := filepath.Join(t.TempDir(), "spec.yaml")
			if err := os.WriteFile(path, []byte(tt.spec), 0o644); err != nil {
				t.Fatalf("write spec: %v", err)
			}
			for rel, content := range tt.files {
				if err := os.WriteFile(filepath.Join(filepath.Dir(path), rel), []byte(content), 0o644); err != nil {
					t.Fatalf("write %s: %v", rel, err)
				}
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

func TestHandleSpecList(t *testing.T) {
	tests := []struct {
		name    string
		args    []string
		wantErr string
	}{
		{name: "live catalog", args: []string{"ls"}},
		{name: "arguments rejected", args: []string{"ls", "extra"}, wantErr: "spec ls takes no arguments"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var listQuery string
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				listQuery = r.URL.RawQuery
				_ = json.NewEncoder(w).Encode(domainapi.NamedSpecListResponse{Specs: []domainapi.NamedSpecCatalogEntry{{
					Name: "upgrade-java", Source: "https://github.com/acme/service", Path: "migs/upgrade.yaml",
					SHA: "0123456789abcdef0123456789abcdef01234567",
				}}})
			}))
			defer srv.Close()
			t.Setenv("PLOY_SERVER_URL", srv.URL)
			t.Setenv("PLOY_AUTH_TOKEN", "test-token")

			var stdout bytes.Buffer
			var stderr bytes.Buffer
			err := Handle(tt.args, &stdout, &stderr)
			if tt.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("Handle(%v) error = %v, want containing %q", tt.args, err, tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("Handle(%v) error = %v", tt.args, err)
			}
			for _, want := range []string{"NAME", "SOURCE", "PATH", "upgrade-java", "github.com/acme/service", "migs/upgrade.yaml", "01234567"} {
				if !strings.Contains(stdout.String(), want) {
					t.Fatalf("stdout = %q, want containing %q", stdout.String(), want)
				}
			}
			if strings.Contains(stdout.String(), "012345678") {
				t.Fatalf("stdout = %q, want 8-character SHA rendering", stdout.String())
			}
			if listQuery != "" {
				t.Fatalf("list query = %q, want empty", listQuery)
			}
		})
	}
}
