// run_options.go defines typed option structs for nodeagent execution.
//
// This file introduces small, focused option structs that clarify which spec/options
// keys are understood by the nodeagent. These types replace untyped map[string]any
// lookups throughout the package while preserving raw JSON where needed for
// wire-level compatibility. The typed options align with the roadmap goal of
// reducing map[string]any casts and improving internal type safety.
//
// ## Stack-Aware Image Selection
//
// The Image field in StepMod, HealingMod, and ExecutionOptions uses the
// contracts.ModImage type to support both universal images (string) and
// stack-specific images (map keyed by stack). Resolution happens at manifest
// build time using the detected stack from Build Gate.
package nodeagent

import (
	"strings"

	domaintypes "github.com/iw2rmb/ploy/internal/domain/types"
	"github.com/iw2rmb/ploy/internal/workflow/contracts"
)

// RunOptions holds all typed configuration options for a run execution.
// It aggregates build gate configuration, healing policy, MR creation wiring,
// execution shaping parameters, and artifact collection settings.
//
// RunOptions is populated from StartRunRequest.Options by parseRunOptions and
// consumed by buildManifestFromRequest and execution orchestration phases.
//
// This type is the canonical source of truth for run options. Raw map[string]any
// access should not be used; all option keys understood by the nodeagent are
// exposed as typed fields on this struct.
type RunOptions struct {
	// BuildGate configures pre-mod build gate validation.
	BuildGate BuildGateOptions

	// Healing configures the heal → re-gate loop when build gate fails.
	Healing *HealingConfig

	// MRWiring configures GitLab merge request creation.
	MRWiring MRWiringOptions

	// MRFlagsPresent tracks which MR flags were explicitly set in the spec.
	// This enables distinguishing between "not set" and "set to false".
	MRFlagsPresent MRFlagsPresence

	// Execution configures container image, command, and retention.
	Execution ExecutionOptions

	// Artifacts configures artifact collection and upload.
	Artifacts ArtifactOptions

	// ServerMetadata holds server-injected metadata for uploads and tracking.
	ServerMetadata ServerMetadataOptions

	// ModIndex is the server-injected index for multi-step runs. It selects
	// which entry in Steps[] to use for manifest building. Defaults to 0.
	// For single-step runs, this field is ignored.
	ModIndex int

	// ModIndexSet is true when mod_index was explicitly provided in the spec.
	// This distinguishes between "not set" (use 0) and "set to 0".
	ModIndexSet bool

	// Steps holds the list of mod steps for multi-step runs (mods[] array).
	// For single-step runs, this slice is empty and Execution options are used.
	// For multi-step runs, this slice contains one entry per mod in mods[].
	Steps []StepMod
}

// BuildGateOptions configures pre-mod build gate validation.
// These options control whether the gate runs, which profile to use, and
// environment variables to inject into the gate execution context.
type BuildGateOptions struct {
	// Enabled controls whether the build gate runs before the main mod execution.
	Enabled bool

	// Profile specifies the gate profile name (e.g., "java-auto", "java-maven").
	// The profile determines which static analysis tools and checks to run.
	Profile string
}

// HealingConfig describes the heal → re-gate loop configuration.
// When the build gate fails, the agent can execute one or more healing mods
// grouped into named strategies (branches), then re-run the gate. This struct
// specifies the retry limit and the healing strategies to execute.
type HealingConfig struct {
	// Retries is the maximum number of healing attempts (default: 1).
	// Each retry executes all healing mods in sequence, then re-runs the gate.
	// For multi-strategy configs, retries apply per-strategy.
	Retries int

	// Strategies is the list of healing strategies (branches) to attempt.
	// Each strategy has a name and its own mods[] list. Strategies can be
	// executed in parallel by the control plane; the first to pass re-gate wins.
	// When Strategies is populated, Mods is ignored.
	Strategies []HealingStrategy
}

