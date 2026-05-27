package app

import (
	"strings"
	"testing"

	"github.com/iw2rmb/ploy/internal/testutil/assertx"
	"github.com/iw2rmb/ploy/internal/testutil/clienv"
)

func TestHelpRegressionLeafHelpReturnsNoError(t *testing.T) {
	t.Parallel()

	tests := []struct {
		args          []string
		expectSnippet string
	}{
		{args: []string{"run", "ls", "--help"}, expectSnippet: "Usage: ploy run ls"},
		{args: []string{"run", "cancel", "--help"}, expectSnippet: "Usage: ploy run cancel"},
		{args: []string{"run", "start", "--help"}, expectSnippet: "Usage: ploy run start"},
		{args: []string{"cluster", "node", "add", "--help"}, expectSnippet: "Usage: ploy cluster node add"},
		{args: []string{"cluster", "token", "list", "--help"}, expectSnippet: "Usage: ploy cluster token list"},
		{args: []string{"config", "env", "set", "--help"}, expectSnippet: "Usage: ploy config env set"},
		{args: []string{"spec", "schema", "--help"}, expectSnippet: "Usage: ploy spec schema"},
		{args: []string{"spec", "validate", "--help"}, expectSnippet: "Usage: ploy spec validate"},
		{args: []string{"mig", "run", "repo", "--help"}, expectSnippet: "Usage: ploy mig run repo"},
	}

	for _, tt := range tests {
		t.Run(strings.Join(tt.args, " "), func(t *testing.T) {
			out := clienv.RunExpectOK(t, executeCmd, tt.args)
			assertx.Contains(t, out, tt.expectSnippet)
		})
	}
}

func TestHelpRegressionCommandRoutesDeepPaths(t *testing.T) {
	t.Parallel()

	tests := []struct {
		args          []string
		expectSnippet string
	}{
		{args: []string{"help", "spec"}, expectSnippet: "Usage: ploy spec <command>"},
		{args: []string{"help", "spec", "schema"}, expectSnippet: "Usage: ploy spec schema"},
		{args: []string{"help", "cluster", "node"}, expectSnippet: "Usage: ploy cluster node <command>"},
		{args: []string{"help", "run", "status"}, expectSnippet: "Usage: ploy run status [--json] <run-id>"},
	}

	for _, tt := range tests {
		t.Run(strings.Join(tt.args, " "), func(t *testing.T) {
			out := clienv.RunExpectOK(t, executeCmd, tt.args)
			assertx.Contains(t, out, tt.expectSnippet)
		})
	}
}
