package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// compileHookSourcesInPlace rewrites root-level hooks entries to canonical
// runtime forms:
//   - shortHash      (for local file/dir hook sources compiled to bundles)
//   - http(s) URL    (remote hook manifests kept as-is)
//
// Local directories are expanded by recursive hook.yaml discovery; each
// discovered hook.yaml contributes one canonical hook entry compiled from
// canonicalized manifest content.
func compileHookSourcesInPlace(ctx context.Context, base *url.URL, client *http.Client, spec map[string]any, specBaseDir string) error {
	raw, ok := spec["hooks"]
	if !ok {
		return nil
	}
	hooks, ok := raw.([]any)
	if !ok || len(hooks) == 0 {
		return nil
	}

	needsCompile := false
	for i, item := range hooks {
		s, ok := item.(string)
		if !ok {
			return fmt.Errorf("hooks[%d]: expected string, got %T", i, item)
		}
		s = strings.TrimSpace(s)
		if s == "" {
			return fmt.Errorf("hooks[%d]: empty hook source", i)
		}
		if isCanonicalHookSource(s) {
			continue
		}
		needsCompile = true
	}
	if !needsCompile {
		return nil
	}

	if base == nil {
		return fmt.Errorf("local hook sources found but no server base URL available for upload")
	}
	if client == nil {
		return fmt.Errorf("local hook sources found but no HTTP client available for upload")
	}

	seen := make(map[string]string)
	bundleMap := collectBundleMapFromSpec(spec)
	compiled := make([]any, 0, len(hooks))
	for i, item := range hooks {
		source := strings.TrimSpace(item.(string))
		if isCanonicalHookSource(source) {
			compiled = append(compiled, source)
			continue
		}

		resolved, err := resolvePath(source, specBaseDir)
		if err != nil {
			return fmt.Errorf("hooks[%d] %q: resolve source: %w", i, source, err)
		}
		info, err := os.Stat(resolved)
		if err != nil {
			return fmt.Errorf("hooks[%d] %q: stat source %q: %w", i, source, resolved, err)
		}
		if !info.IsDir() {
			hash, err := compileHookManifestSource(ctx, base, client, resolved, fmt.Sprintf("hooks[%d]", i), seen, bundleMap)
			if err != nil {
				return fmt.Errorf("hooks[%d] %q: compile hook source: %w", i, source, err)
			}
			compiled = append(compiled, hash)
			continue
		}

		manifests, err := discoverHookYAMLPaths(resolved)
		if err != nil {
			return fmt.Errorf("hooks[%d] %q: %w", i, source, err)
		}
		if len(manifests) == 0 {
			return fmt.Errorf("hooks[%d] %q: directory hook source %q: no hook.yaml files found", i, source, resolved)
		}
		for j, manifest := range manifests {
			hash, err := compileHookManifestSource(ctx, base, client, manifest, fmt.Sprintf("hooks[%d].manifest[%d]", i, j), seen, bundleMap)
			if err != nil {
				return fmt.Errorf("hooks[%d] %q: compile hook manifest %q: %w", i, source, manifest, err)
			}
			compiled = append(compiled, hash)
		}
	}

	spec["hooks"] = compiled
	if len(bundleMap) > 0 {
		spec["bundle_map"] = bundleMap
	}
	return nil
}

func isCanonicalHookSource(source string) bool {
	s := strings.TrimSpace(source)
	if s == "" {
		return false
	}
	if shortHashPattern.MatchString(s) {
		return true
	}
	u, err := url.Parse(s)
	if err != nil || u == nil {
		return false
	}
	return (u.Scheme == "http" || u.Scheme == "https") && u.Host != ""
}

