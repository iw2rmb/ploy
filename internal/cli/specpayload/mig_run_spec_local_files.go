package specpayload

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

func validateLocalFileRecords(spec map[string]any, specBaseDir string) error {
	steps, ok := spec["steps"].([]any)
	if !ok {
		return nil
	}
	for i, rawStep := range steps {
		step, ok := rawStep.(map[string]any)
		if !ok {
			continue
		}
		if err := validateLocalStepFileRecords(step, fmt.Sprintf("steps[%d]", i), specBaseDir); err != nil {
			return err
		}
	}
	return nil
}

func compileHydraRecordsLocalInPlace(spec map[string]any, specBaseDir string) error {
	steps, ok := spec["steps"].([]any)
	if !ok {
		return nil
	}
	for i, rawStep := range steps {
		step, ok := rawStep.(map[string]any)
		if !ok {
			continue
		}
		if err := compileLocalHydraBlock(step, fmt.Sprintf("steps[%d]", i), specBaseDir); err != nil {
			return err
		}
	}
	return nil
}

func compileLocalHydraBlock(step map[string]any, prefix, specBaseDir string) error {
	if err := compileLocalInEntries(step, prefix, specBaseDir); err != nil {
		return err
	}
	if err := compileLocalOutEntries(step, prefix, specBaseDir); err != nil {
		return err
	}
	if err := compileLocalHomeEntries(step, prefix, specBaseDir); err != nil {
		return err
	}
	return compileLocalTmpEntries(step, prefix, specBaseDir)
}

func compileLocalInEntries(step map[string]any, prefix, specBaseDir string) error {
	entries, ok := step["in"].([]any)
	if !ok || len(entries) == 0 {
		return nil
	}
	compiled := make([]any, len(entries))
	for i, raw := range entries {
		entry, ok := raw.(string)
		if !ok {
			return fmt.Errorf("%s.in[%d]: expected string, got %T", prefix, i, raw)
		}
		if isAlreadyCanonical("in", entry) {
			compiled[i] = entry
			continue
		}
		src, dst, err := parseAuthoringInEntry(entry)
		if err != nil {
			return fmt.Errorf("%s.in[%d]: %w", prefix, i, err)
		}
		hash, err := localFileRecordHash(src, specBaseDir)
		if err != nil {
			return fmt.Errorf("%s.in[%d]: %w", prefix, i, err)
		}
		compiled[i] = hash + ":" + dst
	}
	step["in"] = compiled
	return nil
}

func compileLocalOutEntries(step map[string]any, prefix, specBaseDir string) error {
	entries, ok := step["out"].([]any)
	if !ok || len(entries) == 0 {
		return nil
	}
	compiled := make([]any, len(entries))
	for i, raw := range entries {
		entry, ok := raw.(string)
		if !ok {
			return fmt.Errorf("%s.out[%d]: expected string, got %T", prefix, i, raw)
		}
		if isAlreadyCanonical("out", entry) {
			compiled[i] = entry
			continue
		}
		src, dst, err := parseAuthoringOutEntry(entry)
		if err != nil {
			return fmt.Errorf("%s.out[%d]: %w", prefix, i, err)
		}
		hash, err := localFileRecordHash(src, specBaseDir)
		if err != nil {
			return fmt.Errorf("%s.out[%d]: %w", prefix, i, err)
		}
		compiled[i] = hash + ":" + dst
	}
	step["out"] = compiled
	return nil
}

func compileLocalHomeEntries(step map[string]any, prefix, specBaseDir string) error {
	entries, ok := step["home"].([]any)
	if !ok || len(entries) == 0 {
		return nil
	}
	compiled := make([]any, len(entries))
	for i, raw := range entries {
		entry, ok := raw.(string)
		if !ok {
			return fmt.Errorf("%s.home[%d]: expected string, got %T", prefix, i, raw)
		}
		body := entry
		if strings.HasSuffix(entry, ":ro") {
			body = entry[:len(entry)-3]
		}
		if isAlreadyCanonical("home", body) {
			compiled[i] = entry
			continue
		}
		src, dst, readOnly, err := parseAuthoringHomeEntry(entry)
		if err != nil {
			return fmt.Errorf("%s.home[%d]: %w", prefix, i, err)
		}
		hash, err := localFileRecordHash(src, specBaseDir)
		if err != nil {
			return fmt.Errorf("%s.home[%d]: %w", prefix, i, err)
		}
		canonical := hash + ":" + dst
		if readOnly {
			canonical += ":ro"
		}
		compiled[i] = canonical
	}
	step["home"] = compiled
	return nil
}

