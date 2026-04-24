// mig_run_spec.go separates spec file handling from mig run execution.
//
// This file contains buildSpecPayload which parses YAML/JSON spec files
// and compiles Hydra file-record entries (ca/in/out/home) into canonical
// shortHash:dst form. Specs use a single canonical shape:
//   - steps[] array with one entry per step (even single-step runs)
//   - global build gate policy under build_gate
//
// Spec parsing includes validation and error handling for missing files.
// Isolating spec handling from execution flow enables focused testing
// of file I/O and parsing logic without coupling to HTTP submission.
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	cliconfig "github.com/iw2rmb/ploy/internal/cli/config"
	"github.com/iw2rmb/ploy/internal/workflow/contracts"
	"gopkg.in/yaml.v3"
)

var specEnvPlaceholderRE = regexp.MustCompile(`\$\{([A-Za-z_][A-Za-z0-9_]*)\}|\$([A-Za-z_][A-Za-z0-9_]*)`)

func normalizeMigsSpecToJSON(ctx context.Context, base *url.URL, client *http.Client, data []byte, specBaseDir string) (json.RawMessage, error) {
	raw, err := parseSpecInputToMap(data, specBaseDir)
	if err != nil {
		return nil, fmt.Errorf("parse spec (not valid JSON or YAML): %w", err)
	}
	if err := preprocessMigsSpecInPlace(raw, specBaseDir); err != nil {
		return nil, err
	}

	// Apply local config.yaml overlay before Hydra compilation so that
	// overlay file paths are also compiled to canonical form.
	if err := applyConfigOverlayInPlace(raw); err != nil {
		return nil, err
	}
	if err := normalizeStepInEntriesInPlace(raw); err != nil {
		return nil, err
	}

	if err := compileHookSourcesInPlace(ctx, base, client, raw, specBaseDir); err != nil {
		return nil, err
	}

	if err := compileHydraRecordsInPlace(ctx, base, client, raw, specBaseDir); err != nil {
		return nil, err
	}

	jsonBytes, err := json.Marshal(raw)
	if err != nil {
		return nil, fmt.Errorf("marshal spec to JSON: %w", err)
	}

	if _, err := contracts.ParseMigSpecJSON(jsonBytes); err != nil {
		return nil, fmt.Errorf("validate spec: %w", err)
	}

	return jsonBytes, nil
}

func preprocessMigsSpecInPlace(spec map[string]any, specBaseDir string) error {
	if err := resolveImageEnvInPlace(spec); err != nil {
		return fmt.Errorf("resolve image env placeholders: %w", err)
	}

	// Expand $VAR/${VAR} placeholders in envs values at all levels.
	if err := resolveEnvsInPlace(spec); err != nil {
		return fmt.Errorf("resolve envs (top-level): %w", err)
	}

	if steps, ok := spec["steps"].([]any); ok {
		for i, s := range steps {
			if stepEntry, ok := s.(map[string]any); ok {
				if err := resolveEnvsInPlace(stepEntry); err != nil {
					return fmt.Errorf("resolve envs (steps[%d]): %w", i, err)
				}
			}
		}
	}

	return nil
}

// normalizeStepInEntriesInPlace rewrites mixed steps[].in authoring entries:
// - string entries remain in steps[].in (Hydra authoring/canonical forms)
// - object entries {from,to} move to steps[].in_from with canonical /in target
func normalizeStepInEntriesInPlace(spec map[string]any) error {
	steps, ok := spec["steps"].([]any)
	if !ok || len(steps) == 0 {
		return nil
	}

	for i := range steps {
		step, ok := steps[i].(map[string]any)
		if !ok {
			continue
		}
		inRaw, exists := step["in"]
		if !exists {
			continue
		}
		inEntries, ok := inRaw.([]any)
		if !ok {
			return fmt.Errorf("steps[%d].in: expected array, got %T", i, inRaw)
		}

		inStrings := make([]any, 0, len(inEntries))
		inFromObjects := make([]any, 0, len(inEntries))

		for j := range inEntries {
			switch v := inEntries[j].(type) {
			case string:
				inStrings = append(inStrings, v)
			case map[string]any:
				from, ok := v["from"].(string)
				if !ok || strings.TrimSpace(from) == "" {
					return fmt.Errorf("steps[%d].in[%d].from: required string", i, j)
				}
				parsed, err := contracts.ParseInFromURI(from)
				if err != nil {
					return fmt.Errorf("steps[%d].in[%d].from: %w", i, j, err)
				}
				to := ""
				if rawTo, hasTo := v["to"]; hasTo {
					s, ok := rawTo.(string)
					if !ok {
						return fmt.Errorf("steps[%d].in[%d].to: expected string, got %T", i, j, rawTo)
					}
					to = s
				}
				target, err := contracts.NormalizeInFromTarget(to, parsed.OutPath)
				if err != nil {
					return fmt.Errorf("steps[%d].in[%d].to: %w", i, j, err)
				}
				inFromObjects = append(inFromObjects, map[string]any{
					"from": strings.TrimSpace(from),
					"to":   target,
				})
			default:
				return fmt.Errorf("steps[%d].in[%d]: expected string or object, got %T", i, j, inEntries[j])
			}
		}

		step["in"] = inStrings
		if len(inFromObjects) == 0 {
			continue
		}

		existingRaw, hasExisting := step["in_from"]
		if !hasExisting {
			step["in_from"] = inFromObjects
			continue
		}
		existingEntries, ok := existingRaw.([]any)
		if !ok {
			return fmt.Errorf("steps[%d].in_from: expected array, got %T", i, existingRaw)
		}
		combined := make([]any, 0, len(existingEntries)+len(inFromObjects))
		combined = append(combined, existingEntries...)
		combined = append(combined, inFromObjects...)
		step["in_from"] = combined
	}

	return nil
}

