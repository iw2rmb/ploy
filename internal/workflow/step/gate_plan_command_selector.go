package step

import (
	"errors"
	"fmt"
	"strings"

	"github.com/iw2rmb/ploy/internal/workflow/contracts"
)

var errGateTargetUnsupported = errors.New("gate target unsupported")

func resolveGateCommand(
	workspace string,
	language string,
	tool string,
	release string,
	prep *contracts.BuildGateProfileOverride,
	target string,
) ([]string, map[string]string, error) {
	wantedTarget := strings.TrimSpace(target)

	if prep != nil && !prep.Command.IsEmpty() {
		prepTarget := strings.TrimSpace(prep.Target)
		if wantedTarget == "" || prepTarget == wantedTarget {
			if prep.Stack != nil {
				if !stackMatchesPrepOverride(prep.Stack, language, tool, release) {
					return nil, nil, fmt.Errorf("prep stack mismatch: expected %s/%s/%s, got %s/%s/%s",
						strings.TrimSpace(prep.Stack.Language),
						strings.TrimSpace(prep.Stack.Tool),
						strings.TrimSpace(prep.Stack.Release),
						strings.TrimSpace(language),
						strings.TrimSpace(tool),
						strings.TrimSpace(release),
					)
				}
			}
			return wrapWithMaterializerPreamble(prep.Command.ToSlice()), contracts.CopyEnv(prep.Env), nil
		}
	}

	if wantedTarget != "" {
		cmd, err := buildCommandForToolTarget(workspace, tool, wantedTarget)
		if err != nil {
			return nil, nil, fmt.Errorf("%w: %s", errGateTargetUnsupported, err.Error())
		}
		return cmd, nil, nil
	}

	cmd, err := buildCommandForTool(workspace, tool)
	if err != nil {
		return nil, nil, err
	}
	return cmd, nil, nil
}

// wrapWithMaterializerPreamble prepends the env materializer preamble to a
// container command regardless of whether the command was tool-derived or
// profile-override-derived. Under the Hydra-only contract the materializer
// registry is empty, so this is a no-op pass-through.
func wrapWithMaterializerPreamble(cmd []string) []string {
	preamble := envMaterializerPreamble()
	if preamble == "" {
		return cmd
	}
	// Shell-wrapped command: ["/bin/sh", "-c", "actual command"]
	if len(cmd) == 3 && cmd[0] == "/bin/sh" && cmd[1] == "-c" {
		return []string{cmd[0], cmd[1], preamble + cmd[2]}
	}
	// Exec-form: wrap in shell to apply preamble before the command.
	var script strings.Builder
	script.WriteString(preamble)
	script.WriteString("exec")
	for _, a := range cmd {
		script.WriteString(" '")
		script.WriteString(strings.ReplaceAll(a, "'", "'\\''"))
		script.WriteString("'")
	}
	return []string{"/bin/sh", "-c", script.String()}
}

func stackMatchesPrepOverride(stack *contracts.GateProfileStack, language, tool, release string) bool {
	if stack == nil {
		return true
	}
	return contracts.StackFieldsMatch(language, tool, release, stack.Language, stack.Tool, stack.Release)
}
