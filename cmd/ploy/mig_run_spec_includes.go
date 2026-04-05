package main

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"gopkg.in/yaml.v3"
)

var errSpecIncludeCycle = errors.New("spec include cycle detected")

// composeSpecYAML resolves !include macros and deep merge keys (<<) in YAML specs.
// It returns composed YAML bytes ready for map decoding.
func composeSpecYAML(data []byte, sourcePath string) ([]byte, error) {
	rootNode, err := loadYAMLDocumentBytes(data, sourcePath)
	if err != nil {
		return nil, err
	}

	cache := map[string]*yaml.Node{
		sourcePath: rootNode,
	}
	if err := resolveIncludes(rootNode, sourcePath, cache, []string{sourcePath}); err != nil {
		return nil, err
	}
	if err := expandDeepMergeKeys(rootNode.Content[0]); err != nil {
		return nil, err
	}

	composed, err := yaml.Marshal(rootNode.Content[0])
	if err != nil {
		return nil, fmt.Errorf("encode composed spec %s: %w", sourcePath, err)
	}
	return composed, nil
}

func loadYAMLDocumentBytes(data []byte, sourcePath string) (*yaml.Node, error) {
	var root yaml.Node
	if err := yaml.Unmarshal(data, &root); err != nil {
		return nil, fmt.Errorf("decode spec %s: %w", sourcePath, err)
	}
	if root.Kind != yaml.DocumentNode || len(root.Content) == 0 {
		return nil, fmt.Errorf("decode spec %s: document root must be present", sourcePath)
	}
	return &root, nil
}

func resolveIncludes(node *yaml.Node, sourcePath string, cache map[string]*yaml.Node, stack []string) error {
	if node == nil {
		return nil
	}
	if node.Kind == yaml.AliasNode && node.Alias != nil {
		return resolveIncludes(node.Alias, sourcePath, cache, stack)
	}

	if node.Kind == yaml.ScalarNode && node.Tag == "!include" {
		targetFile, pointer, err := parseIncludeRef(sourcePath, node.Value)
		if err != nil {
			return err
		}

		targetID := targetFile
		if pointer != "" {
			targetID = targetFile + "#" + pointer
		}
		if cycle := includeCycle(stack, targetID); len(cycle) > 0 {
			return fmt.Errorf("%w: %s", errSpecIncludeCycle, strings.Join(cycle, " -> "))
		}

		targetRoot, err := loadYAMLFromCache(targetFile, cache)
		if err != nil {
			return err
		}
		selected, err := selectNode(targetRoot.Content[0], pointer, targetFile)
		if err != nil {
			return err
		}

		replacement := cloneNode(selected)
		if err := normalizeIncludedLocalPaths(replacement, targetFile); err != nil {
			return err
		}
		if err := resolveIncludes(replacement, targetFile, cache, append(stack, targetID)); err != nil {
			return err
		}
		replaceNode(node, replacement)
		return nil
	}

	for _, child := range node.Content {
		if err := resolveIncludes(child, sourcePath, cache, stack); err != nil {
			return err
		}
	}
	return nil
}

func loadYAMLFromCache(filePath string, cache map[string]*yaml.Node) (*yaml.Node, error) {
	if root, ok := cache[filePath]; ok {
		return root, nil
	}
	data, err := os.ReadFile(filePath)
	if err != nil {
		return nil, fmt.Errorf("read spec %s: %w", filePath, err)
	}
	root, err := loadYAMLDocumentBytes(data, filePath)
	if err != nil {
		return nil, err
	}
	cache[filePath] = root
	return root, nil
}

func parseIncludeRef(sourcePath string, raw string) (string, string, error) {
	value := strings.TrimSpace(raw)
	if value == "" {
		return "", "", fmt.Errorf("decode spec %s: !include path must not be empty", sourcePath)
	}

	pathPart := value
	pointer := ""
	if hash := strings.Index(value, "#"); hash >= 0 {
		pathPart = strings.TrimSpace(value[:hash])
		pointer = strings.TrimSpace(value[hash+1:])
	}
	if pathPart == "" {
		return "", "", fmt.Errorf("decode spec %s: !include path must not be empty", sourcePath)
	}

	resolvedPath := pathPart
	if !filepath.IsAbs(resolvedPath) {
		resolvedPath = filepath.Join(filepath.Dir(sourcePath), resolvedPath)
	}
	resolvedPath = filepath.Clean(resolvedPath)

	if pointer != "" && !strings.HasPrefix(pointer, "/") {
		return "", "", fmt.Errorf("decode spec %s: !include fragment must start with /", sourcePath)
	}
	return resolvedPath, pointer, nil
}

