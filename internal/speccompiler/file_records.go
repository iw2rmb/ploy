// The compiler resolves authoring-form in/out/home/tmp entries into canonical
// shortHash:dst form suitable for contract validation and run persistence.
//
// Authoring input formats:
//   - in:   src:dst          (right-biased split, dst treated as /in-relative)
//   - out:  src:dst          (right-biased split, dst treated as /out-relative)
//   - home: src:dst{:ro}     (right-biased split, dst is $HOME-relative)
//   - tmp:  src:dst          (right-biased split, dst treated as /tmp-relative or /tmp absolute)
//
// After compilation, entries are rewritten to:
//   - in:   shortHash:/in/dst
//   - out:  shortHash:/out/dst
//   - home: shortHash:dst{:ro}
//   - tmp:  shortHash:/tmp/dst
package speccompiler

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"maps"
	"path"
	"regexp"
	"strings"
)

// shortHashPattern matches a valid shortHash: 7–64 lowercase hex characters.
// Keep this aligned with the stored file-record contract.
var shortHashPattern = regexp.MustCompile(`^[0-9a-f]{7,64}$`)

func IsArchiveShortHash(value string) bool {
	return shortHashPattern.MatchString(value)
}

// shortHashLen is the fixed prefix length for canonical short hashes (12 hex chars).
const shortHashLen = 12

// parseAuthoringInEntry parses an authoring `in` entry: "src:dst".
// Uses right-biased splitting (last colon). dst is normalized under /in.
func parseAuthoringInEntry(s string) (src, dst string, err error) {
	src, dst, err = splitRightBiasedColon(s)
	if err != nil {
		return "", "", fmt.Errorf("in entry %q: %w", s, err)
	}
	dst, err = normalizeAuthoringDestination(dst, "/in/")
	if err != nil {
		return "", "", fmt.Errorf("in entry %q: %w", s, err)
	}
	if err := guardAuthoringTraversal(dst); err != nil {
		return "", "", fmt.Errorf("in entry %q: %w", s, err)
	}
	return src, dst, nil
}

// parseAuthoringOutEntry parses an authoring `out` entry: "src:dst".
// Uses right-biased splitting (last colon). dst is normalized under /out.
func parseAuthoringOutEntry(s string) (src, dst string, err error) {
	src, dst, err = splitRightBiasedColon(s)
	if err != nil {
		return "", "", fmt.Errorf("out entry %q: %w", s, err)
	}
	dst, err = normalizeAuthoringDestination(dst, "/out/")
	if err != nil {
		return "", "", fmt.Errorf("out entry %q: %w", s, err)
	}
	if err := guardAuthoringTraversal(dst); err != nil {
		return "", "", fmt.Errorf("out entry %q: %w", s, err)
	}
	return src, dst, nil
}

// parseAuthoringHomeEntry parses an authoring `home` entry: "src:dst{:ro}".
// Uses right-biased splitting. dst must be relative (no leading /).
func parseAuthoringHomeEntry(s string) (src, dst string, readOnly bool, err error) {
	body := s
	if strings.HasSuffix(s, ":ro") {
		readOnly = true
		body = s[:len(s)-3]
	}
	src, dst, err = splitRightBiasedColon(body)
	if err != nil {
		return "", "", false, fmt.Errorf("home entry %q: %w", s, err)
	}
	dst = strings.TrimSpace(dst)
	if dst == "" {
		return "", "", false, fmt.Errorf("home entry %q: destination required", s)
	}
	dst = strings.TrimPrefix(dst, "/")
	dst = path.Clean(dst)
	if dst == "" || dst == "." {
		return "", "", false, fmt.Errorf("home entry %q: destination required", s)
	}
	if err := guardAuthoringTraversal(dst); err != nil {
		return "", "", false, fmt.Errorf("home entry %q: %w", s, err)
	}
	return src, dst, readOnly, nil
}

