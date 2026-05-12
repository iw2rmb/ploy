package main

import (
	"testing"

	"github.com/iw2rmb/ploy/internal/testutil/assertx"
	"github.com/iw2rmb/ploy/internal/testutil/clienv"
)

func TestRunSubcommandHelpUsesLeafUsage(t *testing.T) {
	tests := []struct {
		name      string
		baseArgs  []string
		wantUsage string
	}{
		{
			name:      "run status",
			baseArgs:  []string{"run", "status"},
			wantUsage: "Usage: ploy run status [--json] <run-id>",
		},
		{
			name:      "run logs",
			baseArgs:  []string{"run", "logs"},
			wantUsage: "Usage: ploy run logs [--max-retries <n>] [--idle-timeout <duration>] [--timeout <duration>] <run-id>",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			for _, flag := range []string{"--help", "-h"} {
				args := append(append([]string{}, tt.baseArgs...), flag)
				out := clienv.RunExpectOK(t, executeCmd, args)
				assertx.Contains(t, out, tt.wantUsage)
				assertx.NotContains(t, out, "Usage: ploy run <command>")
			}
		})
	}
}
