package main

import (
	"context"
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
// discovered hook.yaml contributes one canonical hook entry by compiling the
// hook's parent directory.
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
			hash, err := compileFileRecord(ctx, base, client, resolved, "", seen, bundleMap)
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
		for _, manifest := range manifests {
			parentDir := filepath.Dir(manifest)
			hash, err := compileFileRecord(ctx, base, client, parentDir, "", seen, bundleMap)
			if err != nil {
				return fmt.Errorf("hooks[%d] %q: compile hook directory %q: %w", i, source, parentDir, err)
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
