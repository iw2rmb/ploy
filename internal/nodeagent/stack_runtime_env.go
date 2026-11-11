package nodeagent

import (
	"strings"

	"github.com/iw2rmb/ploy/internal/workflow/contracts"
)

// Stack tuple env keys injected into job containers.
const (
	ployStackLanguageEnv = "PLOY_STACK_LANGUAGE"
	ployStackToolEnv     = "PLOY_STACK_TOOL"
	ployStackReleaseEnv  = "PLOY_STACK_RELEASE"
)

func stackExpectationForRequest(req StartRunRequest, fallback contracts.MigStack) *contracts.StackExpectation {
	if exp := contracts.NormalizeStackExpectation(req.DetectedStack); exp != nil {
		return exp
	}
	return contracts.NormalizeStackExpectation(contracts.StackExpectationFromMigStack(fallback))
}

func injectStackTupleEnv(env map[string]string, exp *contracts.StackExpectation) {
	if env == nil {
		return
	}
	exp = contracts.NormalizeStackExpectation(exp)
	if exp == nil {
		return
	}
	if exp.Language != "" {
		env[ployStackLanguageEnv] = exp.Language
	}
	if exp.Tool != "" {
		env[ployStackToolEnv] = exp.Tool
	}
	if exp.Release != "" {
		env[ployStackReleaseEnv] = exp.Release
	}
}

func migStackFromExpectation(exp *contracts.StackExpectation) contracts.MigStack {
	exp = contracts.NormalizeStackExpectation(exp)
	if exp == nil {
		return contracts.MigStackUnknown
	}
	if !strings.EqualFold(exp.Language, "java") {
		return contracts.MigStackUnknown
	}
	return contracts.ToolToMigStack(exp.Tool)
}

func resolveManifestStack(req StartRunRequest, fallback contracts.MigStack) contracts.MigStack {
	if stack := migStackFromExpectation(req.DetectedStack); stack != contracts.MigStackUnknown {
		return stack
	}
	return fallback
}
