// claimer_spec.go isolates spec JSON payload parsing from claim orchestration.
//
// This file contains parseSpec which decodes run specifications from the
// control plane claim response. It uses the canonical contracts.ParseModsSpecJSON
// parser for structured validation and then converts to the internal RunOptions
// format. Separating spec parsing from claim logic enables focused testing of
// the decoding contract without coupling to HTTP claim mechanics.
package nodeagent

import (
	"encoding/json"

	"github.com/iw2rmb/ploy/internal/workflow/contracts"
)

// parseSpec parses a spec JSON payload into environment variables and typed options.
// It uses the canonical contracts.ParseModsSpecJSON parser for structured
// validation, then converts to the internal RunOptions format.
//
// The spec is expected to contain fields like "steps", "env", "build_gate",
// and other configuration values.
//
// ## Return Values
//
// Returns:
//   - env: map[string]string containing merged environment variables (global env,
//     plus step env for single-step runs; multi-step step env is handled per-step
//     during manifest building).
//   - typedOpts: RunOptions with typed accessors for all understood option keys.
//   - err: non-nil if spec parsing fails.
//
// If the spec is empty, returns an empty env map and zero RunOptions with nil error.
func parseSpec(spec json.RawMessage) (map[string]string, RunOptions, error) {
	env := map[string]string{}
	var typedOpts RunOptions
	if len(spec) == 0 {
		return env, typedOpts, nil
	}

	// Parse using the canonical parser for structural validation.
	modsSpec, err := contracts.ParseModsSpecJSON(spec)
	if err != nil {
		return env, typedOpts, err
	}

	// Derive env with legacy semantics:
	// - Global env applies to every step.
	// - For single-step runs, step env is merged into env (step overrides).
	// - For multi-step runs, env contains only the global env; step env is applied
	//   at manifest build time via typedOpts.Steps[stepIndex].Env.
	env = modsSpecToEnv(modsSpec)

	// Direct conversion from typed ModsSpec to RunOptions.
	typedOpts = modsSpecToRunOptions(modsSpec)

	return env, typedOpts, nil
}

func modsSpecToEnv(spec *contracts.ModsSpec) map[string]string {
	if spec == nil {
		return map[string]string{}
	}

	env := make(map[string]string, len(spec.Env))
	for k, v := range spec.Env {
		env[k] = v
	}

	if len(spec.Steps) == 1 {
		for k, v := range spec.Steps[0].Env {
			env[k] = v
		}
	}

	return env
}
