// mig_run_spec.go separates spec file handling from mig run execution.
//
// This file contains buildSpecPayload which parses YAML/JSON spec files
// and compiles Hydra file-record entries (ca/in/out/home) into canonical
// shortHash:dst form. Specs use a single canonical shape:
//   - steps[] array with one entry per step (even single-step runs)
//   - global build gate policy under build_gate (including build_gate.healing.by_error_kind)
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

	"github.com/iw2rmb/ploy/internal/workflow/contracts"
	"gopkg.in/yaml.v3"
)

var specEnvPlaceholderRE = regexp.MustCompile(`\$\{([A-Za-z_][A-Za-z0-9_]*)\}|\$([A-Za-z_][A-Za-z0-9_]*)`)

func normalizeMigsSpecToJSON(ctx context.Context, base *url.URL, client *http.Client, data []byte) (json.RawMessage, error) {
	var raw map[string]any
	if err := json.Unmarshal(data, &raw); err != nil {
		if err := yaml.Unmarshal(data, &raw); err != nil {
			return nil, fmt.Errorf("parse spec (not valid JSON or YAML): %w", err)
		}
	}
	if err := preprocessMigsSpecInPlace(raw, ""); err != nil {
		return nil, err
	}

	if err := compileHydraRecordsInPlace(ctx, base, client, raw, ""); err != nil {
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
	if err := resolveBuildGateSpecPathInPlace(spec, specBaseDir); err != nil {
		return fmt.Errorf("resolve spec_path (build_gate): %w", err)
	}
	if err := resolveAmataSpecPathInPlace(spec, specBaseDir); err != nil {
		return fmt.Errorf("resolve amata.spec path: %w", err)
	}
	if err := resolveImageEnvInPlace(spec); err != nil {
		return fmt.Errorf("resolve image env placeholders: %w", err)
	}

	// Expand $VAR/${VAR} placeholders in envs values at all levels.
	if err := resolveEnvsInPlace(spec); err != nil {
		return fmt.Errorf("resolve envs (top-level): %w", err)
	}

	if bg, ok := spec["build_gate"].(map[string]any); ok {
		if healing, ok := bg["healing"].(map[string]any); ok {
			if byErrorKind, ok := healing["by_error_kind"].(map[string]any); ok {
				for errorKind, item := range byErrorKind {
					action, ok := item.(map[string]any)
					if !ok {
						continue
					}
					if err := resolveEnvsInPlace(action); err != nil {
						return fmt.Errorf("resolve envs (build_gate.healing.by_error_kind.%s): %w", errorKind, err)
					}
				}
			}
		}
		if router, ok := bg["router"].(map[string]any); ok {
			if err := resolveEnvsInPlace(router); err != nil {
				return fmt.Errorf("resolve envs (build_gate.router): %w", err)
			}
		}
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

	bg, ok := spec["build_gate"].(map[string]any)
	if !ok {
		return nil
	}

	if router, ok := bg["router"].(map[string]any); ok {
		if err := resolveImageInSection(router, "build_gate.router"); err != nil {
			return err
		}
	}

	if healing, ok := bg["healing"].(map[string]any); ok {
		if byErrorKind, ok := healing["by_error_kind"].(map[string]any); ok {
			for errorKind, raw := range byErrorKind {
				action, ok := raw.(map[string]any)
				if !ok {
					continue
				}
				if err := resolveImageInSection(action, fmt.Sprintf("build_gate.healing.by_error_kind.%s", errorKind)); err != nil {
					return err
				}
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

func parseSpecObject(data []byte) (map[string]any, error) {
	var obj map[string]any
	if err := json.Unmarshal(data, &obj); err != nil {
		if err := yaml.Unmarshal(data, &obj); err != nil {
			return nil, fmt.Errorf("parse (not valid JSON or YAML): %w", err)
		}
	}
	if obj == nil {
		return nil, fmt.Errorf("expected object, got empty")
	}
	return obj, nil
}

func readSpecObjectFromPath(path string, specBaseDir string) (map[string]any, error) {
	resolved, err := resolvePath(path, specBaseDir)
	if err != nil {
		return nil, err
	}
	data, err := os.ReadFile(resolved)
	if err != nil {
		return nil, fmt.Errorf("read file %s: %w", resolved, err)
	}
	obj, err := parseSpecObject(data)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", resolved, err)
	}
	return obj, nil
}

func deepMergeObjects(base, overlay map[string]any) map[string]any {
	if base == nil {
		base = map[string]any{}
	}
	out := make(map[string]any, len(base))
	for k, v := range base {
		out[k] = v
	}
	for k, v := range overlay {
		if existing, ok := out[k]; ok {
			existingMap, existingOK := existing.(map[string]any)
			overlayMap, overlayOK := v.(map[string]any)
			if existingOK && overlayOK {
				out[k] = deepMergeObjects(existingMap, overlayMap)
				continue
			}
		}
		out[k] = v
	}
	return out
}

func resolveBuildGateSpecPathInPlace(spec map[string]any, specBaseDir string) error {
	bg, ok := spec["build_gate"].(map[string]any)
	if !ok {
		return nil
	}

	if healing, ok := bg["healing"].(map[string]any); ok {
		if byErrorKind, ok := healing["by_error_kind"].(map[string]any); ok {
			for errorKind, item := range byErrorKind {
				action, ok := item.(map[string]any)
				if !ok {
					continue
				}
				specPathValue, hasSpecPath := action["spec_path"]
				if !hasSpecPath {
					continue
				}
				specPath, ok := specPathValue.(string)
				if !ok {
					return fmt.Errorf("%s.spec_path: expected string path, got %T", errorKind, specPathValue)
				}
				fragment, err := readSpecObjectFromPath(specPath, specBaseDir)
				if err != nil {
					return fmt.Errorf("%s.spec_path: %w", errorKind, err)
				}

				// spec_path is a preprocessing-only key; it must not reach canonical validation.
				delete(action, "spec_path")
				byErrorKind[errorKind] = deepMergeObjects(fragment, action)
			}
		}
	}

	router, ok := bg["router"].(map[string]any)
	if !ok {
		return nil
	}
	specPathValue, hasSpecPath := router["spec_path"]
	if !hasSpecPath {
		return nil
	}
	specPath, ok := specPathValue.(string)
	if !ok {
		return fmt.Errorf("router.spec_path: expected string path, got %T", specPathValue)
	}
	fragment, err := readSpecObjectFromPath(specPath, specBaseDir)
	if err != nil {
		return fmt.Errorf("router.spec_path: %w", err)
	}
	delete(router, "spec_path")
	bg["router"] = deepMergeObjects(fragment, router)
	return nil
}

// resolveAmataSpecPathInPlace loads amata.spec from file paths and replaces each
// path with the file content so typed validation/runtime receive canonical spec text.
func resolveAmataSpecPathInPlace(spec map[string]any, specBaseDir string) error {
	if steps, ok := spec["steps"].([]any); ok {
		for i, raw := range steps {
			stepSpec, ok := raw.(map[string]any)
			if !ok {
				continue
			}
			if err := resolveAmataSpecInSection(stepSpec, fmt.Sprintf("steps[%d].amata", i), specBaseDir); err != nil {
				return err
			}
		}
	}

	bg, ok := spec["build_gate"].(map[string]any)
	if !ok {
		return nil
	}

	if router, ok := bg["router"].(map[string]any); ok {
		if err := resolveAmataSpecInSection(router, "build_gate.router.amata", specBaseDir); err != nil {
			return err
		}
	}

	if healing, ok := bg["healing"].(map[string]any); ok {
		if byErrorKind, ok := healing["by_error_kind"].(map[string]any); ok {
			for errorKind, raw := range byErrorKind {
				action, ok := raw.(map[string]any)
				if !ok {
					continue
				}
				if err := resolveAmataSpecInSection(action, fmt.Sprintf("build_gate.healing.by_error_kind.%s.amata", errorKind), specBaseDir); err != nil {
					return err
				}
			}
		}
	}
	return nil
}

func resolveAmataSpecInSection(section map[string]any, prefix, specBaseDir string) error {
	amataRaw, hasAmata := section["amata"]
	if !hasAmata {
		return nil
	}
	amata, ok := amataRaw.(map[string]any)
	if !ok {
		return fmt.Errorf("%s: expected object, got %T", prefix, amataRaw)
	}

	specRaw, hasSpec := amata["spec"]
	if !hasSpec {
		return nil
	}
	specPath, ok := specRaw.(string)
	if !ok {
		return fmt.Errorf("%s.spec: expected string path, got %T", prefix, specRaw)
	}

	resolvedPath, err := resolvePath(specPath, specBaseDir)
	if err != nil {
		return fmt.Errorf("%s.spec: %w", prefix, err)
	}
	specContent, err := os.ReadFile(resolvedPath)
	if err != nil {
		return fmt.Errorf("%s.spec: read file %s: %w", prefix, resolvedPath, err)
	}
	amata["spec"] = string(specContent)
	return nil
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

// buildSpecPayload loads a spec from file (YAML or JSON) and merges it with CLI flag overrides.
// CLI flags take precedence over spec file values. Returns raw JSON bytes.
//
// Processing order:
//  1. Load spec file (YAML or JSON format) if provided
//  2. Preprocess: resolve spec_path, amata.spec, image env, envs expansion
//  3. Compile Hydra records: ca/in/out/home authoring entries → canonical shortHash:dst form
//  4. Apply CLI flag overrides (higher precedence than spec file) to top-level fields
//  5. Apply defaults (e.g., gitlab_domain when gitlab_pat is set)
//
// Returns nil payload when neither spec file nor CLI overrides are provided.
//
// Multi-step semantics (steps[] array):
//   - Each entry in steps[] represents a sequential transformation step.
//   - All steps share the same repository and global build_gate policy (including healing).
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
		// Try JSON first, fallback to YAML
		if err := json.Unmarshal(data, &specMap); err != nil {
			// Not JSON; try YAML
			if err := yaml.Unmarshal(data, &specMap); err != nil {
				return nil, fmt.Errorf("parse spec file %s (not valid JSON or YAML): %w", specFile, err)
			}
		}
	} else {
		specMap = make(map[string]any)
	}

	if err := preprocessMigsSpecInPlace(specMap, specBaseDir); err != nil {
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
