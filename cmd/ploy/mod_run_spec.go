package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

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
	if envFromFile, ok := spec["env_from_file"].(map[string]any); ok {
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
	}

	// Then, process env (including inline {from_file: path} syntax)
	if env, ok := spec["env"].(map[string]any); ok {
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
			default:
				return fmt.Errorf("env[%s]: expected string or {from_file: path}, got %T", k, v)
			}
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
// 2. Resolve env_from_file references in mod, build_gate_healing.mods[], and top-level
// 3. Apply CLI flag overrides (higher precedence than spec file)
// 4. Apply defaults (e.g., gitlab_domain when gitlab_pat is set)
//
// Returns nil payload when neither spec file nor CLI overrides are provided.
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
	healOnBuild bool,
) ([]byte, error) {
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

	// Resolve env_from_file references:
	// 1) In the nested mod section when present (canonical schema)
	if mod, ok := base["mod"].(map[string]any); ok {
		if err := resolveEnvFromFileInPlace(mod); err != nil {
			return nil, fmt.Errorf("resolve env from file (mod): %w", err)
		}
	} else {
		// 2) Back-compat: resolve in top-level when users omit mod block
		if err := resolveEnvFromFileInPlace(base); err != nil {
			return nil, fmt.Errorf("resolve env from file (top-level): %w", err)
		}
	}

	// Resolve env_from_file references in build_gate_healing.mods[] if present
	if healing, ok := base["build_gate_healing"].(map[string]any); ok {
		if mods, ok := healing["mods"].([]any); ok {
			for i, m := range mods {
				if modEntry, ok := m.(map[string]any); ok {
					if err := resolveEnvFromFileInPlace(modEntry); err != nil {
						return nil, fmt.Errorf("resolve env from file (build_gate_healing.mods[%d]): %w", i, err)
					}
				}
			}
		}
	}

	// Merge CLI flag overrides (CLI flags take precedence)
	hasOverrides := len(modEnvs) > 0 || modImage != "" || retain || modCommand != "" ||
		gitlabPAT != "" || gitlabDomain != "" || mrSuccess || mrFail || healOnBuild

	// Only proceed if we have a spec file or CLI overrides
	if len(base) == 0 && !hasOverrides {
		return nil, nil
	}

	// Apply CLI overrides to the base spec
	if len(modEnvs) > 0 {
		env := make(map[string]string)
		// Preserve existing env from spec file if present
		if existingEnv, ok := base["env"].(map[string]any); ok {
			for k, v := range existingEnv {
				if s, ok := v.(string); ok {
					env[k] = s
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
				env[k] = v
			}
		}
		if len(env) > 0 {
			base["env"] = env
		}
	}

	if modImage != "" {
		base["image"] = modImage
	}

	if retain {
		base["retain_container"] = true
	}

	if modCommand != "" {
		// Allow JSON array for command to pass argv directly to containers with ENTRYPOINT.
		// Fallback to shell string (wrapped as ["/bin/sh","-c",cmd]) when not a JSON array.
		var asArray []string
		if strings.HasPrefix(modCommand, "[") && strings.HasSuffix(modCommand, "]") {
			if err := json.Unmarshal([]byte(modCommand), &asArray); err == nil && len(asArray) > 0 {
				base["command"] = asArray
			} else {
				base["command"] = modCommand
			}
		} else {
			base["command"] = modCommand
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

	// DEPRECATED: --heal-on-build injects a default build_gate_healing when spec lacks it.
	// This is a back-compat shim kept for one release cycle.
	if healOnBuild {
		if _, exists := base["build_gate_healing"]; !exists {
			base["build_gate_healing"] = map[string]any{
				"retries": 1,
				"mods":    []any{},
			}
		}
	}

	if len(base) == 0 {
		return nil, nil
	}

	// Marshal to JSON for submission
	return json.Marshal(base)
}