// resolveEnvsInPlace expands $VAR and ${VAR} placeholders in envs string values.
func resolveEnvsInPlace(spec map[string]any) error {
	envsRaw, ok := spec["envs"]
	if !ok {
		return nil
	}
	switch envs := envsRaw.(type) {
	case map[string]any:
		for k, v := range envs {
			s, ok := v.(string)
			if !ok {
				return fmt.Errorf("envs[%s]: expected string, got %T", k, v)
			}
			expanded, err := expandSpecEnvValue(s)
			if err != nil {
				return fmt.Errorf("envs[%s]: %w", k, err)
			}
			envs[k] = expanded
		}
	case map[string]string:
		expanded := make(map[string]any, len(envs))
		for k, v := range envs {
			exp, err := expandSpecEnvValue(v)
			if err != nil {
				return fmt.Errorf("envs[%s]: %w", k, err)
			}
			expanded[k] = exp
		}
		spec["envs"] = expanded
	}
	return nil
}

func resolveImageEnvInPlace(spec map[string]any) error {
	if steps, ok := spec["steps"].([]any); ok {
		for i, raw := range steps {
			step, ok := raw.(map[string]any)
			if !ok {
				continue
			}
			if err := resolveImageInSection(step, fmt.Sprintf("steps[%d]", i)); err != nil {
				return err
			}
		}
	}

	return nil
}

func resolveImageInSection(section map[string]any, prefix string) error {
	raw, exists := section["image"]
	if !exists {
		return nil
	}

	switch image := raw.(type) {
	case string:
		expanded, err := expandSpecEnvValue(image)
		if err != nil {
			return fmt.Errorf("%s.image: %w", prefix, err)
		}
		section["image"] = expanded
	case map[string]any:
		for stack, rawValue := range image {
			value, ok := rawValue.(string)
			if !ok {
				continue
			}
			expanded, err := expandSpecEnvValue(value)
			if err != nil {
				return fmt.Errorf("%s.image[%q]: %w", prefix, stack, err)
			}
			image[stack] = expanded
		}
	case map[string]string:
		for stack, value := range image {
			expanded, err := expandSpecEnvValue(value)
			if err != nil {
				return fmt.Errorf("%s.image[%q]: %w", prefix, stack, err)
			}
			image[stack] = expanded
		}
		section["image"] = image
	}

	return nil
}

func resolvePath(path string, baseDir ...string) (string, error) {
	resolvedBaseDir := ""
	if len(baseDir) > 0 {
		resolvedBaseDir = baseDir[0]
	}
	trimmed := strings.TrimSpace(path)
	if trimmed == "" {
		return "", fmt.Errorf("path is empty")
	}
	expanded := strings.TrimSpace(os.ExpandEnv(trimmed))
	if expanded == "" {
		return "", fmt.Errorf("path is empty")
	}
	if strings.HasPrefix(expanded, "~/") {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", fmt.Errorf("resolve home dir for path %s: %w", expanded, err)
		}
		return filepath.Join(home, expanded[2:]), nil
	}
	if !filepath.IsAbs(expanded) && strings.TrimSpace(resolvedBaseDir) != "" {
		return filepath.Join(resolvedBaseDir, expanded), nil
	}
	return expanded, nil
}