func compileLocalTmpEntries(step map[string]any, prefix, specBaseDir string) error {
	entries, ok := step["tmp"].([]any)
	if !ok || len(entries) == 0 {
		return nil
	}
	compiled := make([]any, len(entries))
	for i, raw := range entries {
		entry, ok := raw.(string)
		if !ok {
			return fmt.Errorf("%s.tmp[%d]: expected string, got %T", prefix, i, raw)
		}
		if isAlreadyCanonical("tmp", entry) {
			compiled[i] = entry
			continue
		}
		src, dst, err := parseAuthoringTmpEntry(entry)
		if err != nil {
			return fmt.Errorf("%s.tmp[%d]: %w", prefix, i, err)
		}
		hash, err := localFileRecordHash(src, specBaseDir)
		if err != nil {
			return fmt.Errorf("%s.tmp[%d]: %w", prefix, i, err)
		}
		compiled[i] = hash + ":" + dst
	}
	step["tmp"] = compiled
	return nil
}

func localFileRecordHash(srcPath, specBaseDir string) (string, error) {
	resolved, err := resolvePath(srcPath, specBaseDir)
	if err != nil {
		return "", fmt.Errorf("resolve source: %w", err)
	}
	archiveBytes, err := buildSourceArchive(resolved)
	if err != nil {
		return "", fmt.Errorf("build archive: %w", err)
	}
	return computeArchiveShortHash(archiveBytes), nil
}

func validateLocalStepFileRecords(step map[string]any, prefix, specBaseDir string) error {
	mounts, err := collectLocalInMounts(step, prefix, specBaseDir)
	if err != nil {
		return err
	}
	if err := validateLocalOutEntries(step, prefix, specBaseDir); err != nil {
		return err
	}
	if err := validateLocalHomeEntries(step, prefix, specBaseDir); err != nil {
		return err
	}
	if err := validateLocalTmpEntries(step, prefix, specBaseDir); err != nil {
		return err
	}
	return validateMountedYAMLIncludes(mounts, prefix)
}

func collectLocalInMounts(step map[string]any, prefix, specBaseDir string) ([]localInMount, error) {
	entries, ok := step["in"].([]any)
	if !ok || len(entries) == 0 {
		return nil, nil
	}
	mounts := make([]localInMount, 0, len(entries))
	for i, raw := range entries {
		entry, ok := raw.(string)
		if !ok {
			return nil, fmt.Errorf("%s.in[%d]: expected string, got %T", prefix, i, raw)
		}
		if isAlreadyCanonical("in", entry) {
			parsed, err := contracts.ParseStoredInEntry(entry)
			if err != nil {
				return nil, fmt.Errorf("%s.in[%d]: %w", prefix, i, err)
			}
			mounts = append(mounts, localInMount{dst: parsed.Dst})
			continue
		}
		src, dst, err := parseAuthoringInEntry(entry)
		if err != nil {
			return nil, fmt.Errorf("%s.in[%d]: %w", prefix, i, err)
		}
		resolved, info, err := statLocalFileRecordSource(src, specBaseDir)
		if err != nil {
			return nil, fmt.Errorf("%s.in[%d]: %w", prefix, i, err)
		}
		mounts = append(mounts, localInMount{dst: dst, src: resolved, isDir: info.IsDir()})
	}
	return mounts, nil
}

func validateLocalOutEntries(step map[string]any, prefix, specBaseDir string) error {
	entries, ok := step["out"].([]any)
	if !ok || len(entries) == 0 {
		return nil
	}
	for i, raw := range entries {
		entry, ok := raw.(string)
		if !ok {
			return fmt.Errorf("%s.out[%d]: expected string, got %T", prefix, i, raw)
		}
		if isAlreadyCanonical("out", entry) {
			continue
		}
		src, _, err := parseAuthoringOutEntry(entry)
		if err != nil {
			return fmt.Errorf("%s.out[%d]: %w", prefix, i, err)
		}
		if _, _, err := statLocalFileRecordSource(src, specBaseDir); err != nil {
			return fmt.Errorf("%s.out[%d]: %w", prefix, i, err)
		}
	}
	return nil
}

