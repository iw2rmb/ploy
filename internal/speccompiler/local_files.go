package speccompiler

import (
	"fmt"
	"os"
	pathpkg "path"
	"path/filepath"
	"strings"

	"github.com/iw2rmb/ploy/internal/workflow/contracts"
	"gopkg.in/yaml.v3"
)

type localInMount struct {
	dst   string
	src   string
	isDir bool
}

func (c *Compiler) validateLocalFileRecords(spec map[string]any, specBaseDir string) error {
	steps, ok := spec["steps"].([]any)
	if !ok {
		return nil
	}
	for i, rawStep := range steps {
		step, ok := rawStep.(map[string]any)
		if !ok {
			continue
		}
		if err := c.validateLocalStepFileRecords(step, fmt.Sprintf("steps[%d]", i), specBaseDir); err != nil {
			return err
		}
	}
	return nil
}

func (c *Compiler) compileHydraRecordsLocalInPlace(spec map[string]any, specBaseDir string) error {
	steps, ok := spec["steps"].([]any)
	if !ok {
		return nil
	}
	for i, rawStep := range steps {
		step, ok := rawStep.(map[string]any)
		if !ok {
			continue
		}
		hashFile := func(src string) (string, error) {
			return c.localFileRecordHash(src, specBaseDir)
		}
		if err := compileHydraBlock(step, fmt.Sprintf("steps[%d]", i), hashFile); err != nil {
			return err
		}
	}
	return nil
}

func (c *Compiler) localFileRecordHash(srcPath, specBaseDir string) (string, error) {
	resolved, err := c.resolvePath(srcPath, specBaseDir)
	if err != nil {
		return "", fmt.Errorf("resolve source: %w", err)
	}
	archiveBytes, err := c.buildSourceArchive(resolved)
	if err != nil {
		return "", fmt.Errorf("build archive: %w", err)
	}
	return ArchiveShortHash(archiveBytes), nil
}

func (c *Compiler) validateLocalStepFileRecords(step map[string]any, prefix, specBaseDir string) error {
	mounts, err := c.validateLocalHydraEntries(step, prefix, specBaseDir)
	if err != nil {
		return err
	}
	return c.validateMountedYAMLIncludes(mounts, prefix)
}

func (c *Compiler) validateLocalHydraEntries(step map[string]any, prefix, specBaseDir string) ([]localInMount, error) {
	var mounts []localInMount
	for _, kind := range contracts.HydraFileKinds() {
		field := kind.String()
		entries, ok := step[field].([]any)
		if !ok {
			continue
		}
		for i, raw := range entries {
			value, ok := raw.(string)
			if !ok {
				return nil, fmt.Errorf("%s.%s[%d]: expected string, got %T", prefix, field, i, raw)
			}
			if isAlreadyCanonical(value) {
				if kind == contracts.HydraFileIn {
					parsed, err := contracts.ParseStoredEntry(kind, value)
					if err != nil {
						return nil, fmt.Errorf("%s.%s[%d]: %w", prefix, field, i, err)
					}
					mounts = append(mounts, localInMount{dst: parsed.Dst})
				}
				continue
			}
			entry, err := parseAuthoringEntry(kind, value)
			if err != nil {
				return nil, fmt.Errorf("%s.%s[%d]: %w", prefix, field, i, err)
			}
			resolved, info, err := c.statLocalFileRecordSource(entry.src, specBaseDir)
			if err != nil {
				return nil, fmt.Errorf("%s.%s[%d]: %w", prefix, field, i, err)
			}
			if kind == contracts.HydraFileIn {
				mounts = append(mounts, localInMount{dst: entry.dst, src: resolved, isDir: info.IsDir()})
			}
		}
	}
	return mounts, nil
}

func (c *Compiler) statLocalFileRecordSource(srcPath, specBaseDir string) (string, os.FileInfo, error) {
	resolved, err := c.resolvePath(srcPath, specBaseDir)
	if err != nil {
		return "", nil, fmt.Errorf("resolve source: %w", err)
	}
	info, err := c.source.Stat(resolved)
	if err != nil {
		return "", nil, fmt.Errorf("source %s: %w", resolved, err)
	}
	return resolved, info, nil
}

func (c *Compiler) validateMountedYAMLIncludes(mounts []localInMount, prefix string) error {
	byDst := make(map[string]localInMount, len(mounts))
	for _, mount := range mounts {
		if mount.dst != "" {
			byDst[mount.dst] = mount
		}
	}
	seen := make(map[string]struct{})
	for _, mount := range mounts {
		if err := c.validateMountYAMLIncludes(mount, byDst, seen, prefix); err != nil {
			return err
		}
	}
	return nil
}

