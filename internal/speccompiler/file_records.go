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
	"strings"

	"github.com/iw2rmb/ploy/internal/workflow/contracts"
)

// shortHashLen is the fixed prefix length for canonical short hashes (12 hex chars).
const shortHashLen = 12

type authoringFileEntry struct {
	src      string
	dst      string
	readOnly bool
}

// parseAuthoringEntry parses one authoring entry using the destination policy
// owned by its Hydra file kind.
func parseAuthoringEntry(kind contracts.HydraFileKind, s string) (authoringFileEntry, error) {
	body := s
	readOnly := false
	if kind == contracts.HydraFileHome && strings.HasSuffix(s, ":ro") {
		readOnly = true
		body = strings.TrimSuffix(s, ":ro")
	}
	src, dst, err := splitRightBiasedColon(body)
	if err != nil {
		return authoringFileEntry{}, fmt.Errorf("%s entry %q: %w", kind, s, err)
	}

	switch kind {
	case contracts.HydraFileIn, contracts.HydraFileOut:
		dst, err = normalizeAuthoringDestination(dst, "/"+kind.String()+"/")
	case contracts.HydraFileHome:
		dst = strings.TrimSpace(dst)
		if dst == "" {
			err = fmt.Errorf("destination required")
			break
		}
		dst = path.Clean(strings.TrimPrefix(dst, "/"))
		if dst == "" || dst == "." {
			err = fmt.Errorf("destination required")
		}
	case contracts.HydraFileTmp:
		dst, err = normalizeAuthoringTmpDestination(dst)
	default:
		err = fmt.Errorf("invalid Hydra file kind %q", kind)
	}
	if err != nil {
		return authoringFileEntry{}, fmt.Errorf("%s entry %q: %w", kind, s, err)
	}
	if err := guardAuthoringTraversal(dst); err != nil {
		return authoringFileEntry{}, fmt.Errorf("%s entry %q: %w", kind, s, err)
	}
	return authoringFileEntry{src: src, dst: dst, readOnly: readOnly}, nil
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
		hashFile := func(src string) (string, error) {
			return c.compileFileRecord(ctx, src, specBaseDir, seen, bundleMap)
		}
		if err := compileHydraBlock(ref.block, ref.prefix, hashFile); err != nil {
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
	for _, kind := range contracts.HydraFileKinds() {
		entries, ok := block[kind.String()].([]any)
		if !ok {
			continue
		}
		for _, e := range entries {
			s, ok := e.(string)
			if !ok {
				continue
			}
			if !isAlreadyCanonical(s) {
				return true
			}
		}
	}
	return false
}

// isAlreadyCanonical checks if an entry is already in canonical stored form.
func isAlreadyCanonical(s string) bool {
	idx := strings.Index(s, ":")
	if idx <= 0 {
		return false
	}
	return contracts.IsHydraShortHash(s[:idx])
}

type fileRecordHasher func(src string) (string, error)

// compileHydraBlock compiles all authoring entries in contract order.
func compileHydraBlock(block map[string]any, prefix string, hashFile fileRecordHasher) error {
	for _, kind := range contracts.HydraFileKinds() {
		field := kind.String()
		entries, ok := block[field].([]any)
		if !ok || len(entries) == 0 {
			continue
		}
		compiled := make([]any, len(entries))
		for i, raw := range entries {
			s, ok := raw.(string)
			if !ok {
				return fmt.Errorf("%s.%s[%d]: expected string, got %T", prefix, field, i, raw)
			}
			if isAlreadyCanonical(s) {
				compiled[i] = s
				continue
			}
			entry, err := parseAuthoringEntry(kind, s)
			if err != nil {
				return fmt.Errorf("%s.%s[%d]: %w", prefix, field, i, err)
			}
			hash, err := hashFile(entry.src)
			if err != nil {
				return fmt.Errorf("%s.%s[%d]: %w", prefix, field, i, err)
			}
			canonical := hash + ":" + entry.dst
			if entry.readOnly {
				canonical += ":ro"
			}
			compiled[i] = canonical
		}
		block[field] = compiled
	}
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