func discoverHookYAMLPaths(root string) ([]string, error) {
	root = filepath.Clean(root)
	var out []string
	err := filepath.WalkDir(root, func(path string, d os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if d.IsDir() {
			return nil
		}
		if d.Name() != "hook.yaml" {
			return nil
		}
		out = append(out, filepath.Clean(path))
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("walk hook source directory %q: %w", root, err)
	}
	sort.Strings(out)
	return out, nil
}

func collectBundleMapFromSpec(spec map[string]any) map[string]string {
	out := make(map[string]string)
	switch existing := spec["bundle_map"].(type) {
	case map[string]string:
		for k, v := range existing {
			out[k] = v
		}
	case map[string]any:
		for k, v := range existing {
			s, ok := v.(string)
			if ok {
				out[k] = s
			}
		}
	}
	return out
}

func compileHookManifestSource(
	ctx context.Context,
	base *url.URL,
	client *http.Client,
	manifestPath string,
	prefix string,
	seen map[string]string,
	bundleMap map[string]string,
) (string, error) {
	data, err := os.ReadFile(manifestPath)
	if err != nil {
		return "", fmt.Errorf("read hook manifest %q: %w", manifestPath, err)
	}
	hookSpec, err := parseSpecInputToMap(data, filepath.Dir(manifestPath))
	if err != nil {
		return "", fmt.Errorf("parse hook manifest %q: %w", manifestPath, err)
	}
	if err := preprocessHookSpecInPlace(hookSpec, prefix); err != nil {
		return "", err
	}
	if err := compileHookHydraInPlace(ctx, base, client, hookSpec, prefix, filepath.Dir(manifestPath), seen, bundleMap); err != nil {
		return "", err
	}

	payload, err := json.Marshal(hookSpec)
	if err != nil {
		return "", fmt.Errorf("%s: marshal canonical hook spec: %w", prefix, err)
	}
	return compileInlineRecord(ctx, base, client, payload, seen, bundleMap)
}

func preprocessHookSpecInPlace(spec map[string]any, prefix string) error {
	rawSteps, ok := spec["steps"].([]any)
	if !ok {
		return nil
	}
	for i, raw := range rawSteps {
		step, ok := raw.(map[string]any)
		if !ok {
			continue
		}
		stepPrefix := fmt.Sprintf("%s.steps[%d]", prefix, i)
		if err := resolveImageInSection(step, stepPrefix); err != nil {
			return err
		}
		if err := resolveEnvsInPlace(step); err != nil {
			return fmt.Errorf("resolve envs (%s): %w", stepPrefix, err)
		}
	}
	return nil
}

func compileHookHydraInPlace(
	ctx context.Context,
	base *url.URL,
	client *http.Client,
	spec map[string]any,
	prefix string,
	specBaseDir string,
	seen map[string]string,
	bundleMap map[string]string,
) error {
	rawSteps, ok := spec["steps"].([]any)
	if !ok {
		return nil
	}
	for i, raw := range rawSteps {
		step, ok := raw.(map[string]any)
		if !ok {
			continue
		}
		if err := compileHydraBlock(ctx, base, client, step, fmt.Sprintf("%s.steps[%d]", prefix, i), specBaseDir, seen, bundleMap); err != nil {
			return err
		}
	}
	return nil
}

func compileInlineRecord(
	ctx context.Context,
	base *url.URL,
	client *http.Client,
	content []byte,
	seen map[string]string,
	bundleMap map[string]string,
) (string, error) {
	archiveBytes, err := buildInlineContentArchive(content)
	if err != nil {
		return "", fmt.Errorf("build inline archive: %w", err)
	}

	hash := computeArchiveShortHash(archiveBytes)
	cid := computeSpecBundleCID(archiveBytes)
	if bundleID, ok := seen[cid]; ok {
		bundleMap[hash] = bundleID
		return hash, nil
	}

	probeBundleID, exists, err := probeSpecBundleByCID(ctx, base, client, cid)
	if err != nil {
		return "", fmt.Errorf("probe: %w", err)
	}
	bundleID := probeBundleID
	if !exists || bundleID == "" {
		var uploadErr error
		bundleID, _, _, uploadErr = uploadSpecBundle(ctx, base, client, archiveBytes)
		if uploadErr != nil {
			return "", fmt.Errorf("upload: %w", uploadErr)
		}
	}

	seen[cid] = bundleID
	bundleMap[hash] = bundleID
	return hash, nil
}