func validateLocalHomeEntries(step map[string]any, prefix, specBaseDir string) error {
	entries, ok := step["home"].([]any)
	if !ok || len(entries) == 0 {
		return nil
	}
	for i, raw := range entries {
		entry, ok := raw.(string)
		if !ok {
			return fmt.Errorf("%s.home[%d]: expected string, got %T", prefix, i, raw)
		}
		body := entry
		if strings.HasSuffix(entry, ":ro") {
			body = entry[:len(entry)-3]
		}
		if isAlreadyCanonical("home", body) {
			continue
		}
		src, _, _, err := parseAuthoringHomeEntry(entry)
		if err != nil {
			return fmt.Errorf("%s.home[%d]: %w", prefix, i, err)
		}
		if _, _, err := statLocalFileRecordSource(src, specBaseDir); err != nil {
			return fmt.Errorf("%s.home[%d]: %w", prefix, i, err)
		}
	}
	return nil
}

func validateLocalTmpEntries(step map[string]any, prefix, specBaseDir string) error {
	entries, ok := step["tmp"].([]any)
	if !ok || len(entries) == 0 {
		return nil
	}
	for i, raw := range entries {
		entry, ok := raw.(string)
		if !ok {
			return fmt.Errorf("%s.tmp[%d]: expected string, got %T", prefix, i, raw)
		}
		if isAlreadyCanonical("tmp", entry) {
			continue
		}
		src, _, err := parseAuthoringTmpEntry(entry)
		if err != nil {
			return fmt.Errorf("%s.tmp[%d]: %w", prefix, i, err)
		}
		if _, _, err := statLocalFileRecordSource(src, specBaseDir); err != nil {
			return fmt.Errorf("%s.tmp[%d]: %w", prefix, i, err)
		}
	}
	return nil
}

func statLocalFileRecordSource(srcPath, specBaseDir string) (string, os.FileInfo, error) {
	resolved, err := resolvePath(srcPath, specBaseDir)
	if err != nil {
		return "", nil, fmt.Errorf("resolve source: %w", err)
	}
	info, err := os.Stat(resolved)
	if err != nil {
		return "", nil, fmt.Errorf("source %s: %w", resolved, err)
	}
	return resolved, info, nil
}

func validateMountedYAMLIncludes(mounts []localInMount, prefix string) error {
	byDst := make(map[string]localInMount, len(mounts))
	for _, mount := range mounts {
		if mount.dst != "" {
			byDst[mount.dst] = mount
		}
	}
	seen := make(map[string]struct{})
	for _, mount := range mounts {
		if err := validateMountYAMLIncludes(mount, byDst, seen, prefix); err != nil {
			return err
		}
	}
	return nil
}

func validateMountYAMLIncludes(mount localInMount, byDst map[string]localInMount, seen map[string]struct{}, prefix string) error {
	if mount.src == "" || mount.isDir || !isYAMLPath(mount.src) {
		return nil
	}
	key := mount.src + "=>" + mount.dst
	if _, ok := seen[key]; ok {
		return nil
	}
	seen[key] = struct{}{}

	refs, err := collectYAMLIncludeRefs(mount.src)
	if err != nil {
		return fmt.Errorf("%s.in include scan %s: %w", prefix, mount.dst, err)
	}
	for _, ref := range refs {
		include, err := parseMountedIncludeRef(mount.src, mount.dst, ref)
		if err != nil {
			return fmt.Errorf("%s.in include %s: %w", prefix, ref, err)
		}
		if include.localPath != "" {
			if _, err := os.Stat(include.localPath); err != nil {
				return fmt.Errorf("%s.in include %s: source %s: %w", prefix, ref, include.localPath, err)
			}
		}
		target, ok := findMountForRuntimePath(include.runtimePath, byDst)
		if !ok {
			return fmt.Errorf("%s.in include %s: target %s is not mounted by this step's in entries", prefix, ref, include.runtimePath)
		}
		if err := validateMountYAMLIncludes(target, byDst, seen, prefix); err != nil {
			return err
		}
	}
	return nil
}

func collectYAMLIncludeRefs(filePath string) ([]string, error) {
	data, err := os.ReadFile(filePath)
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
