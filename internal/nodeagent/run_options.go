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
// RunOptions is derived from the run spec at the nodeagent boundary (see parseSpec)
// and is passed through StartRunRequest.TypedOptions into manifest building and
// execution orchestration paths.
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

	// Steps holds the list of mod steps for multi-step runs (steps[] array).
	// For single-step runs, this slice is empty and Execution options are used.
	// For multi-step runs, this slice contains one entry per mod in steps[].
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
// When the build gate fails, the agent can execute a single healing mod per gate.
type HealingConfig struct {
	// Retries is the maximum number of healing attempts (default: 1).
	// Each retry executes a healing mod, then re-runs the gate.
	Retries int

	// Mod is the single healing mod specification for this gate.
	Mod HealingMod
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

// StepMod describes a single mod step in a multi-step run (steps[] array).
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

	// Parse healing configuration (single-mod form).
	if bg, ok := opts["build_gate"].(map[string]any); ok {
		if healingMap, ok := bg["healing"].(map[string]any); ok {
			healing := &HealingConfig{
				Retries: 1, // Default to 1 retry.
			}

			// Extract retries (handle both int and float64 from JSON unmarshaling).
			if r, ok := healingMap["retries"].(int); ok && r > 0 {
				healing.Retries = r
			} else if rf, ok := healingMap["retries"].(float64); ok && rf > 0 {
				healing.Retries = int(rf)
			}

			// Single-mod form: mod is the canonical schema.
			if modMap, ok := healingMap["mod"].(map[string]any); ok {
				healing.Mod = parseHealingModFromMap(modMap)
				runOpts.Healing = healing
			}
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

	// Parse command (polymorphic: string, []string, or []any).
	// The []any case handles JSON-unmarshaled arrays from modsSpecToOptions,
	// which converts CommandSpec.Exec to []any via commandSpecToAnyForNested.
	// Without this, single-step specs with exec-array commands (e.g., ["a","b"])
	// would drop into empty command because []any != []string.
	switch cmd := opts["command"].(type) {
	case string:
		runOpts.Execution.Command.Shell = cmd
	case []string:
		runOpts.Execution.Command.Exec = cmd
	case []any:
		// Convert []any to []string, filtering non-string elements.
		for _, elem := range cmd {
			if s, ok := elem.(string); ok {
				runOpts.Execution.Command.Exec = append(runOpts.Execution.Command.Exec, s)
			}
		}
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

	// Parse multi-step steps array for sequential execution.
	// For multi-step runs (steps[] in spec), each entry defines a step.
	// For single-step runs (mod or legacy top-level), Steps remains empty.
	if stepsSlice, ok := opts["steps"].([]any); ok && len(stepsSlice) > 0 {
		for _, stepEntry := range stepsSlice {
			if stepMap, ok := stepEntry.(map[string]any); ok {
				stepMod := parseStepMod(stepMap)
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

// parseHealingModFromMap extracts a HealingMod from an untyped map[string]any.
func parseHealingModFromMap(modMap map[string]any) HealingMod {
	return parseHealingMod(modMap)
}

// modsSpecToRunOptions converts a canonical contracts.ModsSpec directly to RunOptions.
// This is the preferred hot-path conversion that avoids the intermediate map[string]any
// bridge and its associated type conversion hazards (float64/int, []any/[]string).
//
// The function preserves all env merging semantics from the previous modsSpecToOptions +
// parseRunOptions pipeline while eliminating intermediate type conversions.
//
// ## Why Direct Conversion?
//
// The previous two-stage pipeline (modsSpecToOptions → parseRunOptions) introduced
// type hazards on the hot path:
//   - JSON numbers unmarshaled as float64 required int conversion in parseRunOptions
//   - String slices passed through []any required element-by-element conversion
//   - Map values required type assertions at every access point
//
// Direct conversion from the typed ModsSpec struct eliminates these hazards by
// reading strongly-typed fields directly (e.g., spec.BuildGate.Healing.Retries as int).
func modsSpecToRunOptions(spec *contracts.ModsSpec) RunOptions {
	if spec == nil {
		return RunOptions{}
	}

	runOpts := RunOptions{}

	// --- Build Gate Options ---
	// Extract enabled/profile from the typed BuildGate struct.
	if spec.BuildGate != nil {
		runOpts.BuildGate.Enabled = spec.BuildGate.Enabled
		runOpts.BuildGate.Profile = spec.BuildGate.Profile

		// --- Healing Configuration ---
		// Convert contracts.HealingSpec to nodeagent.HealingConfig directly,
		// avoiding the map[string]any bridge that required float64→int conversion.
		if spec.BuildGate.Healing != nil {
			healing := &HealingConfig{
				Retries: spec.BuildGate.Healing.Retries,
			}
			// Default retries to 1 if not specified (spec parser sets default of 1).
			if healing.Retries <= 0 {
				healing.Retries = 1
			}

			// Convert healing mod specification.
			if spec.BuildGate.Healing.Mod != nil {
				healing.Mod = healingModSpecToHealingMod(spec.BuildGate.Healing.Mod)
			}

			runOpts.Healing = healing
		}
	}

	// --- MR Wiring Options ---
	// Direct field assignment from typed spec fields.
	runOpts.MRWiring.GitLabPAT = spec.GitLabPAT
	runOpts.MRWiring.GitLabDomain = spec.GitLabDomain
	if spec.MROnSuccess != nil {
		runOpts.MRWiring.MROnSuccess = *spec.MROnSuccess
		runOpts.MRFlagsPresent.MROnSuccessSet = true
	}
	if spec.MROnFail != nil {
		runOpts.MRWiring.MROnFail = *spec.MROnFail
		runOpts.MRFlagsPresent.MROnFailSet = true
	}

	// --- Execution Options (Single-Step) ---
	// For single-step specs, extract image/command/retain_container from steps[0].
	// Multi-step specs populate Steps[] instead.
	if len(spec.Steps) == 1 {
		step := spec.Steps[0]
		runOpts.Execution.Image = step.Image
		runOpts.Execution.Command = commandSpecToExecutionCommand(step.Command)
		runOpts.Execution.RetainContainer = step.RetainContainer
	}

	// --- Multi-Step Steps Array ---
	// For multi-step specs (len > 1), populate Steps[] with typed StepMod entries.
	// Single-step specs use Execution options instead (Steps remains empty).
	if len(spec.Steps) > 1 {
		runOpts.Steps = make([]StepMod, 0, len(spec.Steps))
		for _, step := range spec.Steps {
			stepMod := StepMod{
				Image:           step.Image,
				Command:         commandSpecToExecutionCommand(step.Command),
				Env:             copyStringMap(step.Env),
				RetainContainer: step.RetainContainer,
			}
			runOpts.Steps = append(runOpts.Steps, stepMod)
		}
	}

	// --- Artifact Options ---
	// Direct slice copy without []any→[]string conversion.
	runOpts.Artifacts.Name = spec.ArtifactName
	if len(spec.ArtifactPaths) > 0 {
		runOpts.Artifacts.Paths = make([]string, 0, len(spec.ArtifactPaths))
		for _, p := range spec.ArtifactPaths {
			if trimmed := strings.TrimSpace(p); trimmed != "" {
				runOpts.Artifacts.Paths = append(runOpts.Artifacts.Paths, p)
			}
		}
	}

	// --- Server Metadata ---
	// Direct assignment from typed spec fields.
	if trimmed := strings.TrimSpace(spec.JobID); trimmed != "" {
		runOpts.ServerMetadata.JobID = domaintypes.JobID(trimmed)
	}

	// --- Mod Index ---
	// Direct int assignment without float64→int conversion.
	if spec.ModIndex != nil {
		runOpts.ModIndex = *spec.ModIndex
		runOpts.ModIndexSet = true
	}

	return runOpts
}

// healingModSpecToHealingMod converts contracts.HealingModSpec to nodeagent.HealingMod.
// This direct conversion avoids the map[string]any bridge for healing mod parsing.
func healingModSpecToHealingMod(spec *contracts.HealingModSpec) HealingMod {
	if spec == nil {
		return HealingMod{}
	}

	return HealingMod{
		Image:           spec.Image,
		Command:         commandSpecToHealingCommand(spec.Command),
		Env:             copyStringMap(spec.Env),
		RetainContainer: spec.RetainContainer,
	}
}

// commandSpecToExecutionCommand converts contracts.CommandSpec to ExecutionCommand.
// Direct field mapping without polymorphic type switching.
func commandSpecToExecutionCommand(cmd contracts.CommandSpec) ExecutionCommand {
	return ExecutionCommand{
		Shell: cmd.Shell,
		Exec:  cmd.Exec,
	}
}

// commandSpecToHealingCommand converts contracts.CommandSpec to HealingCommand.
// Direct field mapping without polymorphic type switching.
func commandSpecToHealingCommand(cmd contracts.CommandSpec) HealingCommand {
	return HealingCommand{
		Shell: cmd.Shell,
		Exec:  cmd.Exec,
	}
}

// copyStringMap creates a shallow copy of a string map.
// Returns nil if the input is nil or empty.
func copyStringMap(m map[string]string) map[string]string {
	if len(m) == 0 {
		return nil
	}
	out := make(map[string]string, len(m))
	for k, v := range m {
		out[k] = v
	}
	return out
}
