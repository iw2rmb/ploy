package step

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/iw2rmb/ploy/internal/workflow/contracts"
)

const (
	mavenWrapperCompileCommand = "./mvnw -B -e clean compile"
	mavenBuildFallbackCommand  = "mvn --ff -B -q -e -DskipTests=true -Dstyle.color=never -f /workspace/pom.xml clean install"
	mavenAllTestsFallbackCmd   = "mvn --ff -B -q -e -DskipTests=false -Dstyle.color=never -f /workspace/pom.xml clean install"
)

// buildCommandForTool returns the default all-tests command for the given tool.
func buildCommandForTool(workspace string, tool string) ([]string, error) {
	return buildCommandForToolTarget(workspace, tool, contracts.GateProfileTargetAllTests)
}

// buildCommandForToolTarget returns a deterministic command for a tool/target pair.
func buildCommandForToolTarget(workspace string, tool string, target string) ([]string, error) {
	wrap := func(toolCmd string) []string {
		parts := []string{"set -eu"}
		parts = append(parts, toolCmd)
		return []string{"/bin/sh", "-lc", strings.Join(parts, "; ")}
	}
	switch strings.ToLower(strings.TrimSpace(tool)) {
	case "maven":
		buildCommand := mavenBuildFallbackCommand
		allTestsCommand := mavenAllTestsFallbackCmd
		if hasMavenWrapperSpecified(workspace) {
			buildCommand = mavenWrapperCompileCommand
			allTestsCommand = mavenWrapperCompileCommand
		}
		switch strings.TrimSpace(target) {
		case contracts.GateProfileTargetBuild:
			return wrap(buildCommand), nil
		case contracts.GateProfileTargetAllTests:
			return wrap(allTestsCommand), nil
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
			return wrap(gradleExec + " -q --stacktrace --build-cache build -x test -p /workspace"), nil
		case contracts.GateProfileTargetAllTests:
			return wrap(gradleExec + " -q --stacktrace --build-cache test -p /workspace"), nil
		default:
			return nil, fmt.Errorf("unsupported gradle target: %q", target)
		}
	case "go":
		return wrap("go test ./..."), nil
	case "cargo":
		return wrap("cargo test"), nil
	case "pip", "poetry":
		return wrap("python -m compileall -q /workspace"), nil
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

func hasMavenWrapperSpecified(workspace string) bool {
	workspace = strings.TrimSpace(workspace)
	if workspace == "" {
		return false
	}
	p := filepath.Join(workspace, "mvnw")
	info, err := os.Stat(p)
	return err == nil && !info.IsDir()
}
