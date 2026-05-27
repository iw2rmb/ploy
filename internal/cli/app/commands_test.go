package app

import (
	"bytes"
	"strings"
	"testing"
)

func TestRootCmdCobraBehavior(t *testing.T) {
	t.Run("version flag", func(t *testing.T) {
		stdout := &bytes.Buffer{}
		stderr := &bytes.Buffer{}
		rootCmd := NewRootCmdWithIO(stdout, stderr)
		rootCmd.SetArgs([]string{"--version"})

		err := rootCmd.Execute()
		if err != nil {
			t.Errorf("expected nil error for --version, got: %v", err)
		}

		output := stdout.String()
		if !strings.Contains(output, "dev") {
			t.Errorf("expected version output in stdout, got: %q", output)
		}
		if stderr.Len() != 0 {
			t.Errorf("expected empty stderr for version output, got: %q", stderr.String())
		}
	})
}

func TestCompletionCommandGeneratesScripts(t *testing.T) {
	t.Parallel()

	tests := []struct {
		shell string
		want  string
	}{
		{shell: "bash", want: "bash completion"},
		{shell: "zsh", want: "#compdef"},
		{shell: "fish", want: "complete -c ploy"},
		{shell: "powershell", want: "Register-ArgumentCompleter"},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.shell, func(t *testing.T) {
			t.Parallel()

			stdout := &bytes.Buffer{}
			stderr := &bytes.Buffer{}
			rootCmd := NewRootCmdWithIO(stdout, stderr)
			rootCmd.SetArgs([]string{"completion", tt.shell})
			if err := rootCmd.Execute(); err != nil {
				t.Fatalf("completion %s failed: %v", tt.shell, err)
			}
			if got := stdout.String(); !strings.Contains(got, tt.want) {
				t.Fatalf("completion %s output missing %q:\n%s", tt.shell, tt.want, got)
			}
		})
	}
}

func TestCobraCommandTreeRouting(t *testing.T) {
	tests := []struct {
		name    string
		args    []string
		wantOK  bool
		wantOut string
		wantErr string
	}{
		{name: "root parent help", args: nil, wantOK: true, wantOut: "Available Commands:"},
		{name: "cluster parent help", args: []string{"cluster"}, wantOK: true, wantOut: "ploy cluster [command]"},
		{name: "cluster node parent help", args: []string{"cluster", "node"}, wantOK: true, wantOut: "ploy cluster node [command]"},
		{name: "config env parent help", args: []string{"config", "env"}, wantOK: true, wantOut: "ploy config env [command]"},
		{name: "mig repo parent help", args: []string{"mig", "repo"}, wantOK: true, wantOut: "ploy mig repo [command]"},
		{name: "top-level unknown", args: []string{"unknown"}, wantErr: `unknown command "unknown"`},
		{name: "cluster unknown", args: []string{"cluster", "unknown"}, wantErr: `unknown command "unknown"`},
		{name: "cluster node unknown", args: []string{"cluster", "node", "unknown"}, wantErr: `unknown command "unknown"`},
		{name: "job follow unknown", args: []string{"job", "follow"}, wantErr: `unknown command "follow"`},
		{name: "help run status", args: []string{"help", "run", "status"}, wantOK: true, wantOut: "ploy run status [--json] <run-id> [flags]"},
		{name: "run status help", args: []string{"run", "status", "--help"}, wantOK: true, wantOut: "ploy run status [--json] <run-id> [flags]"},
		{name: "run logs help", args: []string{"run", "logs", "--help"}, wantOK: true, wantOut: "ploy run logs <run-id> [flags]"},
		{name: "token list help", args: []string{"cluster", "token", "list", "--help"}, wantOK: true, wantOut: "ploy cluster token list [flags]"},
		{name: "spec validate help", args: []string{"spec", "validate", "--help"}, wantOK: true, wantOut: "ploy spec validate <path> [<path>...] [flags]"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			buf := &bytes.Buffer{}
			err := executeCmd(tt.args, buf)
			if tt.wantOK && err != nil {
				t.Fatalf("expected success, got %v", err)
			}
			if !tt.wantOK {
				if err == nil {
					t.Fatal("expected error, got nil")
				}
				if !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("expected error containing %q, got %v", tt.wantErr, err)
				}
			}
			if tt.wantOut != "" && !strings.Contains(buf.String(), tt.wantOut) {
				t.Fatalf("expected output containing %q, got %q", tt.wantOut, buf.String())
			}
		})
	}
}