func selectNode(root *yaml.Node, pointer, sourcePath string) (*yaml.Node, error) {
	if pointer == "" {
		return root, nil
	}

	current := root
	parts := strings.Split(strings.TrimPrefix(pointer, "/"), "/")
	for _, rawPart := range parts {
		part := decodePointerPart(rawPart)
		next, err := pointerStep(current, part)
		if err != nil {
			return nil, fmt.Errorf("decode spec %s: !include fragment %q: %w", sourcePath, pointer, err)
		}
		current = next
	}
	return current, nil
}

func decodePointerPart(value string) string {
	replaced := strings.ReplaceAll(value, "~1", "/")
	return strings.ReplaceAll(replaced, "~0", "~")
}

func pointerStep(node *yaml.Node, token string) (*yaml.Node, error) {
	switch node.Kind {
	case yaml.MappingNode:
		for i := 0; i+1 < len(node.Content); i += 2 {
			key := node.Content[i]
			if key.Kind == yaml.ScalarNode && key.Value == token {
				return node.Content[i+1], nil
			}
		}
		return nil, fmt.Errorf("mapping key %q not found", token)
	case yaml.SequenceNode:
		index, err := strconv.Atoi(token)
		if err != nil {
			return nil, fmt.Errorf("sequence index %q is invalid", token)
		}
		if index < 0 || index >= len(node.Content) {
			return nil, fmt.Errorf("sequence index %d is out of range", index)
		}
		return node.Content[index], nil
	default:
		return nil, fmt.Errorf("cannot traverse node kind %d", node.Kind)
	}
}

func includeCycle(stack []string, target string) []string {
	index := -1
	for i := range stack {
		if stack[i] == target {
			index = i
			break
		}
	}
	if index < 0 {
		return nil
	}
	cycle := append([]string{}, stack[index:]...)
	cycle = append(cycle, target)
	return cycle
}

func expandDeepMergeKeys(node *yaml.Node) error {
	if node == nil {
		return nil
	}
	if node.Kind == yaml.AliasNode && node.Alias != nil {
		return expandDeepMergeKeys(node.Alias)
	}

	for _, child := range node.Content {
		if err := expandDeepMergeKeys(child); err != nil {
			return err
		}
	}

	if node.Kind != yaml.MappingNode {
		return nil
	}

	merged := &yaml.Node{Kind: yaml.MappingNode}

	for i := 0; i+1 < len(node.Content); i += 2 {
		keyNode := node.Content[i]
		valueNode := node.Content[i+1]
		if keyNode.Kind == yaml.ScalarNode && keyNode.Value == "<<" {
			sources, err := collectMergeSources(valueNode)
			if err != nil {
				return err
			}
			for _, src := range sources {
				if err := mergeMappingInto(merged, src); err != nil {
					return err
				}
			}
			continue
		}
		if err := setOrDeepMergeKey(merged, keyNode.Value, valueNode); err != nil {
			return err
		}
	}

	node.Content = merged.Content
	return nil
}

func collectMergeSources(valueNode *yaml.Node) ([]*yaml.Node, error) {
	valueNode = derefAlias(valueNode)
	switch valueNode.Kind {
	case yaml.MappingNode:
		return []*yaml.Node{valueNode}, nil
	case yaml.SequenceNode:
		out := make([]*yaml.Node, 0, len(valueNode.Content))
		for _, item := range valueNode.Content {
			item = derefAlias(item)
			if item.Kind != yaml.MappingNode {
				return nil, fmt.Errorf("decode spec: merge key expects mapping or sequence of mappings")
			}
			out = append(out, item)
		}
		return out, nil
	default:
		return nil, fmt.Errorf("decode spec: merge key expects mapping or sequence of mappings")
	}
}

func mergeMappingInto(dst *yaml.Node, src *yaml.Node) error {
	src = derefAlias(src)
	if src.Kind != yaml.MappingNode {
		return fmt.Errorf("decode spec: merge source must be a mapping")
	}
	for i := 0; i+1 < len(src.Content); i += 2 {
		if err := setOrDeepMergeKey(dst, src.Content[i].Value, src.Content[i+1]); err != nil {
			return err
		}
	}
	return nil
}

func setOrDeepMergeKey(dst *yaml.Node, key string, overlayValue *yaml.Node) error {
	idx := findMappingKeyIndex(dst, key)
	if idx < 0 {
		dst.Content = append(dst.Content, cloneNode(&yaml.Node{
			Kind:  yaml.ScalarNode,
			Tag:   "!!str",
			Value: key,
		}), cloneNode(overlayValue))
		return nil
	}

	existing := dst.Content[idx+1]
	baseMap := derefAlias(existing)
	overlayMap := derefAlias(overlayValue)
	if baseMap.Kind == yaml.MappingNode && overlayMap.Kind == yaml.MappingNode {
		mergedMap := cloneNode(baseMap)
		if err := mergeMappingInto(mergedMap, overlayMap); err != nil {
			return err
		}
		dst.Content[idx+1] = mergedMap
		return nil
	}
	dst.Content[idx+1] = cloneNode(overlayValue)
	return nil
}