func parseSpecInputToMap(data []byte, specBaseDir string) (map[string]any, error) {
	var obj map[string]any
	if err := json.Unmarshal(data, &obj); err != nil {
		specRootPath, rootErr := composeSpecRootPath(specBaseDir)
		if rootErr != nil {
			return nil, rootErr
		}
		composed, composeErr := composeSpecYAML(data, specRootPath)
		if composeErr != nil {
			return nil, composeErr
		}
		if err := yaml.Unmarshal(composed, &obj); err != nil {
			return nil, fmt.Errorf("parse (not valid JSON or YAML): %w", err)
		}
	}
	if obj == nil {
		return nil, fmt.Errorf("expected object, got empty")
	}
	return obj, nil
}

func composeSpecRootPath(specBaseDir string) (string, error) {
	base := strings.TrimSpace(specBaseDir)
	if base == "" {
		wd, err := os.Getwd()
		if err != nil {
			return "", fmt.Errorf("resolve working directory for spec: %w", err)
		}
		base = wd
	}
	resolved, err := filepath.Abs(base)
	if err != nil {
		return "", fmt.Errorf("resolve absolute spec directory %s: %w", base, err)
	}
	return filepath.Join(resolved, ".root-spec.yaml"), nil
}

func expandSpecEnvValue(raw string) (string, error) {
	if !strings.Contains(raw, "$") {
		return raw, nil
	}

	missing := make(map[string]struct{})
	expanded := specEnvPlaceholderRE.ReplaceAllStringFunc(raw, func(match string) string {
		var name string
		if strings.HasPrefix(match, "${") {
			name = strings.TrimSuffix(strings.TrimPrefix(match, "${"), "}")
		} else {
			name = strings.TrimPrefix(match, "$")
		}

		if v, ok := os.LookupEnv(name); ok {
			return v
		}
		missing[name] = struct{}{}
		return ""
	})

	if len(missing) == 0 {
		return expanded, nil
	}

	names := make([]string, 0, len(missing))
	for name := range missing {
		names = append(names, name)
	}
	sort.Strings(names)

	return "", fmt.Errorf("unresolved environment variables: %s", strings.Join(names, ", "))
}

// applyConfigOverlayInPlace loads the local config.yaml overlay and merges it
// into the spec using deterministic rules. This runs after preprocessing and
// before Hydra compilation so overlay file paths also get compiled.
//
// Routing:
//   - steps[] entries receive the "mig" job section overlay
//   - top-level envs receive the "mig" section envs (primary job type)
func applyConfigOverlayInPlace(spec map[string]any) error {
	ov, err := cliconfig.LoadOverlay()
	if err != nil {
		return fmt.Errorf("config overlay: %w", err)
	}
	if ov.Defaults == nil || ov.Defaults.Job == nil {
		return nil
	}

	migCfg := ov.JobSection("mig")
	// Apply mig overlay to top-level envs.
	if migCfg != nil && len(migCfg.Envs) > 0 {
		cliconfig.MergeJobConfigIntoSpec(spec, &cliconfig.JobConfig{Envs: migCfg.Envs})
	}

	// Apply mig overlay to each step block.
	if steps, ok := spec["steps"].([]any); ok {
		for _, s := range steps {
			step, ok := s.(map[string]any)
			if !ok {
				continue
			}
			cliconfig.MergeJobConfigIntoSpec(step, migCfg)
		}
	}

	return nil
}