// parseAuthoringTmpEntry parses an authoring `tmp` entry: "src:dst".
// Uses right-biased splitting. Absolute destinations must stay under /tmp;
// relative destinations are normalized under /tmp.
func parseAuthoringTmpEntry(s string) (src, dst string, err error) {
	src, rawDst, err := splitRightBiasedColon(s)
	if err != nil {
		return "", "", fmt.Errorf("tmp entry %q: %w", s, err)
	}
	dst, err = normalizeAuthoringTmpDestination(rawDst)
	if err != nil {
		return "", "", fmt.Errorf("tmp entry %q: %w", s, err)
	}
	if err := guardAuthoringTraversal(dst); err != nil {
		return "", "", fmt.Errorf("tmp entry %q: %w", s, err)
	}
	return src, dst, nil
}

func normalizeAuthoringDestination(dst, root string) (string, error) {
	trimmed := strings.TrimSpace(dst)
	if trimmed == "" {
		return "", fmt.Errorf("destination required")
	}

	relative := trimmed
	relative = strings.TrimPrefix(relative, "/")
	relative = path.Clean(relative)
	if relative == "" || relative == "." {
		return "", fmt.Errorf("destination required")
	}

	return root + relative, nil
}

func normalizeAuthoringTmpDestination(dst string) (string, error) {
	trimmed := strings.TrimSpace(dst)
	if trimmed == "" {
		return "", fmt.Errorf("destination required")
	}
	if strings.HasPrefix(trimmed, "/") {
		cleaned := path.Clean(trimmed)
		if !strings.HasPrefix(cleaned, "/tmp/") {
			return "", fmt.Errorf("destination must start with /tmp/")
		}
		return cleaned, nil
	}
	relative := path.Clean(trimmed)
	if relative == "" || relative == "." {
		return "", fmt.Errorf("destination required")
	}
	return "/tmp/" + relative, nil
}

// splitRightBiasedColon splits at the last colon, returning (left, right).
func splitRightBiasedColon(s string) (left, right string, err error) {
	idx := strings.LastIndex(s, ":")
	if idx < 0 {
		return "", "", fmt.Errorf("expected src:dst format")
	}
	left = s[:idx]
	right = s[idx+1:]
	if strings.TrimSpace(left) == "" {
		return "", "", fmt.Errorf("source is empty")
	}
	if strings.TrimSpace(right) == "" {
		return "", "", fmt.Errorf("destination is empty")
	}
	return left, right, nil
}

// guardAuthoringTraversal rejects paths containing ".." components.
func guardAuthoringTraversal(p string) error {
	for _, part := range strings.Split(p, "/") {
		if part == ".." {
			return fmt.Errorf("path traversal not allowed: %q", p)
		}
	}
	return nil
}

// compileHydraRecordsInPlace walks all container blocks in the spec and compiles
// authoring-form in/out/home/tmp entries into canonical shortHash:dst form.
// Returns nil immediately when no authoring entries are present.
func (c *Compiler) compileHydraRecordsInPlace(ctx context.Context, spec map[string]any, specBaseDir string) error {
	type blockRef struct {
		block  map[string]any
		prefix string
	}
	var blocks []blockRef

	if steps, ok := spec["steps"].([]any); ok {
		for i, s := range steps {
			if step, ok := s.(map[string]any); ok {
				if hasAuthoringEntries(step) {
					blocks = append(blocks, blockRef{step, fmt.Sprintf("steps[%d]", i)})
				}
			}
		}
	}

	if len(blocks) == 0 {
		return nil
	}

	if c.bundles == nil {
		return fmt.Errorf("file-backed records found but no bundle persistence available")
	}

	// Cache each CID for the duration of one compilation pass.
	seen := make(map[string]string)
	// Accumulates shortHash → bundleID for runtime materialization.
	// Seed from any existing bundle_map so that already-canonical entries
	// retain their mappings when the spec mixes canonical and authoring forms.
	bundleMap := make(map[string]string)
	switch existing := spec["bundle_map"].(type) {
	case map[string]string:
		maps.Copy(bundleMap, existing)
	case map[string]any:
		for k, v := range existing {
			if s, ok := v.(string); ok {
				bundleMap[k] = s
			}
		}
	}
	for _, ref := range blocks {
		if err := c.compileHydraBlock(ctx, ref.block, ref.prefix, specBaseDir, seen, bundleMap); err != nil {
			return err
		}
	}
	if len(bundleMap) > 0 {
		spec["bundle_map"] = bundleMap
	}
	return nil
}