func (c *Compiler) validateMountYAMLIncludes(mount localInMount, byDst map[string]localInMount, seen map[string]struct{}, prefix string) error {
	if mount.src == "" || mount.isDir || !isYAMLPath(mount.src) {
		return nil
	}
	key := mount.src + "=>" + mount.dst
	if _, ok := seen[key]; ok {
		return nil
	}
	seen[key] = struct{}{}

	refs, err := c.collectYAMLIncludeRefs(mount.src)
	if err != nil {
		return fmt.Errorf("%s.in include scan %s: %w", prefix, mount.dst, err)
	}
	for _, ref := range refs {
		include, err := parseMountedIncludeRef(mount.src, mount.dst, ref)
		if err != nil {
			return fmt.Errorf("%s.in include %s: %w", prefix, ref, err)
		}
		if include.localPath != "" {
			if _, err := c.source.Stat(include.localPath); err != nil {
				return fmt.Errorf("%s.in include %s: source %s: %w", prefix, ref, include.localPath, err)
			}
		}
		target, ok := findMountForRuntimePath(include.runtimePath, byDst)
		if !ok {
			return fmt.Errorf("%s.in include %s: target %s is not mounted by this step's in entries", prefix, ref, include.runtimePath)
		}
		if err := c.validateMountYAMLIncludes(target, byDst, seen, prefix); err != nil {
			return err
		}
	}
	return nil
}

func (c *Compiler) collectYAMLIncludeRefs(filePath string) ([]string, error) {
	data, err := c.source.ReadFile(filePath)
	if err != nil {
		return nil, fmt.Errorf("read: %w", err)
	}
	var root yaml.Node
	if err := yaml.Unmarshal(data, &root); err != nil {
		return nil, fmt.Errorf("decode YAML: %w", err)
	}
	var refs []string
	collectYAMLIncludeRefsFromNode(&root, &refs)
	return refs, nil
}

func collectYAMLIncludeRefsFromNode(node *yaml.Node, refs *[]string) {
	if node == nil {
		return
	}
	if node.Kind == yaml.AliasNode && node.Alias != nil {
		collectYAMLIncludeRefsFromNode(node.Alias, refs)
		return
	}
	if node.Kind == yaml.ScalarNode && node.Tag == "!include" {
		*refs = append(*refs, node.Value)
		return
	}
	for _, child := range node.Content {
		collectYAMLIncludeRefsFromNode(child, refs)
	}
}

type mountedIncludeRef struct {
	localPath   string
	runtimePath string
}

func parseMountedIncludeRef(sourceLocalPath, sourceRuntimePath, raw string) (mountedIncludeRef, error) {
	value := strings.TrimSpace(raw)
	if value == "" {
		return mountedIncludeRef{}, fmt.Errorf("path must not be empty")
	}
	pathPart := value
	if hash := strings.Index(value, "#"); hash >= 0 {
		pathPart = strings.TrimSpace(value[:hash])
		pointer := strings.TrimSpace(value[hash+1:])
		if pointer != "" && !strings.HasPrefix(pointer, "/") {
			return mountedIncludeRef{}, fmt.Errorf("fragment must start with /")
		}
	}
	if pathPart == "" {
		return mountedIncludeRef{}, fmt.Errorf("path must not be empty")
	}

	ref := mountedIncludeRef{}
	if filepath.IsAbs(pathPart) {
		ref.runtimePath = pathpkg.Clean(filepath.ToSlash(pathPart))
	} else {
		ref.localPath = filepath.Clean(filepath.Join(filepath.Dir(sourceLocalPath), pathPart))
		ref.runtimePath = pathpkg.Clean(pathpkg.Join(pathpkg.Dir(sourceRuntimePath), filepath.ToSlash(pathPart)))
	}
	if !strings.HasPrefix(ref.runtimePath, "/in/") {
		return mountedIncludeRef{}, fmt.Errorf("target %s must be under /in", ref.runtimePath)
	}
	return ref, nil
}

func findMountForRuntimePath(runtimePath string, byDst map[string]localInMount) (localInMount, bool) {
	if mount, ok := byDst[runtimePath]; ok {
		return mount, true
	}
	for _, mount := range byDst {
		if !mount.isDir {
			continue
		}
		prefix := strings.TrimRight(mount.dst, "/") + "/"
		if strings.HasPrefix(runtimePath, prefix) {
			return mount, true
		}
	}
	return localInMount{}, false
}

func isYAMLPath(filePath string) bool {
	switch strings.ToLower(filepath.Ext(filePath)) {
	case ".yaml", ".yml":
		return true
	default:
		return false
	}
}
