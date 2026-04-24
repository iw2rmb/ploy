package step

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/iw2rmb/ploy/internal/workflow/contracts"
)

func TestBuildCommandForToolTarget_MavenWrapperPreference(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name        string
		target      string
		withWrapper bool
		wantCommand string
	}{
		{
			name:        "build_without_wrapper_uses_maven_fallback",
			target:      contracts.GateProfileTargetBuild,
			withWrapper: false,
			wantCommand: mavenBuildFallbackCommand,
		},
		{
			name:        "all_tests_without_wrapper_uses_maven_fallback",
			target:      contracts.GateProfileTargetAllTests,
			withWrapper: false,
			wantCommand: mavenAllTestsFallbackCmd,
		},
		{
			name:        "build_with_wrapper_uses_mvnw_compile",
			target:      contracts.GateProfileTargetBuild,
			withWrapper: true,
			wantCommand: mavenWrapperCompileCommand,
		},
		{
			name:        "all_tests_with_wrapper_uses_mvnw_compile",
			target:      contracts.GateProfileTargetAllTests,
			withWrapper: true,
			wantCommand: mavenWrapperCompileCommand,
		},
	}

	for _, tc := range testCases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			workspace := createMavenWorkspace(t, "17")
			if tc.withWrapper {
				if err := os.WriteFile(filepath.Join(workspace, "mvnw"), []byte("#!/bin/sh\n"), 0o755); err != nil {
					t.Fatalf("write mvnw: %v", err)
				}
			}

			got, err := buildCommandForToolTarget(workspace, "maven", tc.target)
			if err != nil {
				t.Fatalf("buildCommandForToolTarget() error: %v", err)
			}
			if len(got) != 3 {
				t.Fatalf("buildCommandForToolTarget() len=%d, want 3", len(got))
			}

			commandScript := got[2]
			if !strings.Contains(commandScript, tc.wantCommand) {
				t.Fatalf("command=%q, want to contain %q", commandScript, tc.wantCommand)
			}
		})
	}
}