// hasAuthoringEntries checks whether a block contains any non-canonical entries
// that require compilation.
func hasAuthoringEntries(block map[string]any) bool {
	for _, key := range []string{"in", "out", "home", "tmp"} {
		entries, ok := block[key].([]any)
		if !ok {
			continue
		}
		for _, e := range entries {
			s, ok := e.(string)
			if !ok {
				continue
			}
			if !isAlreadyCanonical(key, s) {
				return true
			}
		}
	}
	return false
}

// isAlreadyCanonical checks if an entry is already in canonical stored form.
func isAlreadyCanonical(field, s string) bool {
	// For in/out/home, check if the first segment before : is a short hash.
	idx := strings.Index(s, ":")
	if idx <= 0 {
		return false
	}
	return shortHashPattern.MatchString(s[:idx])
}

// compileHydraBlock compiles authoring entries in a single container block.
func (c *Compiler) compileHydraBlock(ctx context.Context, block map[string]any, prefix, specBaseDir string, seen map[string]string, bundleMap map[string]string) error {
	if err := c.compileInEntries(ctx, block, prefix, specBaseDir, seen, bundleMap); err != nil {
		return err
	}
	if err := c.compileOutEntries(ctx, block, prefix, specBaseDir, seen, bundleMap); err != nil {
		return err
	}
	if err := c.compileHomeEntries(ctx, block, prefix, specBaseDir, seen, bundleMap); err != nil {
		return err
	}
	return c.compileTmpEntries(ctx, block, prefix, specBaseDir, seen, bundleMap)
}

func (c *Compiler) compileInEntries(ctx context.Context, block map[string]any, prefix, specBaseDir string, seen map[string]string, bundleMap map[string]string) error {
	entries, ok := block["in"].([]any)
	if !ok || len(entries) == 0 {
		return nil
	}
	compiled := make([]any, len(entries))
	for i, e := range entries {
		s, ok := e.(string)
		if !ok {
			return fmt.Errorf("%s.in[%d]: expected string, got %T", prefix, i, e)
		}
		idx := strings.Index(s, ":")
		if idx > 0 && shortHashPattern.MatchString(s[:idx]) {
			compiled[i] = s
			continue
		}
		src, dst, err := parseAuthoringInEntry(s)
		if err != nil {
			return fmt.Errorf("%s.in[%d]: %w", prefix, i, err)
		}
		hash, err := c.compileFileRecord(ctx, src, specBaseDir, seen, bundleMap)
		if err != nil {
			return fmt.Errorf("%s.in[%d]: %w", prefix, i, err)
		}
		compiled[i] = hash + ":" + dst
	}
	block["in"] = compiled
	return nil
}

func (c *Compiler) compileOutEntries(ctx context.Context, block map[string]any, prefix, specBaseDir string, seen map[string]string, bundleMap map[string]string) error {
	entries, ok := block["out"].([]any)
	if !ok || len(entries) == 0 {
		return nil
	}
	compiled := make([]any, len(entries))
	for i, e := range entries {
		s, ok := e.(string)
		if !ok {
			return fmt.Errorf("%s.out[%d]: expected string, got %T", prefix, i, e)
		}
		idx := strings.Index(s, ":")
		if idx > 0 && shortHashPattern.MatchString(s[:idx]) {
			compiled[i] = s
			continue
		}
		src, dst, err := parseAuthoringOutEntry(s)
		if err != nil {
			return fmt.Errorf("%s.out[%d]: %w", prefix, i, err)
		}
		hash, err := c.compileFileRecord(ctx, src, specBaseDir, seen, bundleMap)
		if err != nil {
			return fmt.Errorf("%s.out[%d]: %w", prefix, i, err)
		}
		compiled[i] = hash + ":" + dst
	}
	block["out"] = compiled
	return nil
}

