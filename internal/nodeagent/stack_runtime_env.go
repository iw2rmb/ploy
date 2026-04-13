package nodeagent

import (
	"strings"

	"github.com/iw2rmb/ploy/internal/workflow/contracts"
	"github.com/iw2rmb/ploy/internal/workflow/lifecycle"
)

func stackExpectationForRequest(req StartRunRequest, fallback contracts.MigStack) *contracts.StackExpectation {
	if exp := contracts.NormalizeStackExpectation(req.DetectedStack); exp != nil {
		return exp
	}
	return contracts.NormalizeStackExpectation(lifecycle.StackExpectationFromMigStack(fallback))
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
		env[contracts.PLOYStackLanguageEnv] = exp.Language
	}
	if exp.Tool != "" {
		env[contracts.PLOYStackToolEnv] = exp.Tool
	}
	if exp.Release != "" {
		env[contracts.PLOYStackReleaseEnv] = exp.Release
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
