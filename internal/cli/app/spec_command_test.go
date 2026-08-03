package app

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	domainapi "github.com/iw2rmb/ploy/internal/domain/api"
)

func TestSpecCobraCommandRouting(t *testing.T) {
	tests := []struct {
		name    string
		args    []string
		setup   func(t *testing.T)
		wantOut string
	}{
		{name: "spec ls help", args: []string{"spec", "ls", "--help"}, wantOut: "ploy spec ls [flags]"},
		{
			name: "spec ls routes to handler",
			args: []string{"spec", "ls"},
			setup: func(t *testing.T) {
				t.Helper()
				srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					_ = json.NewEncoder(w).Encode(domainapi.NamedSpecListResponse{Specs: []domainapi.NamedSpecCatalogEntry{{
						Name: "upgrade-java", Source: "https://github.com/acme/service", Path: "migs/upgrade.yaml",
						SHA: "0123456789abcdef0123456789abcdef01234567",
					}}})
				}))
				t.Cleanup(srv.Close)
				t.Setenv("PLOY_SERVER_URL", srv.URL)
			},
			wantOut: "upgrade-java",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.setup != nil {
				tt.setup(t)
			}
			stdout := &bytes.Buffer{}
			stderr := &bytes.Buffer{}
			root := NewRootCmdWithIO(stdout, stderr)
			root.SetArgs(tt.args)
			if err := root.Execute(); err != nil {
				t.Fatalf("Execute(%v) error = %v", tt.args, err)
			}
			output := stdout.String() + stderr.String()
			if !strings.Contains(output, tt.wantOut) {
				t.Fatalf("output = %q, want containing %q", output, tt.wantOut)
			}
		})
	}
}

func TestSpecCobraExposesTargetCommands(t *testing.T) {
	cmd := newSpecCmd(&bytes.Buffer{}, &bytes.Buffer{})
	children := cmd.Commands()
	got := make([]string, 0, len(children))
	for _, child := range children {
		got = append(got, child.Name())
	}
	want := []string{"ls", "schema", "validate"}
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Fatalf("spec commands = %v, want %v", got, want)
	}
}
