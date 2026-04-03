// mig_run_spec_hydra.go implements the Hydra file-record compiler for CLI spec processing.
//
// The compiler resolves authoring-form ca/in/out/home entries into canonical
// shortHash:dst form suitable for contract validation and server submission.
//
// Authoring input formats:
//   - ca:   source-path
//   - in:   src:dst          (right-biased split, dst must start with /in/)
//   - out:  src:dst          (right-biased split, dst must start with /out/)
//   - home: src:dst{:ro}     (right-biased split, dst is $HOME-relative)
//
// After compilation, entries are rewritten to:
//   - ca:   shortHash
//   - in:   shortHash:/in/dst
//   - out:  shortHash:/out/dst
//   - home: shortHash:dst{:ro}
package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"maps"
	"net/http"
	"net/url"
	"regexp"
	"strings"
)

// shortHashPattern matches a valid shortHash: 7–64 lowercase hex characters.
// Local copy of the contracts-level pattern for use in CLI compilation.
var shortHashPattern = regexp.MustCompile(`^[0-9a-f]{7,64}$`)

// shortHashLen is the fixed prefix length for canonical short hashes (12 hex chars).
const shortHashLen = 12

// parseAuthoringInEntry parses an authoring `in` entry: "src:dst".
// Uses right-biased splitting (last colon). dst must start with "/in/".
func parseAuthoringInEntry(s string) (src, dst string, err error) {
	src, dst, err = splitRightBiasedColon(s)
	if err != nil {
		return "", "", fmt.Errorf("in entry %q: %w", s, err)
	}
	if !strings.HasPrefix(dst, "/in/") {
		return "", "", fmt.Errorf("in entry %q: destination must start with /in/", s)
	}
	if err := guardAuthoringTraversal(dst); err != nil {
		return "", "", fmt.Errorf("in entry %q: %w", s, err)
	}
	return src, dst, nil
}