func findMappingKeyIndex(node *yaml.Node, key string) int {
	if node == nil || node.Kind != yaml.MappingNode {
		return -1
	}
	for i := 0; i+1 < len(node.Content); i += 2 {
		k := node.Content[i]
		if k.Kind == yaml.ScalarNode && k.Value == key {
			return i
		}
	}
	return -1
}

func derefAlias(node *yaml.Node) *yaml.Node {
	if node != nil && node.Kind == yaml.AliasNode && node.Alias != nil {
		return node.Alias
	}
	return node
}

func normalizeIncludedLocalPaths(node *yaml.Node, sourcePath string) error {
	if node == nil {
		return nil
	}
	node = derefAlias(node)
	if node.Kind == yaml.SequenceNode {
		for _, child := range node.Content {
			if err := normalizeIncludedLocalPaths(child, sourcePath); err != nil {
				return err
			}
		}
		return nil
	}
	if node.Kind != yaml.MappingNode {
		return nil
	}

	for i := 0; i+1 < len(node.Content); i += 2 {
		key := node.Content[i]
		value := derefAlias(node.Content[i+1])
		if key.Kind != yaml.ScalarNode {
			if err := normalizeIncludedLocalPaths(value, sourcePath); err != nil {
				return err
			}
			continue
		}

		switch key.Value {
		case "ca":
			normalizeCAEntries(value, sourcePath)
		case "in":
			normalizeMountEntries(value, sourcePath, false)
		case "out":
			normalizeMountEntries(value, sourcePath, false)
		case "home":
			normalizeMountEntries(value, sourcePath, true)
		}

		if err := normalizeIncludedLocalPaths(value, sourcePath); err != nil {
			return err
		}
	}
	return nil
}

func normalizeCAEntries(node *yaml.Node, sourcePath string) {
	if node.Kind != yaml.SequenceNode {
		return
	}
	for _, entry := range node.Content {
		entry = derefAlias(entry)
		if entry.Kind != yaml.ScalarNode {
			continue
		}
		s := strings.TrimSpace(entry.Value)
		if shortHashPattern.MatchString(s) {
			continue
		}
		entry.Value = normalizeLocalSourcePath(sourcePath, s)
	}
}

func normalizeMountEntries(node *yaml.Node, sourcePath string, isHome bool) {
	if node.Kind != yaml.SequenceNode {
		return
	}
	for _, entry := range node.Content {
		entry = derefAlias(entry)
		if entry.Kind != yaml.ScalarNode {
			continue
		}
		s := strings.TrimSpace(entry.Value)
		if s == "" {
			continue
		}

		body := s
		suffix := ""
		if isHome && strings.HasSuffix(body, ":ro") {
			body = strings.TrimSuffix(body, ":ro")
			suffix = ":ro"
		}
		if idx := strings.Index(body, ":"); idx > 0 && shortHashPattern.MatchString(body[:idx]) {
			continue
		}
		idx := strings.LastIndex(body, ":")
		if idx <= 0 {
			continue
		}

		src := strings.TrimSpace(body[:idx])
		dst := body[idx:]
		entry.Value = normalizeLocalSourcePath(sourcePath, src) + dst + suffix
	}
}

func normalizeLocalSourcePath(sourcePath, raw string) string {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" || strings.HasPrefix(trimmed, "$") || strings.HasPrefix(trimmed, "~/") || filepath.IsAbs(trimmed) {
		return trimmed
	}
	if strings.Contains(trimmed, "\n") || strings.Contains(trimmed, "\r") {
		return trimmed
	}
	return filepath.Clean(filepath.Join(filepath.Dir(sourcePath), trimmed))
}

func replaceNode(dst *yaml.Node, src *yaml.Node) {
	dst.Kind = src.Kind
	dst.Style = src.Style
	dst.Tag = src.Tag
	dst.Value = src.Value
	dst.Anchor = src.Anchor
	dst.Alias = src.Alias
	dst.Content = src.Content
	dst.HeadComment = src.HeadComment
	dst.LineComment = src.LineComment
	dst.FootComment = src.FootComment
}

func cloneNode(node *yaml.Node) *yaml.Node {
	if node == nil {
		return nil
	}
	cloned := *node
	if len(node.Content) > 0 {
		cloned.Content = make([]*yaml.Node, len(node.Content))
		for i := range node.Content {
			cloned.Content[i] = cloneNode(node.Content[i])
		}
	}
	if node.Alias != nil {
		cloned.Alias = cloneNode(node.Alias)
	}
	return &cloned
}
