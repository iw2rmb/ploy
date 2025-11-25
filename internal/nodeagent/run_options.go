// run_options.go defines typed option structs for nodeagent execution.
//
// This file introduces small, focused option structs that clarify which spec/options
// keys are understood by the nodeagent. These types replace untyped map[string]any
// lookups throughout the package while preserving raw JSON where needed for
// wire-level compatibility. The typed options align with the roadmap goal of
// reducing map[string]any casts and improving internal type safety.
package nodeagent

// RunOptions holds all typed configuration options for a run execution.
// It aggregates build gate configuration, healing policy, MR creation wiring,
// execution shaping parameters, and artifact collection settings.
//
// RunOptions is populated from StartRunRequest.Options by parseRunOptions and
// consumed by buildManifestFromRequest and execution orchestration phases.
type RunOptions struct {
	// BuildGate configures pre-mod build gate validation.
	BuildGate BuildGateOptions

	// Healing configures the heal → re-gate loop when build gate fails.
	Healing *HealingConfig

	// MRWiring configures GitLab merge request creation.
	MRWiring MRWiringOptions

	// Execution configures container image, command, and retention.
	Execution ExecutionOptions

	// Artifacts configures artifact collection and upload.
	Artifacts ArtifactOptions

	// ServerMetadata holds server-injected metadata for uploads and tracking.
	ServerMetadata ServerMetadataOptions

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
// to fix the workspace, then re-run the gate. This struct specifies the
// retry limit and the healing mods to execute.
type HealingConfig struct {
	// Retries is the maximum number of healing attempts (default: 1).
	// Each retry executes all healing mods in sequence, then re-runs the gate.
	Retries int

	// Mods is the list of healing mod specifications to execute on gate failure.
	// Each mod runs with /workspace (RW), /out (RW), and /in (RO) mounts.
	Mods []HealingMod
}

// HealingMod describes a single healing mod container specification.
// Healing mods run after gate failure to attempt workspace fixes before re-gate.
type HealingMod struct {
	// Image is the container image for the healing mod (required).
	Image string

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
	Image string

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
	// If empty, the server generates a default name based on run_id and stage_id.
	Name string
}

// ServerMetadataOptions holds server-injected metadata for uploads and tracking.
// These options are populated by the control plane and used by the nodeagent
// for status reporting, artifact uploads, and run correlation.
type ServerMetadataOptions struct {
	// StageID is the server-provided stage identifier for upload correlation.
	// This value is used to associate artifacts and status updates with a stage.
	StageID string
}

// StepMod describes a single mod step in a multi-step run (mods[] array).
// Each step has its own image, command, and environment configuration.
// Steps execute sequentially with shared workspace, each running gate+mod.
type StepMod struct {
	// Image is the container image for this step (required).
	Image string

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
// The raw map is still preserved in StartRunRequest.Options for backward
// compatibility, but typed accessors should be preferred for new code.
func parseRunOptions(opts map[string]any) RunOptions {
	runOpts := RunOptions{}

	// Parse build gate options (flattened by parseSpec).
	if enabled, ok := opts["build_gate_enabled"].(bool); ok {
		runOpts.BuildGate.Enabled = enabled
	}
	if profile, ok := opts["build_gate_profile"].(string); ok {
		runOpts.BuildGate.Profile = profile
	}

	// Parse healing configuration.
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

		// Extract healing mods.
		if modsSlice, ok := healingMap["mods"].([]any); ok {
			for _, modEntry := range modsSlice {
				if modMap, ok := modEntry.(map[string]any); ok {
					mod := parseHealingMod(modMap)
					healing.Mods = append(healing.Mods, mod)
				}
			}
		}

		runOpts.Healing = healing
	}

	// Parse MR wiring options.
	if pat, ok := opts["gitlab_pat"].(string); ok {
		runOpts.MRWiring.GitLabPAT = pat
	}
	if domain, ok := opts["gitlab_domain"].(string); ok {
		runOpts.MRWiring.GitLabDomain = domain
	}
	if mrSuccess, ok := opts["mr_on_success"].(bool); ok {
		runOpts.MRWiring.MROnSuccess = mrSuccess
	}
	if mrFail, ok := opts["mr_on_fail"].(bool); ok {
		runOpts.MRWiring.MROnFail = mrFail
	}

	// Parse execution options.
	if image, ok := opts["image"].(string); ok {
		runOpts.Execution.Image = image
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
	// artifact_paths is handled separately in uploadConfiguredArtifacts
	// due to its []any representation; we don't parse it here to avoid
	// duplicating the conversion logic.

	// Parse server metadata.
	if stageID, ok := opts["stage_id"].(string); ok {
		runOpts.ServerMetadata.StageID = stageID
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
	if image, ok := modMap["image"].(string); ok {
		stepMod.Image = image
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
	if image, ok := modMap["image"].(string); ok {
		mod.Image = image
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