// HealingStrategy represents a named healing branch with its own sequence of mods.
// Multiple strategies can be executed in parallel by the control plane to race
// different healing approaches (e.g., AI-powered vs. static patches).
//
// Strategy Semantics:
//   - Each strategy operates on an independent workspace clone.
//   - Strategies execute in parallel (subject to available nodes).
//   - Each strategy runs its Mods sequentially, then re-gates.
//   - The first strategy whose re-gate passes wins; others are canceled.
//   - If all strategies exhaust retries without passing, the run fails.
type HealingStrategy struct {
	// Name is an optional identifier for this strategy (e.g., "codex-ai", "static-patch").
	// Used for logging, metrics, and debugging. If empty, defaults to "strategy-<index>".
	Name string

	// Mods is the list of healing mod specifications for this strategy.
	// Executed sequentially; after all mods complete, re-gate is triggered.
	Mods []HealingMod
}

// NormalizedStrategies returns the healing strategies to use for execution.
// If Strategies is populated, returns it directly. Returns nil if no healing
// configuration is present.
func (h *HealingConfig) NormalizedStrategies() []HealingStrategy {
	if h == nil {
		return nil
	}
	if len(h.Strategies) > 0 {
		return h.Strategies
	}
	return nil
}

// HealingMod describes a single healing mod container specification.
// Healing mods run after gate failure to attempt workspace fixes before re-gate.
type HealingMod struct {
	// Image is the container image for the healing mod (required).
	// Supports both universal images (string) and stack-specific images (map).
	// Resolution to a concrete image happens at manifest build time using the
	// detected stack from Build Gate.
	Image contracts.ModImage

	// Command is the container command override (optional).
	// Can be a single shell string or an array of command + args.
	Command HealingCommand

	// Env holds environment variables to inject into the healing container.
	Env map[string]string

	// RetainContainer controls whether the healing container is retained for debugging.
	RetainContainer bool
}

// HealingCommand represents a healing mod command as either a shell string or exec array.
// This type encapsulates the polymorphic command representation in healing mod specs.
type HealingCommand struct {
	// Shell holds the command when specified as a single shell string.
	// When non-empty, the command is executed via ["/bin/sh", "-c", Shell].
	Shell string

	// Exec holds the command when specified as an exec array.
	// When non-nil, the command is executed directly without a shell.
	Exec []string
}

// IsEmpty returns true if no command is specified.
func (c HealingCommand) IsEmpty() bool {
	return c.Shell == "" && len(c.Exec) == 0
}

// ToSlice converts the command to a []string suitable for manifest execution.
// Returns nil if the command is empty.
func (c HealingCommand) ToSlice() []string {
	if len(c.Exec) > 0 {
		return c.Exec
	}
	if c.Shell != "" {
		return []string{"/bin/sh", "-c", c.Shell}
	}
	return nil
}

// MRWiringOptions configures GitLab merge request creation for run outcomes.
// These options control when MRs are created and how to authenticate with GitLab.
type MRWiringOptions struct {
	// GitLabPAT is the personal access token for GitLab API authentication.
	// This value is never logged and is only passed to the GitLab client.
	GitLabPAT string

	// GitLabDomain is the GitLab instance domain (e.g., "gitlab.com").
	GitLabDomain string

	// MROnSuccess controls whether to create an MR when the run succeeds.
	MROnSuccess bool

	// MROnFail controls whether to create an MR when the run fails.
	MROnFail bool
}

// ExecutionOptions configures container execution parameters.
// These options shape how the main mod container is configured and retained.
type ExecutionOptions struct {
	// Image is the container image to run (default: "ubuntu:latest").
	// Supports both universal images (string) and stack-specific images (map).
	// Resolution to a concrete image happens at manifest build time using the
	// detected stack from Build Gate.
	Image contracts.ModImage

	// Command is the container command override (optional).
	// Can be a shell string or an array of command + args.
	Command ExecutionCommand

	// RetainContainer controls whether the container is retained after run completion.
	RetainContainer bool
}