func (c *Compiler) compileHomeEntries(ctx context.Context, block map[string]any, prefix, specBaseDir string, seen map[string]string, bundleMap map[string]string) error {
	entries, ok := block["home"].([]any)
	if !ok || len(entries) == 0 {
		return nil
	}
	compiled := make([]any, len(entries))
	for i, e := range entries {
		s, ok := e.(string)
		if !ok {
			return fmt.Errorf("%s.home[%d]: expected string, got %T", prefix, i, e)
		}
		// Check if already canonical: strip optional :ro, then check first segment.
		body := s
		if strings.HasSuffix(s, ":ro") {
			body = s[:len(s)-3]
		}
		idx := strings.Index(body, ":")
		if idx > 0 && shortHashPattern.MatchString(body[:idx]) {
			compiled[i] = s
			continue
		}
		src, dst, readOnly, err := parseAuthoringHomeEntry(s)
		if err != nil {
			return fmt.Errorf("%s.home[%d]: %w", prefix, i, err)
		}
		hash, err := c.compileFileRecord(ctx, src, specBaseDir, seen, bundleMap)
		if err != nil {
			return fmt.Errorf("%s.home[%d]: %w", prefix, i, err)
		}
		canonical := hash + ":" + dst
		if readOnly {
			canonical += ":ro"
		}
		compiled[i] = canonical
	}
	block["home"] = compiled
	return nil
}

func (c *Compiler) compileTmpEntries(ctx context.Context, block map[string]any, prefix, specBaseDir string, seen map[string]string, bundleMap map[string]string) error {
	entries, ok := block["tmp"].([]any)
	if !ok || len(entries) == 0 {
		return nil
	}
	compiled := make([]any, len(entries))
	for i, e := range entries {
		s, ok := e.(string)
		if !ok {
			return fmt.Errorf("%s.tmp[%d]: expected string, got %T", prefix, i, e)
		}
		idx := strings.Index(s, ":")
		if idx > 0 && shortHashPattern.MatchString(s[:idx]) {
			compiled[i] = s
			continue
		}
		src, dst, err := parseAuthoringTmpEntry(s)
		if err != nil {
			return fmt.Errorf("%s.tmp[%d]: %w", prefix, i, err)
		}
		hash, err := c.compileFileRecord(ctx, src, specBaseDir, seen, bundleMap)
		if err != nil {
			return fmt.Errorf("%s.tmp[%d]: %w", prefix, i, err)
		}
		compiled[i] = hash + ":" + dst
	}
	block["tmp"] = compiled
	return nil
}

// compileFileRecord resolves a source path, builds a deterministic archive,
// persists the archive through the caller's bundle adapter, and returns the
// short hash. The seen map caches CIDs already persisted during this pass.
// The bundleMap accumulates shortHash → bundleID mappings for runtime
// materialization.
func (c *Compiler) compileFileRecord(ctx context.Context, srcPath, specBaseDir string, seen map[string]string, bundleMap map[string]string) (string, error) {
	resolved, err := c.resolvePath(srcPath, specBaseDir)
	if err != nil {
		return "", fmt.Errorf("resolve source: %w", err)
	}

	archiveBytes, err := c.buildSourceArchive(resolved)
	if err != nil {
		return "", fmt.Errorf("build archive: %w", err)
	}

	hash := ArchiveShortHash(archiveBytes)
	cid := BundleCID(archiveBytes)

	// Avoid a second persistence call for duplicate content in the same spec.
	if bundleID, ok := seen[cid]; ok {
		bundleMap[hash] = bundleID
		return hash, nil
	}

	bundleID, err := c.bundles.Ensure(ctx, cid, archiveBytes)
	if err != nil {
		return "", fmt.Errorf("persist bundle: %w", err)
	}

	seen[cid] = bundleID
	bundleMap[hash] = bundleID
	return hash, nil
}

// computeArchiveShortHash computes the SHA256 of data and returns the short hash prefix.
func ArchiveShortHash(data []byte) string {
	h := sha256.Sum256(data)
	return hex.EncodeToString(h[:])[:shortHashLen]
}
