package configure

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/iw2rmb/ploy/internal/testutil/clienv"
)

// TestHandleConfigEnvShow_FlagValidation verifies flag parsing for the 'show' subcommand.
func TestHandleConfigEnvShow_FlagValidation(t *testing.T) {
	tests := []struct {
		name            string
		args            []string
		wantErrContains string
	}{
		{"missing key", nil, "--key is required"},
		{"empty key", []string{"--key", ""}, "--key is required"},
		{"invalid from", []string{"--key", "FOO", "--from", "bogus"}, "invalid --from target"},
		{"empty from", []string{"--key", "FOO", "--from", ""}, "--from value cannot be empty"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			clienv.RunExpectError(t, executeConfigEnvShow, tt.args, tt.wantErrContains)
		})
	}
}

// TestHandleConfigEnvSet_FlagValidation verifies flag parsing for the 'set' subcommand.
func TestHandleConfigEnvSet_FlagValidation(t *testing.T) {
	tests := []struct {
		name            string
		args            []string
		wantErrContains string
	}{
		{"missing key", []string{"--value", "test"}, "--key is required"},
		{"empty key", []string{"--key", "", "--value", "test"}, "--key is required"},
		{"missing value and file", []string{"--key", "FOO"}, "either --value or --file is required"},
		{"value and file exclusive", []string{"--key", "FOO", "--value", "bar", "--file", "test.txt"}, "--value and --file are mutually exclusive"},
		{"invalid on selector", []string{"--key", "FOO", "--value", "bar", "--on", "invalid"}, "invalid --on selector"},
		{"file not found", []string{"--key", "FOO", "--file", "/nonexistent/path/file.txt"}, "read file"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			clienv.RunExpectError(t, executeConfigEnvSet, tt.args, tt.wantErrContains)
		})
	}
}

// TestHandleConfigEnvUnset_FlagValidation verifies flag parsing for the 'unset' subcommand.
func TestHandleConfigEnvUnset_FlagValidation(t *testing.T) {
	tests := []struct {
		name            string
		args            []string
		wantErrContains string
	}{
		{"missing key", nil, "--key is required"},
		{"empty key", []string{"--key", ""}, "--key is required"},
		{"invalid from", []string{"--key", "FOO", "--from", "bogus"}, "invalid --from target"},
		{"empty from", []string{"--key", "FOO", "--from", ""}, "--from value cannot be empty"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			clienv.RunExpectError(t, executeConfigEnvUnset, tt.args, tt.wantErrContains)
		})
	}
}

// TestHandleConfigEnvSetValidOnSelectors verifies that all valid --on values are accepted.
func TestHandleConfigEnvSetValidOnSelectors(t *testing.T) {
	validSelectors := []string{"all", "jobs", "server", "nodes", "gates", "steps"}
	for _, sel := range validSelectors {
		t.Run(sel, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusOK)
			}))
			defer srv.Close()
			clienv.UseServerDescriptor(t, srv.URL)
			buf := &bytes.Buffer{}
			err := executeConfigEnvSet([]string{"--key", "FOO", "--value", "bar", "--on", sel}, buf)
			if err != nil {
				t.Fatalf("selector %q should be valid, got: %v", sel, err)
			}
		})
	}
}

// TestHandleConfigEnvSetOnAllExclusive verifies that --on all cannot be combined with other selectors.
func TestHandleConfigEnvSetOnAllExclusive(t *testing.T) {
	tests := []struct {
		name string
		args []string
	}{
		{name: "all then gates", args: []string{"--key", "FOO", "--value", "bar", "--on", "all", "--on", "gates"}},
		{name: "gates then all", args: []string{"--key", "FOO", "--value", "bar", "--on", "gates", "--on", "all"}},
		{name: "all then jobs", args: []string{"--key", "FOO", "--value", "bar", "--on", "all", "--on", "jobs"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			clienv.RunExpectError(t, executeConfigEnvSet, tt.args, "--on all is exclusive")
		})
	}
}

// TestHandleConfigEnvSetMultipleOnSelectors verifies that multiple --on selectors are accepted and deduplicated.
func TestHandleConfigEnvSetMultipleOnSelectors(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()
	clienv.UseServerDescriptor(t, srv.URL)
	buf := &bytes.Buffer{}
	err := executeConfigEnvSet([]string{"--key", "FOO", "--value", "bar", "--on", "gates", "--on", "steps"}, buf)
	if err != nil {
		t.Fatalf("selectors should be valid, got: %v", err)
	}
}

// TestExpandOnSelector verifies selector expansion and validation.
func TestExpandOnSelector(t *testing.T) {
	tests := []struct {
		name      string
		selector  string
		wantNames []string
		wantErr   string
	}{
		{name: "all expands to four targets", selector: "all", wantNames: []string{"gates", "nodes", "server", "steps"}},
		{name: "jobs expands to gates+steps", selector: "jobs", wantNames: []string{"gates", "steps"}},
		{name: "server single", selector: "server", wantNames: []string{"server"}},
		{name: "nodes single", selector: "nodes", wantNames: []string{"nodes"}},
		{name: "gates single", selector: "gates", wantNames: []string{"gates"}},
		{name: "steps single", selector: "steps", wantNames: []string{"steps"}},
		{name: "invalid selector", selector: "bogus", wantErr: "invalid --on selector"},
		{name: "empty selector", selector: "", wantErr: "invalid --on selector"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			targets, err := expandOnSelector(tt.selector)
			if tt.wantErr != "" {
				if err == nil {
					t.Fatalf("expected error containing %q", tt.wantErr)
				}
				if !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("expected error containing %q, got: %v", tt.wantErr, err)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			got := make([]string, len(targets))
			for i, tgt := range targets {
				got[i] = tgt.String()
			}
			if len(got) != len(tt.wantNames) {
				t.Fatalf("expected %v, got %v", tt.wantNames, got)
			}
			for i := range got {
				if got[i] != tt.wantNames[i] {
					t.Fatalf("expected %v, got %v", tt.wantNames, got)
				}
			}
		})
	}
}
