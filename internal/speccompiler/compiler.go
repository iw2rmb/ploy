// Package speccompiler compiles authoring specs into canonical run snapshots.
// Specs use a single canonical shape:
//   - steps[] array with one entry per step (even single-step runs)
//   - global build gate policy under build_gate
package speccompiler

import (
	"context"
	"encoding/json"
	"fmt"
	"io/fs"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"github.com/iw2rmb/ploy/internal/workflow/contracts"
	"gopkg.in/yaml.v3"
)

const specEnvNamePattern = `[A-Za-z_][A-Za-z0-9_]*`

var (
	specEnvNameRE        = regexp.MustCompile(`^` + specEnvNamePattern + `$`)
	specEnvPlaceholderRE = regexp.MustCompile(`\$\{(` + specEnvNamePattern + `)\}|\$(` + specEnvNamePattern + `)`)
)

func IsEnvironmentPlaceholderName(name string) bool {
	return specEnvNameRE.MatchString(name)
}

// Source owns path policy and file access for one compilation source.
type Source interface {
	ResolveSpec(path string) (string, error)
	ResolveBaseDir(path string) (string, error)
	ResolveReference(path, baseDir string) (string, error)
	ResolveFileRecord(path, baseDir string) (string, error)
	ReadFile(path string) ([]byte, error)
	Stat(path string) (fs.FileInfo, error)
	ReadDir(path string) ([]fs.DirEntry, error)
	WorkingDir() (string, error)
}

// BundleStore persists a content-addressed source archive and returns its ID.
type BundleStore interface {
	Ensure(ctx context.Context, cid string, archive []byte) (string, error)
}

// Options supplies the caller-owned dependencies used during compilation.
type Options struct {
	Source       Source
	LookupEnv    func(string) (string, bool)
	ApplyOverlay func(map[string]any) error
	Bundles      BundleStore
}

// Compiler is safe for concurrent use when its supplied dependencies are safe.
type Compiler struct {
	source       Source
	lookupEnv    func(string) (string, bool)
	applyOverlay func(map[string]any) error
	bundles      BundleStore
}

func New(opts Options) (*Compiler, error) {
	if opts.Source == nil {
		return nil, fmt.Errorf("spec compiler source is required")
	}
	if opts.LookupEnv == nil {
		opts.LookupEnv = func(string) (string, bool) { return "", false }
	}
	if opts.ApplyOverlay == nil {
		opts.ApplyOverlay = func(map[string]any) error { return nil }
	}
	return &Compiler{
		source:       opts.Source,
		lookupEnv:    opts.LookupEnv,
		applyOverlay: opts.ApplyOverlay,
		bundles:      opts.Bundles,
	}, nil
}

func (c *Compiler) Normalize(ctx context.Context, data []byte, specBaseDir string) (json.RawMessage, error) {
	return c.normalizeWithStepSelector(ctx, data, specBaseDir, "")
}

