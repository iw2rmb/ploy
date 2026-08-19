// hydra.go defines Hydra canonical stored-entry parsers and validators for
// the envs/in/out/home/tmp contract fields.
//
// Canonical stored-entry formats:
//   - in:   "shortHash:dst"      where dst starts with /in/
//   - out:  "shortHash:dst"      where dst starts with /out/
//   - home: "shortHash:dst{:ro}" where dst is $HOME-relative (no leading /)
//   - tmp:  "shortHash:dst"      where dst starts with /tmp/
//
// shortHash is a hex-only, colon-free prefix of the full content hash.
package contracts

import (
	"fmt"
	"path"
	"regexp"
	"strings"
)

// ValidHydraSections lists the known section names for typed Hydra overlays.
var ValidHydraSections = map[string]bool{
	"pre_gate":  true,
	"post_gate": true,
	"mig":       true,
}

// ValidateHydraSection returns an error if section is not a known Hydra section.
func ValidateHydraSection(section string) error {
	if !ValidHydraSections[section] {
		return fmt.Errorf("invalid hydra section %q (must be one of: mig, post_gate, pre_gate)", section)
	}
	return nil
}

// shortHashPattern matches a valid shortHash: 7–64 lowercase hex characters.
var shortHashPattern = regexp.MustCompile(`^[0-9a-f]{7,64}$`)

// IsHydraShortHash reports whether value is a canonical Hydra content-hash
// prefix.
func IsHydraShortHash(value string) bool {
	return shortHashPattern.MatchString(value)
}

// ParsedStoredEntry holds the result of parsing a canonical stored entry.
type ParsedStoredEntry struct {
	Hash     string
	Dst      string
	ReadOnly bool
}

// HydraFileKind identifies a Hydra file-record field.
type HydraFileKind string

const (
	HydraFileIn   HydraFileKind = "in"
	HydraFileOut  HydraFileKind = "out"
	HydraFileHome HydraFileKind = "home"
	HydraFileTmp  HydraFileKind = "tmp"
)

var hydraFileKinds = [...]HydraFileKind{
	HydraFileIn,
	HydraFileOut,
	HydraFileHome,
	HydraFileTmp,
}

// HydraFileKinds returns all file kinds in contract iteration order.
func HydraFileKinds() []HydraFileKind {
	kinds := make([]HydraFileKind, len(hydraFileKinds))
	copy(kinds, hydraFileKinds[:])
	return kinds
}

func (k HydraFileKind) String() string {
	return string(k)
}

// Entries returns the manifest entries owned by the file kind.
func (k HydraFileKind) Entries(manifest StepManifest) []string {
	switch k {
	case HydraFileIn:
		return manifest.In
	case HydraFileOut:
		return manifest.Out
	case HydraFileHome:
		return manifest.Home
	case HydraFileTmp:
		return manifest.Tmp
	default:
		return nil
	}
}

// ParseStoredEntry parses a canonical stored entry for the given file kind.
func ParseStoredEntry(kind HydraFileKind, s string) (ParsedStoredEntry, error) {
	readOnly := kind == HydraFileIn
	body := s
	if kind == HydraFileHome && strings.HasSuffix(s, ":ro") {
		readOnly = true
		body = strings.TrimSuffix(s, ":ro")
	}

	hash, dst, err := splitHashDst(body)
	if err != nil {
		return ParsedStoredEntry{}, fmt.Errorf("%s entry %q: %w", kind, s, err)
	}
	dst = path.Clean(dst)

	switch kind {
	case HydraFileIn, HydraFileOut, HydraFileTmp:
		root := "/" + kind.String() + "/"
		if !strings.HasPrefix(dst, root) {
			return ParsedStoredEntry{}, fmt.Errorf("%s entry %q: destination must start with %s", kind, s, root)
		}
	case HydraFileHome:
		if dst == "" || dst == "." {
			return ParsedStoredEntry{}, fmt.Errorf("home entry %q: destination required", s)
		}
		if strings.HasPrefix(dst, "/") {
			return ParsedStoredEntry{}, fmt.Errorf("home entry %q: destination must be relative (no leading /)", s)
		}
	default:
		return ParsedStoredEntry{}, fmt.Errorf("invalid Hydra file kind %q", kind)
	}

	if err := guardPathTraversal(dst); err != nil {
		return ParsedStoredEntry{}, fmt.Errorf("%s entry %q: %w", kind, s, err)
	}
	return ParsedStoredEntry{Hash: hash, Dst: dst, ReadOnly: readOnly}, nil
}

// CanonicalHomeEntry reconstructs the canonical stored home entry string
// from parsed fields: "hash:dst" or "hash:dst:ro".
func (p ParsedStoredEntry) CanonicalHomeEntry() string {
	s := p.Hash + ":" + p.Dst
	if p.ReadOnly {
		s += ":ro"
	}
	return s
}

// ValidateHomeDestination validates a home destination path without requiring
// a full canonical entry. The destination must be relative, non-empty, cleaned,
// and free of path traversal.
func ValidateHomeDestination(dst string) error {
	cleaned := path.Clean(dst)
	if cleaned == "" || cleaned == "." {
		return fmt.Errorf("home destination %q: destination required", dst)
	}
	if strings.HasPrefix(cleaned, "/") {
		return fmt.Errorf("home destination %q: must be relative (no leading /)", dst)
	}
	if err := guardPathTraversal(cleaned); err != nil {
		return fmt.Errorf("home destination %q: %w", dst, err)
	}
	return nil
}

// ValidateHydraEntries validates stored entries and rejects duplicate destinations.
func ValidateHydraEntries(kind HydraFileKind, entries []string, prefix string) error {
	seen := make(map[string]struct{}, len(entries))
	for i, entry := range entries {
		parsed, err := ParseStoredEntry(kind, entry)
		if err != nil {
			return fmt.Errorf("%s[%d]: %w", prefix, i, err)
		}
		if _, dup := seen[parsed.Dst]; dup {
			return fmt.Errorf("%s[%d]: duplicate destination %q", prefix, i, parsed.Dst)
		}
		seen[parsed.Dst] = struct{}{}
	}
	return nil
}

// validateHydraFields validates the Hydra fields (in, out, home, tmp) on a
// container spec.
func validateHydraFields(in, out, home, tmp []string, prefix string) error {
	entries := map[HydraFileKind][]string{
		HydraFileIn: in, HydraFileOut: out, HydraFileHome: home, HydraFileTmp: tmp,
	}
	for _, kind := range HydraFileKinds() {
		if err := ValidateHydraEntries(kind, entries[kind], prefix+"."+kind.String()); err != nil {
			return err
		}
	}
	return nil
}

// splitHashDst splits "shortHash:dst" at the first colon.
func splitHashDst(s string) (hash, dst string, err error) {
	idx := strings.Index(s, ":")
	if idx < 0 {
		return "", "", fmt.Errorf("expected format shortHash:dst")
	}
	hash = s[:idx]
	dst = s[idx+1:]
	if !IsHydraShortHash(hash) {
		return "", "", fmt.Errorf("invalid short hash %q (must be 7-64 hex chars)", hash)
	}
	if strings.TrimSpace(dst) == "" {
		return "", "", fmt.Errorf("destination required")
	}
	return hash, dst, nil
}

// guardPathTraversal rejects paths containing ".." components.
func guardPathTraversal(p string) error {
	for _, part := range strings.Split(p, "/") {
		if part == ".." {
			return fmt.Errorf("path traversal not allowed: %q", p)
		}
	}
	return nil
}