// ExecutionCommand represents a mod command as either a shell string or exec array.
// This type mirrors HealingCommand but is used for main mod execution.
type ExecutionCommand struct {
	// Shell holds the command when specified as a single shell string.
	Shell string

	// Exec holds the command when specified as an exec array.
	Exec []string
}

// IsEmpty returns true if no command is specified.
func (c ExecutionCommand) IsEmpty() bool {
	return c.Shell == "" && len(c.Exec) == 0
}

// ToSlice converts the command to a []string suitable for manifest execution.
// Returns nil if the command is empty.
func (c ExecutionCommand) ToSlice() []string {
	if len(c.Exec) > 0 {
		return c.Exec
	}
	if c.Shell != "" {
		return []string{"/bin/sh", "-c", c.Shell}
	}
	return nil
}

// ArtifactOptions configures artifact collection and upload.
// These options specify which workspace files/directories to bundle and upload,
// and how to name the uploaded bundle.
type ArtifactOptions struct {
	// Paths is the list of workspace-relative files/directories to upload.
	// Each path is relative to /workspace and can be a file or directory.
	Paths []string

	// Name is the custom name for the uploaded artifact bundle (optional).
	// If empty, the server generates a default name based on run_id and job_id.
	Name string
}

// MRFlagsPresence tracks whether MR creation flags were explicitly set in the spec.
// This enables distinguishing between "not set" and "set to false" for MR wiring options.
type MRFlagsPresence struct {
	// MROnSuccessSet is true when mr_on_success was explicitly specified in the spec.
	MROnSuccessSet bool

	// MROnFailSet is true when mr_on_fail was explicitly specified in the spec.
	MROnFailSet bool
}

// ServerMetadataOptions holds server-injected metadata for uploads and tracking.
// These options are populated by the control plane and used by the nodeagent
// for status reporting, artifact uploads, and run correlation.
type ServerMetadataOptions struct {
	// JobID is the server-provided job identifier for upload correlation.
	// This value is used to associate artifacts and status updates with a job.
	JobID domaintypes.JobID
}

// StepMod describes a single mod step in a multi-step run (mods[] array).
// Each step has its own image, command, and environment configuration.
// Steps execute sequentially with shared workspace, each running gate+mod.
type StepMod struct {
	// Image is the container image for this step (required).
	// Supports both universal images (string) and stack-specific images (map).
	// Resolution to a concrete image happens at manifest build time using the
	// detected stack from Build Gate.
	Image contracts.ModImage

	// Command is the container command override for this step (optional).
	Command ExecutionCommand

	// Env holds environment variables specific to this step.
	Env map[string]string

	// RetainContainer controls whether this step's container is retained.
	RetainContainer bool
}

