// mod_run_spec.go separates spec file handling from mig run execution.
//
// This file contains buildSpecPayload which parses YAML/JSON spec files
// and resolves env_from_file references to inject file content as environment
// variables. Specs use a single canonical shape:
//   - steps[] array with one entry per step (even single-step runs)
//   - global build gate policy under build_gate (including build_gate.healing.by_error_kind)
//
// Spec parsing includes validation and error handling for missing files.
// Isolating spec handling from execution flow enables focused testing
// of file I/O and parsing logic without coupling to HTTP submission.
package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/iw2rmb/ploy/internal/workflow/contracts"
	"gopkg.in/yaml.v3"
)

func normalizeModsSpecToJSON(data []byte) (json.RawMessage, error) {
	var raw map[string]any
	if err := json.Unmarshal(data, &raw); err != nil {
		if err := yaml.Unmarshal(data, &raw); err != nil {
			return nil, fmt.Errorf("parse spec (not valid JSON or YAML): %w", err)
		}
	}

	jsonBytes, err := json.Marshal(raw)
	if err != nil {
		return nil, fmt.Errorf("marshal spec to JSON: %w", err)
	}

	if _, err := contracts.ParseModsSpecJSON(jsonBytes); err != nil {
		return nil, fmt.Errorf("validate spec: %w", err)
	}

	return jsonBytes, nil
}

// resolveEnvFromFile reads a file path (expanding ~) and returns its content as a string.
// File content is treated as sensitive, so any errors redact the file path for security.
func resolveEnvFromFile(path string) (string, error) {
	// Expand ~ to user home directory
	if strings.HasPrefix(path, "~/") {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", fmt.Errorf("resolve home dir for path %s: %w", path, err)
		}
		path = filepath.Join(home, path[2:])
	}
	data, err := os.ReadFile(path)
	if err != nil {
		// Redact file path to avoid leaking sensitive locations in error messages
		return "", fmt.Errorf("read env file (path redacted): %w", err)
	}
	return string(data), nil
}

// resolveEnvFromFileInPlace processes env and env_from_file from a spec section,
// resolving file references and merging them into the env map in-place.
// Removes env_from_file after resolution.
//
// Supports two syntaxes:
// 1. env_from_file as a sibling map: {"env_from_file": {"KEY": "/path/to/file"}}
// 2. Inline syntax within env: {"env": {"KEY": {"from_file": "/path/to/file"}}}
//
// Values from env take precedence over env_from_file.
func resolveEnvFromFileInPlace(spec map[string]any) error {
	// Prepare the merged env map
	mergedEnv := make(map[string]any)

	// First, process env_from_file if present (sibling syntax)
	switch envFromFile := spec["env_from_file"].(type) {
	case map[string]any:
		for k, v := range envFromFile {
			path, ok := v.(string)
			if !ok {
				return fmt.Errorf("env_from_file[%s]: expected string path, got %T", k, v)
			}
			content, err := resolveEnvFromFile(path)
			if err != nil {
				return fmt.Errorf("env_from_file[%s]: %w", k, err)
			}
			mergedEnv[k] = content
		}
		// Remove env_from_file after processing to keep spec clean
		delete(spec, "env_from_file")
	case map[string]string:
		for k, path := range envFromFile {
			content, err := resolveEnvFromFile(path)
			if err != nil {
				return fmt.Errorf("env_from_file[%s]: %w", k, err)
			}
			mergedEnv[k] = content
		}
		delete(spec, "env_from_file")
	}

	// Then, process env (including inline {from_file: path} syntax)
	switch env := spec["env"].(type) {
	case map[string]any:
		for k, v := range env {
			switch val := v.(type) {
			case string:
				// Direct string value (overwrites env_from_file if present)
				mergedEnv[k] = val
			case map[string]any:
				// Check for {from_file: "path"} syntax
				if fromFile, ok := val["from_file"].(string); ok {
					content, err := resolveEnvFromFile(fromFile)
					if err != nil {
						return fmt.Errorf("env[%s].from_file: %w", k, err)
					}
					mergedEnv[k] = content
				} else {
					return fmt.Errorf("env[%s]: expected string or {from_file: path}, got unsupported map structure", k)
				}
			case map[string]string:
				if fromFile, ok := val["from_file"]; ok {
					content, err := resolveEnvFromFile(fromFile)
					if err != nil {
						return fmt.Errorf("env[%s].from_file: %w", k, err)
					}
					mergedEnv[k] = content
				} else {
					return fmt.Errorf("env[%s]: expected string or {from_file: path}, got unsupported map structure", k)
				}
			default:
				return fmt.Errorf("env[%s]: expected string or {from_file: path}, got %T", k, v)
			}
		}
	case map[string]string:
		for k, v := range env {
			mergedEnv[k] = v
		}
	}

	// Update spec with merged env (only if we have any env values)
	if len(mergedEnv) > 0 {
		spec["env"] = mergedEnv
	}

	return nil
}