// parseAuthoringOutEntry parses an authoring `out` entry: "src:dst".
// Uses right-biased splitting (last colon). dst must start with "/out/".
func parseAuthoringOutEntry(s string) (src, dst string, err error) {
	src, dst, err = splitRightBiasedColon(s)
	if err != nil {
		return "", "", fmt.Errorf("out entry %q: %w", s, err)
	}
	if !strings.HasPrefix(dst, "/out/") {
		return "", "", fmt.Errorf("out entry %q: destination must start with /out/", s)
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
	if dst == "" {
		return "", "", false, fmt.Errorf("home entry %q: destination required", s)
	}
	if strings.HasPrefix(dst, "/") {
		return "", "", false, fmt.Errorf("home entry %q: destination must be relative (no leading /)", s)
	}
	if err := guardAuthoringTraversal(dst); err != nil {
		return "", "", false, fmt.Errorf("home entry %q: %w", s, err)
	}
	return src, dst, readOnly, nil
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
// authoring-form ca/in/out/home entries into canonical shortHash:dst form.
// Returns nil immediately when no authoring entries are present.
func compileHydraRecordsInPlace(ctx context.Context, base *url.URL, client *http.Client, spec map[string]any, specBaseDir string) error {
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

	if bg, ok := spec["build_gate"].(map[string]any); ok {
		if router, ok := bg["router"].(map[string]any); ok {
			if hasAuthoringEntries(router) {
				blocks = append(blocks, blockRef{router, "build_gate.router"})
			}
		}
		if healing, ok := bg["healing"].(map[string]any); ok {
			if byErrorKind, ok := healing["by_error_kind"].(map[string]any); ok {
				for errorKind, item := range byErrorKind {
					action, ok := item.(map[string]any)
					if !ok {
						continue
					}
					if hasAuthoringEntries(action) {
						blocks = append(blocks, blockRef{action, fmt.Sprintf("build_gate.healing.by_error_kind.%s", errorKind)})
					}
				}
			}
		}
	}

	if len(blocks) == 0 {
		return nil
	}

	if base == nil {
		return fmt.Errorf("file-backed records found but no server base URL available for upload")
	}
	if client == nil {
		return fmt.Errorf("file-backed records found but no HTTP client available for upload")
	}

	// In-process cache: CID → bundleID for content already uploaded/probed this pass.
	seen := make(map[string]string)
	// Accumulates shortHash → bundleID for runtime materialization.
	// Seed from any existing bundle_map so that already-canonical entries
	// retain their mappings when the spec mixes canonical and authoring forms.
	bundleMap := make(map[string]string)
	if existing, ok := spec["bundle_map"].(map[string]string); ok {
		maps.Copy(bundleMap, existing)
	}
	for _, ref := range blocks {
		if err := compileHydraBlock(ctx, base, client, ref.block, ref.prefix, specBaseDir, seen, bundleMap); err != nil {
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
	for _, key := range []string{"ca", "in", "out", "home"} {
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
	if field == "ca" {
		return shortHashPattern.MatchString(strings.TrimSpace(s))
	}
	// For in/out/home, check if the first segment before : is a short hash.
	idx := strings.Index(s, ":")
	if idx <= 0 {
		return false
	}
	return shortHashPattern.MatchString(s[:idx])
}

// compileHydraBlock compiles authoring entries in a single container block.
func compileHydraBlock(ctx context.Context, base *url.URL, client *http.Client, block map[string]any, prefix, specBaseDir string, seen map[string]string, bundleMap map[string]string) error {
	if err := compileCAEntries(ctx, base, client, block, prefix, specBaseDir, seen, bundleMap); err != nil {
		return err
	}
	if err := compileInEntries(ctx, base, client, block, prefix, specBaseDir, seen, bundleMap); err != nil {
		return err
	}
	if err := compileOutEntries(ctx, base, client, block, prefix, specBaseDir, seen, bundleMap); err != nil {
		return err
	}
	return compileHomeEntries(ctx, base, client, block, prefix, specBaseDir, seen, bundleMap)
}

func compileCAEntries(ctx context.Context, base *url.URL, client *http.Client, block map[string]any, prefix, specBaseDir string, seen map[string]string, bundleMap map[string]string) error {
	entries, ok := block["ca"].([]any)
	if !ok || len(entries) == 0 {
		return nil
	}
	compiled := make([]any, 0, len(entries))
	dedupSet := make(map[string]bool)
	for i, e := range entries {
		s, ok := e.(string)
		if !ok {
			return fmt.Errorf("%s.ca[%d]: expected string, got %T", prefix, i, e)
		}
		s = strings.TrimSpace(s)
		var hash string
		if shortHashPattern.MatchString(s) {
			hash = s
		} else {
			var err error
			hash, err = compileFileRecord(ctx, base, client, s, specBaseDir, seen, bundleMap)
			if err != nil {
				return fmt.Errorf("%s.ca[%d]: %w", prefix, i, err)
			}
		}
		if !dedupSet[hash] {
			dedupSet[hash] = true
			compiled = append(compiled, hash)
		}
	}
	block["ca"] = compiled
	return nil
}

func compileInEntries(ctx context.Context, base *url.URL, client *http.Client, block map[string]any, prefix, specBaseDir string, seen map[string]string, bundleMap map[string]string) error {
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
		hash, err := compileFileRecord(ctx, base, client, src, specBaseDir, seen, bundleMap)
		if err != nil {
			return fmt.Errorf("%s.in[%d]: %w", prefix, i, err)
		}
		compiled[i] = hash + ":" + dst
	}
	block["in"] = compiled
	return nil
}

func compileOutEntries(ctx context.Context, base *url.URL, client *http.Client, block map[string]any, prefix, specBaseDir string, seen map[string]string, bundleMap map[string]string) error {
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
		hash, err := compileFileRecord(ctx, base, client, src, specBaseDir, seen, bundleMap)
		if err != nil {
			return fmt.Errorf("%s.out[%d]: %w", prefix, i, err)
		}
		compiled[i] = hash + ":" + dst
	}
	block["out"] = compiled
	return nil
}

func compileHomeEntries(ctx context.Context, base *url.URL, client *http.Client, block map[string]any, prefix, specBaseDir string, seen map[string]string, bundleMap map[string]string) error {
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
		hash, err := compileFileRecord(ctx, base, client, src, specBaseDir, seen, bundleMap)
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

// compileFileRecord resolves a source path, builds a deterministic archive,
// probes the server for an existing bundle with the same CID, uploads only
// if missing, and returns the short hash. The seen map caches CIDs → bundleIDs
// that have already been verified or uploaded during this compilation pass.
// The bundleMap accumulates shortHash → bundleID mappings for runtime
// materialization.
func compileFileRecord(ctx context.Context, base *url.URL, client *http.Client, srcPath, specBaseDir string, seen map[string]string, bundleMap map[string]string) (string, error) {
	resolved, err := resolvePath(srcPath, specBaseDir)
	if err != nil {
		return "", fmt.Errorf("resolve source: %w", err)
	}

	archiveBytes, err := buildSourceArchive(resolved)
	if err != nil {
		return "", fmt.Errorf("build archive: %w", err)
	}

	hash := computeArchiveShortHash(archiveBytes)
	cid := computeSpecBundleCID(archiveBytes)

	// In-process dedup: skip probe+upload if same content already handled this pass.
	if bundleID, ok := seen[cid]; ok {
		bundleMap[hash] = bundleID
		return hash, nil
	}

	// Probe server for existing content by CID before uploading.
	probeBundleID, exists, err := probeSpecBundleByCID(ctx, base, client, cid)
	if err != nil {
		return "", fmt.Errorf("probe: %w", err)
	}

	var bundleID string
	if exists && probeBundleID != "" {
		bundleID = probeBundleID
	} else {
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

// computeArchiveShortHash computes the SHA256 of data and returns the short hash prefix.
func computeArchiveShortHash(data []byte) string {
	h := sha256.Sum256(data)
	return hex.EncodeToString(h[:])[:shortHashLen]
}
