package step

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/iw2rmb/ploy/internal/workflow/contracts"
)

// caPreambleScript returns the PLOY_CA_CERTS materializer preamble for backward
// compatibility with callers that reference the old function name. New code should
// use envMaterializerPreamble() directly.
func caPreambleScript() string {
	return envMaterializerPreamble()
}

// buildCommandForTool returns the default all-tests command for the given tool.
func buildCommandForTool(workspace string, tool string) ([]string, error) {
	return buildCommandForToolTarget(workspace, tool, contracts.GateProfileTargetAllTests)
}

// buildCommandForToolTarget returns a deterministic command for a tool/target pair.
// The env materializer preamble (PLOY_CA_CERTS trust-store setup) is prepended to
// every gate command so certificates injected via global config are honored.
func buildCommandForToolTarget(workspace string, tool string, target string) ([]string, error) {
	preamble := envMaterializerPreamble()
	switch strings.ToLower(strings.TrimSpace(tool)) {
	case "maven":
		switch strings.TrimSpace(target) {
		case contracts.GateProfileTargetBuild:
			script := preamble + "mvn --ff -B -q -e -DskipTests=true -Dstyle.color=never -f /workspace/pom.xml clean install"
			return []string{"/bin/sh", "-lc", script}, nil
		case contracts.GateProfileTargetAllTests:
			script := preamble + "mvn --ff -B -q -e -DskipTests=false -Dstyle.color=never -f /workspace/pom.xml clean install"
			return []string{"/bin/sh", "-lc", script}, nil
		default:
			return nil, fmt.Errorf("unsupported maven target: %q", target)
		}
	case "gradle":
		gradleExec := "gradle"
		if hasGradleWrapperSpecified(workspace) {
			gradleExec = "./gradlew"
		}
		switch strings.TrimSpace(target) {
		case contracts.GateProfileTargetBuild:
			script := preamble + gradleExec + " -q --stacktrace --build-cache build -x test -p /workspace"
			return []string{"/bin/sh", "-lc", script}, nil
		case contracts.GateProfileTargetAllTests:
			script := preamble + gradleExec + " -q --stacktrace --build-cache test -p /workspace"
			return []string{"/bin/sh", "-lc", script}, nil
		default:
			return nil, fmt.Errorf("unsupported gradle target: %q", target)
		}
	case "go":
		script := preamble + "go test ./..."
		return []string{"/bin/sh", "-lc", script}, nil
	case "cargo":
		script := preamble + "cargo test"
		return []string{"/bin/sh", "-lc", script}, nil
	case "pip", "poetry":
		script := preamble + "python -m compileall -q /workspace"
		return []string{"/bin/sh", "-lc", script}, nil
	default:
		return nil, fmt.Errorf("unsupported build tool: %q", tool)
	}
}

func hasGradleWrapperSpecified(workspace string) bool {
	workspace = strings.TrimSpace(workspace)
	if workspace == "" {
		return false
	}
	p := filepath.Join(workspace, "gradle", "wrapper", "gradle-wrapper.properties")
	info, err := os.Stat(p)
	return err == nil && !info.IsDir()
}