// buildSpecPayload loads a spec from file (YAML or JSON) and merges it with CLI flag overrides.
// CLI flags take precedence over spec file values. Returns raw JSON bytes.
//
// Processing order:
// 1. Load spec file (YAML or JSON format) if provided
// 2. Resolve env_from_file references in:
//   - top-level env
//   - steps[] entries
//   - build_gate.healing.by_error_kind.* (healing actions)
//   - build_gate.router (router)
//
// 3. Apply CLI flag overrides (higher precedence than spec file) to top-level fields.
//
// 4. Apply defaults (e.g., gitlab_domain when gitlab_pat is set)
//
// Returns nil payload when neither spec file nor CLI overrides are provided.
//
// Multi-step semantics (steps[] array):
// - Each entry in steps[] represents a sequential transformation step.
// - All steps share the same repository and global build_gate policy (including healing).
// - The CLI preserves steps[] without modification; image/command/retain overrides do not apply when len(steps) > 1.
// - The server copies steps[] indexes into jobs.next_id and diffs.next_id.
func buildSpecPayload(
	specFile string,
	modEnvs []string,
	modImage string,
	retain bool,
	modCommand string,
	gitlabPAT string,
	gitlabDomain string,
	mrSuccess bool,
	mrFail bool,
) ([]byte, error) {
	_ = retain

	// Start with spec from file (if provided)
	var base map[string]any
	if specFile != "" {
		data, err := os.ReadFile(specFile)
		if err != nil {
			return nil, fmt.Errorf("read spec file %s: %w", specFile, err)
		}
		// Try JSON first, fallback to YAML
		if err := json.Unmarshal(data, &base); err != nil {
			// Not JSON; try YAML
			if err := yaml.Unmarshal(data, &base); err != nil {
				return nil, fmt.Errorf("parse spec file %s (not valid JSON or YAML): %w", specFile, err)
			}
		}
	} else {
		base = make(map[string]any)
	}

	// Resolve env_from_file references in the canonical top-level env block.
	if err := resolveEnvFromFileInPlace(base); err != nil {
		return nil, fmt.Errorf("resolve env from file (top-level): %w", err)
	}

	// Resolve env_from_file references in build_gate.healing.by_error_kind.* and build_gate.router.
	if bg, ok := base["build_gate"].(map[string]any); ok {
		if healing, ok := bg["healing"].(map[string]any); ok {
			if byErrorKind, ok := healing["by_error_kind"].(map[string]any); ok {
				for errorKind, item := range byErrorKind {
					action, ok := item.(map[string]any)
					if !ok {
						continue
					}
					if err := resolveEnvFromFileInPlace(action); err != nil {
						return nil, fmt.Errorf("resolve env from file (build_gate.healing.by_error_kind.%s): %w", errorKind, err)
					}
				}
			}
		}
		if router, ok := bg["router"].(map[string]any); ok {
			if err := resolveEnvFromFileInPlace(router); err != nil {
				return nil, fmt.Errorf("resolve env from file (build_gate.router): %w", err)
			}
		}
	}

	// Resolve env_from_file references in steps[] array entries.
	if steps, ok := base["steps"].([]any); ok {
		for i, s := range steps {
			if stepEntry, ok := s.(map[string]any); ok {
				if err := resolveEnvFromFileInPlace(stepEntry); err != nil {
					return nil, fmt.Errorf("resolve env from file (steps[%d]): %w", i, err)
				}
			}
		}
	}

	// Merge CLI flag overrides (CLI flags take precedence)
	hasOverrides := len(modEnvs) > 0 || modImage != "" || modCommand != "" ||
		gitlabPAT != "" || gitlabDomain != "" || mrSuccess || mrFail

	// Only proceed if we have a spec file or CLI overrides
	if len(base) == 0 && !hasOverrides {
		return nil, nil
	}

	if len(modEnvs) > 0 {
		// Start from existing env.
		current := make(map[string]any)
		if existingEnv, ok := base["env"].(map[string]any); ok {
			for k, v := range existingEnv {
				if s, ok := v.(string); ok {
					current[k] = s
				}
			}
		}

		// Apply CLI overrides (higher precedence than spec file)
		for _, kv := range modEnvs {
			kv = strings.TrimSpace(kv)
			if kv == "" {
				continue
			}
			var k, v string
			if idx := strings.IndexByte(kv, '='); idx >= 0 {
				k = strings.TrimSpace(kv[:idx])
				v = kv[idx+1:]
			} else {
				k = kv
				v = ""
			}
			if k != "" {
				current[k] = v
			}
		}
		if len(current) > 0 {
			base["env"] = current
		}
	}

	// Image/command overrides apply only to single-step specs. For multi-step
	// specs (len(steps) > 1), these overrides are ignored.
	var stepsLen int
	if steps, ok := base["steps"].([]any); ok {
		stepsLen = len(steps)
	}
	if stepsLen <= 1 && (modImage != "" || modCommand != "") {
		// Ensure steps[0] exists and is a map.
		var step0 map[string]any
		if stepsLen == 1 {
			if m, ok := base["steps"].([]any)[0].(map[string]any); ok {
				step0 = m
			}
		}
		if step0 == nil {
			step0 = make(map[string]any)
			base["steps"] = []any{step0}
		}

		if modImage != "" {
			step0["image"] = modImage
		}
		if modCommand != "" {
			// Allow JSON array for command to pass argv directly to containers with ENTRYPOINT.
			// Fallback to plain string when not a JSON array.
			var asArray []string
			if strings.HasPrefix(modCommand, "[") && strings.HasSuffix(modCommand, "]") {
				if err := json.Unmarshal([]byte(modCommand), &asArray); err == nil && len(asArray) > 0 {
					step0["command"] = asArray
				} else {
					step0["command"] = modCommand
				}
			} else {
				step0["command"] = modCommand
			}
		}
	}

	// Add GitLab options (never print PAT in logs; node agent will handle redaction)
	if gitlabPAT != "" {
		base["gitlab_pat"] = gitlabPAT
	}
	if gitlabDomain != "" {
		base["gitlab_domain"] = gitlabDomain
	}
	if mrSuccess {
		base["mr_on_success"] = true
	}
	if mrFail {
		base["mr_on_fail"] = true
	}

	// Default gitlab_domain to "gitlab.com" when gitlab_pat is provided but gitlab_domain is empty.
	// This runs after all CLI overrides to check the final state.
	if _, hasPAT := base["gitlab_pat"]; hasPAT {
		if _, hasDomain := base["gitlab_domain"]; !hasDomain {
			base["gitlab_domain"] = "gitlab.com"
		}
	}

	if len(base) == 0 {
		return nil, nil
	}

	// Marshal to JSON for submission
	jsonBytes, err := json.Marshal(base)
	if err != nil {
		return nil, fmt.Errorf("marshal spec: %w", err)
	}

	// Validate spec using the canonical parser to catch structural issues early.
	// This ensures the CLI surfaces validation errors before submission.
	if _, err := contracts.ParseModsSpecJSON(jsonBytes); err != nil {
		return nil, fmt.Errorf("validate spec: %w", err)
	}

	return jsonBytes, nil
}