func (c *Compiler) normalizeWithStepSelector(ctx context.Context, data []byte, specBaseDir string, stepSelector string) (json.RawMessage, error) {
	var err error
	specBaseDir, err = c.source.ResolveBaseDir(specBaseDir)
	if err != nil {
		return nil, fmt.Errorf("resolve spec base directory: %w", err)
	}
	raw, err := c.parseSpecInputToMap(data, specBaseDir)
	if err != nil {
		return nil, fmt.Errorf("parse spec (not valid JSON or YAML): %w", err)
	}
	sourcePath, err := c.composeSpecRootPath(specBaseDir)
	if err != nil {
		return nil, err
	}
	if err := c.expandSpecRefsInPlace(raw, sourcePath); err != nil {
		return nil, fmt.Errorf("expand refs: %w", err)
	}
	if err := selectStepInPlace(raw, stepSelector, sourcePath); err != nil {
		return nil, err
	}
	if err := c.preprocessMigsSpecInPlace(raw); err != nil {
		return nil, err
	}

	// Apply overlays before file-record compilation so added paths become canonical too.
	if err := c.applyOverlay(raw); err != nil {
		return nil, err
	}
	if err := validateSpecShape(raw); err != nil {
		return nil, err
	}

	if err := c.validateLocalFileRecords(raw, specBaseDir); err != nil {
		return nil, fmt.Errorf("validate local file records: %w", err)
	}

	if err := c.compileHydraRecordsInPlace(ctx, raw, specBaseDir); err != nil {
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

// ValidateFile validates a JSON/YAML spec without persisting bundles.
func (c *Compiler) ValidateFile(path string) (json.RawMessage, error) {
	cleanSpecPath, err := c.source.ResolveSpec(path)
	if err != nil {
		return nil, fmt.Errorf("resolve spec file %s: %w", path, err)
	}
	data, err := c.source.ReadFile(cleanSpecPath)
	if err != nil {
		return nil, fmt.Errorf("read spec file %s: %w", cleanSpecPath, err)
	}
	return c.Validate(data, filepath.Dir(cleanSpecPath))
}

// Validate validates a JSON/YAML spec without persisting bundles.
func (c *Compiler) Validate(data []byte, specBaseDir string) (json.RawMessage, error) {
	var err error
	specBaseDir, err = c.source.ResolveBaseDir(specBaseDir)
	if err != nil {
		return nil, fmt.Errorf("resolve spec base directory: %w", err)
	}
	raw, err := c.parseSpecInputToMap(data, specBaseDir)
	if err != nil {
		return nil, fmt.Errorf("parse spec (not valid JSON or YAML): %w", err)
	}
	sourcePath, err := c.composeSpecRootPath(specBaseDir)
	if err != nil {
		return nil, err
	}
	if err := c.expandSpecRefsInPlace(raw, sourcePath); err != nil {
		return nil, fmt.Errorf("expand refs: %w", err)
	}
	if err := c.preprocessMigsSpecInPlace(raw); err != nil {
		return nil, err
	}
	if err := c.applyOverlay(raw); err != nil {
		return nil, err
	}
	if err := c.validateLocalFileRecords(raw, specBaseDir); err != nil {
		return nil, fmt.Errorf("validate local file records: %w", err)
	}
	if err := c.compileHydraRecordsLocalInPlace(raw, specBaseDir); err != nil {
		return nil, fmt.Errorf("compile local file records: %w", err)
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

func (c *Compiler) preprocessMigsSpecInPlace(spec map[string]any) error {
	if err := c.resolveImageEnvInPlace(spec); err != nil {
		return fmt.Errorf("resolve image env placeholders: %w", err)
	}

	// Expand $VAR/${VAR} placeholders in envs values at all levels.
	if err := c.resolveEnvsInPlace(spec); err != nil {
		return fmt.Errorf("resolve envs (top-level): %w", err)
	}

	if steps, ok := spec["steps"].([]any); ok {
		for i, s := range steps {
			if stepEntry, ok := s.(map[string]any); ok {
				if err := c.resolveEnvsInPlace(stepEntry); err != nil {
					return fmt.Errorf("resolve envs (steps[%d]): %w", i, err)
				}
			}
		}
	}

	return nil
}

// resolveEnvsInPlace expands $VAR and ${VAR} placeholders in envs string values.
func (c *Compiler) resolveEnvsInPlace(spec map[string]any) error {
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
			expanded, err := c.expandSpecEnvValue(s)
			if err != nil {
				return fmt.Errorf("envs[%s]: %w", k, err)
			}
			envs[k] = expanded
		}
	case map[string]string:
		expanded := make(map[string]any, len(envs))
		for k, v := range envs {
			exp, err := c.expandSpecEnvValue(v)
			if err != nil {
				return fmt.Errorf("envs[%s]: %w", k, err)
			}
			expanded[k] = exp
		}
		spec["envs"] = expanded
	}
	return nil
}

func (c *Compiler) resolveImageEnvInPlace(spec map[string]any) error {
	if steps, ok := spec["steps"].([]any); ok {
		for i, raw := range steps {
			step, ok := raw.(map[string]any)
			if !ok {
				continue
			}
			if err := c.resolveImageInSection(step, fmt.Sprintf("steps[%d]", i)); err != nil {
				return err
			}
		}
	}

	if buildGateRaw, ok := spec["build_gate"]; ok {
		buildGate, ok := buildGateRaw.(map[string]any)
		if !ok {
			return fmt.Errorf("build_gate: expected object, got %T", buildGateRaw)
		}
		if err := c.resolveBuildGateImageRulesInPlace(buildGate); err != nil {
			return err
		}
	}

	return nil
}

func (c *Compiler) resolveBuildGateImageRulesInPlace(buildGate map[string]any) error {
	rawImages, exists := buildGate["images"]
	if !exists || rawImages == nil {
		return nil
	}
	images, ok := rawImages.([]any)
	if !ok {
		return nil
	}
	for i, item := range images {
		rule, ok := item.(map[string]any)
		if !ok {
			continue
		}
		rawImage, exists := rule["image"]
		if !exists {
			continue
		}
		image, ok := rawImage.(string)
		if !ok {
			continue
		}
		stackExp, err := stackExpectationFromBuildGateRule(rule, i)
		if err != nil {
			return err
		}
		expanded, err := contracts.ExpandImageTemplate(image, stackExp)
		if err != nil {
			return fmt.Errorf("build_gate.images[%d].image: %w", i, err)
		}
		rule["image"] = expanded
	}
	return nil
}

func stackExpectationFromBuildGateRule(rule map[string]any, index int) (*contracts.StackExpectation, error) {
	rawStack, exists := rule["stack"]
	if !exists || rawStack == nil {
		return nil, nil
	}
	stack, ok := rawStack.(map[string]any)
	if !ok {
		return nil, fmt.Errorf("build_gate.images[%d].stack: expected object, got %T", index, rawStack)
	}

	exp := &contracts.StackExpectation{}
	if languageRaw, exists := stack["language"]; exists && languageRaw != nil {
		language, ok := languageRaw.(string)
		if !ok {
			return nil, fmt.Errorf("build_gate.images[%d].stack.language: expected string, got %T", index, languageRaw)
		}
		exp.Language = strings.TrimSpace(language)
	}
	if toolRaw, exists := stack["tool"]; exists && toolRaw != nil {
		tool, ok := toolRaw.(string)
		if !ok {
			return nil, fmt.Errorf("build_gate.images[%d].stack.tool: expected string, got %T", index, toolRaw)
		}
		exp.Tool = strings.TrimSpace(tool)
	}
	if releaseRaw, exists := stack["release"]; exists && releaseRaw != nil {
		release, err := contracts.ParseReleaseValue(releaseRaw, fmt.Sprintf("build_gate.images[%d].stack.release", index))
		if err != nil {
			return nil, err
		}
		exp.Release = strings.TrimSpace(release)
	}
	return exp, nil
}

func (c *Compiler) resolveImageInSection(section map[string]any, prefix string) error {
	raw, exists := section["image"]
	if !exists {
		return nil
	}

	switch image := raw.(type) {
	case string:
		expanded, err := c.expandSpecEnvValue(image)
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
			expanded, err := c.expandSpecEnvValue(value)
			if err != nil {
				return fmt.Errorf("%s.image[%q]: %w", prefix, stack, err)
			}
			image[stack] = expanded
		}
	case map[string]string:
		for stack, value := range image {
			expanded, err := c.expandSpecEnvValue(value)
			if err != nil {
				return fmt.Errorf("%s.image[%q]: %w", prefix, stack, err)
			}
			image[stack] = expanded
		}
		section["image"] = image
	}

	return nil
}

func (c *Compiler) resolvePath(path string, baseDir ...string) (string, error) {
	base := ""
	if len(baseDir) > 0 {
		base = baseDir[0]
	}
	return c.source.ResolveFileRecord(path, base)
}

func (c *Compiler) parseSpecInputToMap(data []byte, specBaseDir string) (map[string]any, error) {
	var obj map[string]any
	if err := json.Unmarshal(data, &obj); err != nil {
		specRootPath, rootErr := c.composeSpecRootPath(specBaseDir)
		if rootErr != nil {
			return nil, rootErr
		}
		composed, composeErr := c.composeSpecYAML(data, specRootPath)
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

func (c *Compiler) composeSpecRootPath(specBaseDir string) (string, error) {
	base := strings.TrimSpace(specBaseDir)
	if base == "" {
		wd, err := c.source.WorkingDir()
		if err != nil {
			return "", fmt.Errorf("resolve working directory for spec: %w", err)
		}
		base = wd
	}
	return filepath.Join(base, ".root-spec.yaml"), nil
}

func (c *Compiler) expandSpecEnvValue(raw string) (string, error) {
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

		if v, ok := c.lookupEnv(name); ok {
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

// Overrides contains caller-supplied values that take precedence over source values.
type Overrides struct {
	Envs     []string
	StepEnvs map[string][]string
	Image    string
	Command  string
}

// Build loads a spec and compiles it into canonical JSON.
//
// Processing order:
//  1. Load and compose the spec, then select a step when requested
//  2. Apply step environment overrides before source placeholder expansion
//  3. Expand image and environment placeholders, then apply the optional overlay
//  4. Apply top-level caller overrides and compile local file records
//  5. Validate the current spec contract
//
// Returns nil payload when neither a spec file nor overrides are provided.
//
// Multi-step semantics (steps[] array):
//   - Each entry in steps[] represents a sequential transformation step.
//   - All steps share the same repository and global build_gate policy.
//   - Image and command overrides do not apply when len(steps) > 1.
func (c *Compiler) Build(ctx context.Context, specFile string, overrides Overrides) ([]byte, error) {
	return c.BuildSelected(ctx, specFile, "", overrides)
}

func (c *Compiler) BuildSelected(ctx context.Context, specFile string, stepSelector string, overrides Overrides) ([]byte, error) {
	// Start with spec from file (if provided)
	var specMap map[string]any
	specBaseDir := ""
	specSourcePath := ""
	if specFile != "" {
		cleanSpecPath, err := c.source.ResolveSpec(specFile)
		if err != nil {
			return nil, fmt.Errorf("resolve spec file %s: %w", specFile, err)
		}
		specBaseDir = filepath.Dir(cleanSpecPath)
		specSourcePath = cleanSpecPath
		data, err := c.source.ReadFile(cleanSpecPath)
		if err != nil {
			return nil, fmt.Errorf("read spec file %s: %w", cleanSpecPath, err)
		}
		specMap, err = c.parseSpecInputToMap(data, specBaseDir)
		if err != nil {
			return nil, fmt.Errorf("parse spec file %s (not valid JSON or YAML): %w", cleanSpecPath, err)
		}
	} else {
		specMap = make(map[string]any)
		sourcePath, err := c.composeSpecRootPath(specBaseDir)
		if err != nil {
			return nil, err
		}
		specSourcePath = sourcePath
	}

	if err := c.expandSpecRefsInPlace(specMap, specSourcePath); err != nil {
		return nil, fmt.Errorf("expand refs: %w", err)
	}
	if err := selectStepInPlace(specMap, stepSelector, specSourcePath); err != nil {
		return nil, err
	}

	if err := applyStepEnvOverridesInPlace(specMap, overrides.StepEnvs); err != nil {
		return nil, err
	}

	if err := c.preprocessMigsSpecInPlace(specMap); err != nil {
		return nil, err
	}

	if err := c.applyOverlay(specMap); err != nil {
		return nil, err
	}

	// Caller overrides take precedence over source values.
	hasOverrides := len(overrides.Envs) > 0 || len(overrides.StepEnvs) > 0 || overrides.Image != "" || overrides.Command != ""

	// Only proceed if there is a spec file or an override.
	if len(specMap) == 0 && !hasOverrides {
		return nil, nil
	}

	if _, hasSteps := specMap["steps"]; hasSteps || !hasOverrides {
		if err := validateSpecShape(specMap); err != nil {
			return nil, err
		}
	}

	if len(overrides.Envs) > 0 {
		// Start from existing envs.
		current := make(map[string]any)
		if existingEnvs, ok := specMap["envs"].(map[string]any); ok {
			for k, v := range existingEnvs {
				if s, ok := v.(string); ok {
					current[k] = s
				}
			}
		}

		// Later override entries win, matching command-line order.
		for _, kv := range overrides.Envs {
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
	if stepsLen <= 1 && (overrides.Image != "" || overrides.Command != "") {
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

		if overrides.Image != "" {
			step0["image"] = overrides.Image
		}
		if overrides.Command != "" {
			// Allow JSON array for command to pass argv directly to containers with ENTRYPOINT.
			// Fallback to plain string when not a JSON array.
			var asArray []string
			if strings.HasPrefix(overrides.Command, "[") && strings.HasSuffix(overrides.Command, "]") {
				if err := json.Unmarshal([]byte(overrides.Command), &asArray); err == nil && len(asArray) > 0 {
					step0["command"] = asArray
				} else {
					step0["command"] = overrides.Command
				}
			} else {
				step0["command"] = overrides.Command
			}
		}
	}

	if len(specMap) == 0 {
		return nil, nil
	}

	if err := validateSpecShape(specMap); err != nil {
		return nil, err
	}

	if err := c.validateLocalFileRecords(specMap, specBaseDir); err != nil {
		return nil, fmt.Errorf("validate local file records: %w", err)
	}

	if err := c.compileHydraRecordsInPlace(ctx, specMap, specBaseDir); err != nil {
		return nil, err
	}

	// Marshal to JSON for submission
	jsonBytes, err := json.Marshal(specMap)
	if err != nil {
		return nil, fmt.Errorf("marshal spec: %w", err)
	}

	// Validate spec using the canonical parser to catch structural issues early.
	// Validate the final snapshot before returning it to either caller.
	if _, err := contracts.ParseMigSpecJSON(jsonBytes); err != nil {
		return nil, fmt.Errorf("validate spec: %w", err)
	}

	return jsonBytes, nil
}

func validateSpecShape(spec map[string]any) error {
	jsonBytes, err := json.Marshal(spec)
	if err != nil {
		return fmt.Errorf("marshal spec for validation: %w", err)
	}
	if err := contracts.ValidateMigSpecJSON(jsonBytes); err != nil {
		return fmt.Errorf("validate spec: %w", err)
	}
	return nil
}