// buildSpecPayload loads a spec from file (YAML or JSON) and merges it with CLI flag overrides.
// CLI flags take precedence over spec file values. Returns raw JSON bytes.
//
// Processing order:
//  1. Load spec file (YAML or JSON format) if provided
//  2. Preprocess: resolve !include composition, image env, envs expansion
//  3. Compile Hydra records: ca/in/out/home authoring entries → canonical shortHash:dst form
//  4. Apply CLI flag overrides (higher precedence than spec file) to top-level fields
//  5. Apply defaults (e.g., gitlab_domain when gitlab_pat is set)
//
// Returns nil payload when neither spec file nor CLI overrides are provided.
//
// Multi-step semantics (steps[] array):
//   - Each entry in steps[] represents a sequential transformation step.
//   - All steps share the same repository and global build_gate policy.
//   - The CLI preserves steps[] without modification; image/command overrides do not apply when len(steps) > 1.
//   - The server copies steps[] indexes into jobs.next_id and diffs.next_id.
func buildSpecPayload(
	ctx context.Context,
	base *url.URL,
	client *http.Client,
	specFile string,
	migEnvs []string,
	migImage string,
	retain bool,
	migCommand string,
	gitlabPAT string,
	gitlabDomain string,
	mrSuccess bool,
	mrFail bool,
) ([]byte, error) {
	_ = retain

	// Start with spec from file (if provided)
	var specMap map[string]any
	specBaseDir := ""
	if specFile != "" {
		specBaseDir = filepath.Dir(specFile)
		data, err := os.ReadFile(specFile)
		if err != nil {
			return nil, fmt.Errorf("read spec file %s: %w", specFile, err)
		}
		specMap, err = parseSpecInputToMap(data, specBaseDir)
		if err != nil {
			return nil, fmt.Errorf("parse spec file %s (not valid JSON or YAML): %w", specFile, err)
		}
	} else {
		specMap = make(map[string]any)
	}

	if err := preprocessMigsSpecInPlace(specMap, specBaseDir); err != nil {
		return nil, err
	}

	if err := applyConfigOverlayInPlace(specMap); err != nil {
		return nil, err
	}

	if err := compileHookSourcesInPlace(ctx, base, client, specMap, specBaseDir); err != nil {
		return nil, err
	}

	if err := compileHydraRecordsInPlace(ctx, base, client, specMap, specBaseDir); err != nil {
		return nil, err
	}

	// Merge CLI flag overrides (CLI flags take precedence)
	hasOverrides := len(migEnvs) > 0 || migImage != "" || migCommand != "" ||
		gitlabPAT != "" || gitlabDomain != "" || mrSuccess || mrFail

	// Only proceed if we have a spec file or CLI overrides
	if len(specMap) == 0 && !hasOverrides {
		return nil, nil
	}

	if len(migEnvs) > 0 {
		// Start from existing envs.
		current := make(map[string]any)
		if existingEnvs, ok := specMap["envs"].(map[string]any); ok {
			for k, v := range existingEnvs {
				if s, ok := v.(string); ok {
					current[k] = s
				}
			}
		}

		// Apply CLI overrides (higher precedence than spec file)
		for _, kv := range migEnvs {
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
			specMap["envs"] = current
		}
	}

	// Image/command overrides apply only to single-step specs. For multi-step
	// specs (len(steps) > 1), these overrides are ignored.
	var stepsLen int
	if steps, ok := specMap["steps"].([]any); ok {
		stepsLen = len(steps)
	}
	if stepsLen <= 1 && (migImage != "" || migCommand != "") {
		// Ensure steps[0] exists and is a map.
		var step0 map[string]any
		if stepsLen == 1 {
			if m, ok := specMap["steps"].([]any)[0].(map[string]any); ok {
				step0 = m
			}
		}
		if step0 == nil {
			step0 = make(map[string]any)
			specMap["steps"] = []any{step0}
		}

		if migImage != "" {
			step0["image"] = migImage
		}
		if migCommand != "" {
			// Allow JSON array for command to pass argv directly to containers with ENTRYPOINT.
			// Fallback to plain string when not a JSON array.
			var asArray []string
			if strings.HasPrefix(migCommand, "[") && strings.HasSuffix(migCommand, "]") {
				if err := json.Unmarshal([]byte(migCommand), &asArray); err == nil && len(asArray) > 0 {
					step0["command"] = asArray
				} else {
					step0["command"] = migCommand
				}
			} else {
				step0["command"] = migCommand
			}
		}
	}

	// Add GitLab options (never print PAT in logs; node agent will handle redaction)
	if gitlabPAT != "" {
		specMap["gitlab_pat"] = gitlabPAT
	}
	if gitlabDomain != "" {
		specMap["gitlab_domain"] = gitlabDomain
	}
	if mrSuccess {
		specMap["mr_on_success"] = true
	}
	if mrFail {
		specMap["mr_on_fail"] = true
	}

	// Default gitlab_domain to "gitlab.com" when gitlab_pat is provided but gitlab_domain is empty.
	// This runs after all CLI overrides to check the final state.
	if _, hasPAT := specMap["gitlab_pat"]; hasPAT {
		if _, hasDomain := specMap["gitlab_domain"]; !hasDomain {
			specMap["gitlab_domain"] = "gitlab.com"
		}
	}

	if len(specMap) == 0 {
		return nil, nil
	}

	// Marshal to JSON for submission
	jsonBytes, err := json.Marshal(specMap)
	if err != nil {
		return nil, fmt.Errorf("marshal spec: %w", err)
	}

	// Validate spec using the canonical parser to catch structural issues early.
	// This ensures the CLI surfaces validation errors before submission.
	if _, err := contracts.ParseMigSpecJSON(jsonBytes); err != nil {
		return nil, fmt.Errorf("validate spec: %w", err)
	}

	return jsonBytes, nil
}