// parseRunOptions extracts typed options from untyped map[string]any.
// This function centralizes the map[string]any → RunOptions conversion and
// provides a single point for option validation and defaulting.
//
// All callers should use the typed RunOptions struct instead of accessing
// the raw map directly. The typed struct is the canonical source of truth.
func parseRunOptions(opts map[string]any) RunOptions {
	runOpts := RunOptions{}

	// Parse build gate options (flattened by parseSpec).
	if enabled, ok := opts["build_gate_enabled"].(bool); ok {
		runOpts.BuildGate.Enabled = enabled
	}
	if profile, ok := opts["build_gate_profile"].(string); ok {
		runOpts.BuildGate.Profile = profile
	}

	// Parse healing configuration (multi-strategy form).
	if healingMap, ok := opts["build_gate_healing"].(map[string]any); ok {
		healing := &HealingConfig{
			Retries: 1, // Default to 1 retry.
		}

		// Extract retries (handle both int and float64 from JSON unmarshaling).
		if r, ok := healingMap["retries"].(int); ok && r > 0 {
			healing.Retries = r
		} else if rf, ok := healingMap["retries"].(float64); ok && rf > 0 {
			healing.Retries = int(rf)
		}

		// Multi-strategy form: strategies[] is the canonical schema.
		if strategiesSlice, ok := healingMap["strategies"].([]any); ok && len(strategiesSlice) > 0 {
			for _, stratEntry := range strategiesSlice {
				if stratMap, ok := stratEntry.(map[string]any); ok {
					strategy := parseHealingStrategy(stratMap)
					healing.Strategies = append(healing.Strategies, strategy)
				}
			}
		}

		// Only treat healing as configured when at least one strategy is present.
		if len(healing.Strategies) > 0 {
			runOpts.Healing = healing
		}
	}

	// Parse MR wiring options.
	if pat, ok := opts["gitlab_pat"].(string); ok {
		runOpts.MRWiring.GitLabPAT = pat
	}
	if domain, ok := opts["gitlab_domain"].(string); ok {
		runOpts.MRWiring.GitLabDomain = domain
	}
	// Track MR flag presence separately from their values to distinguish
	// between "not set" and "set to false".
	if mrSuccess, ok := opts["mr_on_success"].(bool); ok {
		runOpts.MRWiring.MROnSuccess = mrSuccess
		runOpts.MRFlagsPresent.MROnSuccessSet = true
	}
	if mrFail, ok := opts["mr_on_fail"].(bool); ok {
		runOpts.MRWiring.MROnFail = mrFail
		runOpts.MRFlagsPresent.MROnFailSet = true
	}

	// Parse execution options.
	// Image supports both string (universal) and map (stack-specific) forms.
	// ParseModImage handles the polymorphic conversion.
	if imgVal, hasImage := opts["image"]; hasImage {
		if modImage, err := contracts.ParseModImage(imgVal); err == nil {
			runOpts.Execution.Image = modImage
		}
		// On parse error, leave Image empty; validation happens at manifest build.
	}
	if retain, ok := opts["retain_container"].(bool); ok {
		runOpts.Execution.RetainContainer = retain
	}

	// Parse command (polymorphic: string or []string).
	if cmdStr, ok := opts["command"].(string); ok {
		runOpts.Execution.Command.Shell = cmdStr
	} else if cmdSlice, ok := opts["command"].([]string); ok {
		runOpts.Execution.Command.Exec = cmdSlice
	}

	// Parse artifact options.
	if name, ok := opts["artifact_name"].(string); ok {
		runOpts.Artifacts.Name = name
	}
	// Parse artifact_paths (accepts both []any from JSON and []string from programmatic callers).
	switch paths := opts["artifact_paths"].(type) {
	case []any:
		for _, p := range paths {
			if s, ok := p.(string); ok && strings.TrimSpace(s) != "" {
				runOpts.Artifacts.Paths = append(runOpts.Artifacts.Paths, s)
			}
		}
	case []string:
		for _, s := range paths {
			if strings.TrimSpace(s) != "" {
				runOpts.Artifacts.Paths = append(runOpts.Artifacts.Paths, s)
			}
		}
	}

	// Parse server metadata.
	if jobID, ok := opts["job_id"].(string); ok {
		if trimmed := strings.TrimSpace(jobID); trimmed != "" {
			runOpts.ServerMetadata.JobID = domaintypes.JobID(trimmed)
		}
	}

	// Parse mod_index for multi-step runs (server-injected per-job index).
	// Handle both int and float64 (JSON unmarshals numbers as float64).
	switch mi := opts["mod_index"].(type) {
	case int:
		runOpts.ModIndex = mi
		runOpts.ModIndexSet = true
	case float64:
		runOpts.ModIndex = int(mi)
		runOpts.ModIndexSet = true
	}

	// Parse multi-step mods array for sequential execution.
	// For multi-step runs (mods[] in spec), each entry defines a step.
	// For single-step runs (mod or legacy top-level), Steps remains empty.
	if modsSlice, ok := opts["mods"].([]any); ok && len(modsSlice) > 0 {
		for _, modEntry := range modsSlice {
			if modMap, ok := modEntry.(map[string]any); ok {
				stepMod := parseStepMod(modMap)
				runOpts.Steps = append(runOpts.Steps, stepMod)
			}
		}
	}

	return runOpts
}

