package app

import (
	"bytes"
	"testing"

	"github.com/iw2rmb/ploy/internal/cli/common"
	"github.com/iw2rmb/ploy/internal/testutil/assertx"
)

// TestWantsHelpFunction tests the common.WantsHelp helper function.
func TestWantsHelpFunction(t *testing.T) {
	tests := []struct {
		name     string
		args     []string
		expected bool
	}{
		{name: "single --help", args: []string{"--help"}, expected: true},
		{name: "single -h", args: []string{"-h"}, expected: true},
		{name: "empty args", args: []string{}, expected: false},
		{name: "nil args", args: nil, expected: false},
		{name: "subcommand only", args: []string{"deploy"}, expected: false},
		{name: "--help with extra arg", args: []string{"--help", "extra"}, expected: false},
		{name: "-h with extra arg", args: []string{"-h", "extra"}, expected: false},
		{name: "subcommand then --help", args: []string{"deploy", "--help"}, expected: false},
		{name: "--Help (wrong case)", args: []string{"--Help"}, expected: false},
		{name: "-H (wrong case)", args: []string{"-H"}, expected: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := common.WantsHelp(tt.args); got != tt.expected {
				t.Errorf("common.WantsHelp(%v) = %v, expected %v", tt.args, got, tt.expected)
			}
		})
	}
}

// TestRootHelpConsistency verifies that ploy --help and ploy help produce identical output.
func TestRootHelpConsistency(t *testing.T) {
	helpFlagBuf := &bytes.Buffer{}
	if err := executeCmd([]string{"--help"}, helpFlagBuf); err != nil {
		t.Fatalf("ploy --help failed: %v", err)
	}
	helpCmdBuf := &bytes.Buffer{}
	if err := executeCmd([]string{"help"}, helpCmdBuf); err != nil {
		t.Fatalf("ploy help failed: %v", err)
	}
	assertx.Contains(t, helpFlagBuf.String(), "Ploy CLI v2")
	if helpFlagBuf.String() != helpCmdBuf.String() {
		t.Errorf("ploy --help and ploy help produce different output:\n--help:\n%s\nhelp:\n%s",
			helpFlagBuf.String(), helpCmdBuf.String())
	}
}