// parseStepMod extracts a StepMod from an untyped map[string]any.
// This function handles the polymorphic command representation (string or []any)
// and provides safe type conversions for multi-step mod entries.
func parseStepMod(modMap map[string]any) StepMod {
	stepMod := StepMod{
		Env: make(map[string]string),
	}

	// Extract image (required for multi-step mods).
	// Image supports both string (universal) and map (stack-specific) forms.
	if imgVal, hasImage := modMap["image"]; hasImage {
		if modImage, err := contracts.ParseModImage(imgVal); err == nil {
			stepMod.Image = modImage
		}
		// On parse error, leave Image empty; validation happens at manifest build.
	}

	// Extract command (polymorphic: string or []any).
	switch cmd := modMap["command"].(type) {
	case string:
		stepMod.Command.Shell = cmd
	case []any:
		for _, elem := range cmd {
			if s, ok := elem.(string); ok {
				stepMod.Command.Exec = append(stepMod.Command.Exec, s)
			}
		}
	}

	// Extract env map.
	if envMap, ok := modMap["env"].(map[string]any); ok {
		for k, v := range envMap {
			if s, ok := v.(string); ok {
				stepMod.Env[k] = s
			}
		}
	}

	// Extract retain_container.
	if retain, ok := modMap["retain_container"].(bool); ok {
		stepMod.RetainContainer = retain
	}

	return stepMod
}

// parseHealingMod extracts a HealingMod from an untyped map[string]any.
// This function handles the polymorphic command representation (string or []any)
// and provides safe type conversions with defaults.
func parseHealingMod(modMap map[string]any) HealingMod {
	mod := HealingMod{
		Env: make(map[string]string),
	}

	// Extract image (required, but we don't validate here; validation happens
	// in buildHealingManifest where context allows better error messages).
	// Image supports both string (universal) and map (stack-specific) forms.
	if imgVal, hasImage := modMap["image"]; hasImage {
		if modImage, err := contracts.ParseModImage(imgVal); err == nil {
			mod.Image = modImage
		}
		// On parse error, leave Image empty; validation happens at manifest build.
	}

	// Extract command (polymorphic: string or []any).
	switch cmd := modMap["command"].(type) {
	case string:
		mod.Command.Shell = cmd
	case []any:
		for _, elem := range cmd {
			if s, ok := elem.(string); ok {
				mod.Command.Exec = append(mod.Command.Exec, s)
			}
		}
	}

	// Extract env map.
	if envMap, ok := modMap["env"].(map[string]any); ok {
		for k, v := range envMap {
			if s, ok := v.(string); ok {
				mod.Env[k] = s
			}
		}
	}

	// Extract retain_container.
	if retain, ok := modMap["retain_container"].(bool); ok {
		mod.RetainContainer = retain
	}

	return mod
}

// parseHealingStrategy extracts a HealingStrategy from an untyped map[string]any.
// This function handles the multi-strategy healing schema where each strategy
// has a name and its own mods[] list for parallel healing branches.
func parseHealingStrategy(stratMap map[string]any) HealingStrategy {
	strategy := HealingStrategy{}

	// Extract strategy name (optional; used for logging and metrics).
	if name, ok := stratMap["name"].(string); ok {
		strategy.Name = name
	}

	// Extract mods[] for this strategy.
	if modsSlice, ok := stratMap["mods"].([]any); ok {
		for _, modEntry := range modsSlice {
			if modMap, ok := modEntry.(map[string]any); ok {
				mod := parseHealingMod(modMap)
				strategy.Mods = append(strategy.Mods, mod)
			}
		}
	}

	return strategy
}
