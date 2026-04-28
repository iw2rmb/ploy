// hydra.go defines Hydra canonical stored-entry parsers and validators for
// the envs/ca/in/out/home contract fields.
//
// Canonical stored-entry formats:
//   - in:   "shortHash:dst"      where dst starts with /in/
//   - out:  "shortHash:dst"      where dst starts with /out/
//   - home: "shortHash:dst{:ro}" where dst is $HOME-relative (no leading /)
//   - ca:   "shortHash"          plain hex hash
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
	"re_gate":   true,
	"post_gate": true,
	"mig":       true,
	"heal":      true,
}

// ValidCAConfigSections lists the known section names for config ca entries.
var ValidCAConfigSections = map[string]bool{
	"pre_gate":  true,
	"re_gate":   true,
	"post_gate": true,
	"mig":       true,
	"heal":      true,
}

// ValidateHydraSection returns an error if section is not a known Hydra section.
func ValidateHydraSection(section string) error {
	if !ValidHydraSections[section] {
		return fmt.Errorf("invalid hydra section %q (must be one of: heal, mig, post_gate, pre_gate, re_gate)", section)
	}
	return nil
}

// ValidateCAConfigSection returns an error if section is not valid for config ca.
func ValidateCAConfigSection(section string) error {
	if !ValidCAConfigSections[section] {
		return fmt.Errorf("invalid config ca section %q (must be one of: heal, mig, post_gate, pre_gate, re_gate)", section)
	}
	return nil
}

// shortHashPattern matches a valid shortHash: 7–64 lowercase hex characters.
var shortHashPattern = regexp.MustCompile(`^[0-9a-f]{7,64}$`)

// ParsedStoredEntry holds the result of parsing a canonical stored entry.
type ParsedStoredEntry struct {
	Hash     string
	Dst      string
	ReadOnly bool
}

// ParseStoredInEntry parses a canonical `in` entry: "shortHash:dst".
// dst must be absolute and start with "/in/".
func ParseStoredInEntry(s string) (ParsedStoredEntry, error) {
	hash, dst, err := splitHashDst(s)
	if err != nil {
		return ParsedStoredEntry{}, fmt.Errorf("in entry %q: %w", s, err)
	}
	dst = path.Clean(dst)
	if !strings.HasPrefix(dst, "/in/") {
		return ParsedStoredEntry{}, fmt.Errorf("in entry %q: destination must start with /in/", s)
	}
	if err := guardPathTraversal(dst); err != nil {
		return ParsedStoredEntry{}, fmt.Errorf("in entry %q: %w", s, err)
	}
	return ParsedStoredEntry{Hash: hash, Dst: dst, ReadOnly: true}, nil
}

// ParseStoredOutEntry parses a canonical `out` entry: "shortHash:dst".
// dst must be absolute and start with "/out/".
func ParseStoredOutEntry(s string) (ParsedStoredEntry, error) {
	hash, dst, err := splitHashDst(s)
	if err != nil {
		return ParsedStoredEntry{}, fmt.Errorf("out entry %q: %w", s, err)
	}
	dst = path.Clean(dst)
	if !strings.HasPrefix(dst, "/out/") {
		return ParsedStoredEntry{}, fmt.Errorf("out entry %q: destination must start with /out/", s)
	}
	if err := guardPathTraversal(dst); err != nil {
		return ParsedStoredEntry{}, fmt.Errorf("out entry %q: %w", s, err)
	}
	return ParsedStoredEntry{Hash: hash, Dst: dst, ReadOnly: false}, nil
}

// ParseStoredHomeEntry parses a canonical `home` entry: "shortHash:dst{:ro}".
// dst must be relative (no leading /) and must not traverse above $HOME.
// Mode defaults to rw; optional :ro suffix forces read-only.
func ParseStoredHomeEntry(s string) (ParsedStoredEntry, error) {
	// Check for trailing :ro suffix.
	readOnly := false
	body := s
	if strings.HasSuffix(s, ":ro") {
		readOnly = true
		body = s[:len(s)-3]
	}
	hash, dst, err := splitHashDst(body)
	if err != nil {
		return ParsedStoredEntry{}, fmt.Errorf("home entry %q: %w", s, err)
	}
	dst = path.Clean(dst)
	if dst == "" || dst == "." {
		return ParsedStoredEntry{}, fmt.Errorf("home entry %q: destination required", s)
	}
	if strings.HasPrefix(dst, "/") {
		return ParsedStoredEntry{}, fmt.Errorf("home entry %q: destination must be relative (no leading /)", s)
	}
	if err := guardPathTraversal(dst); err != nil {
		return ParsedStoredEntry{}, fmt.Errorf("home entry %q: %w", s, err)
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

// ParseStoredCAEntry validates a canonical `ca` entry: a plain shortHash.
func ParseStoredCAEntry(s string) (string, error) {
	trimmed := strings.TrimSpace(s)
	if !shortHashPattern.MatchString(trimmed) {
		return "", fmt.Errorf("ca entry %q: must be a valid short hash (7-64 hex chars)", s)
	}
	return trimmed, nil
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

// ValidateHydraInEntries validates a slice of canonical `in` entries.
func ValidateHydraInEntries(entries []string, prefix string) error {
	seen := make(map[string]struct{}, len(entries))
	for i, entry := range entries {
		parsed, err := ParseStoredInEntry(entry)
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

// ValidateHydraOutEntries validates a slice of canonical `out` entries.
func ValidateHydraOutEntries(entries []string, prefix string) error {
	seen := make(map[string]struct{}, len(entries))
	for i, entry := range entries {
		parsed, err := ParseStoredOutEntry(entry)
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

// ValidateHydraHomeEntries validates a slice of canonical `home` entries.
func ValidateHydraHomeEntries(entries []string, prefix string) error {
	seen := make(map[string]struct{}, len(entries))
	for i, entry := range entries {
		parsed, err := ParseStoredHomeEntry(entry)
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

// ValidateHydraCAEntries validates a slice of canonical `ca` entries.
func ValidateHydraCAEntries(entries []string, prefix string) error {
	seen := make(map[string]struct{}, len(entries))
	for i, entry := range entries {
		hash, err := ParseStoredCAEntry(entry)
		if err != nil {
			return fmt.Errorf("%s[%d]: %w", prefix, i, err)
		}
		if _, dup := seen[hash]; dup {
			return fmt.Errorf("%s[%d]: duplicate hash %q", prefix, i, hash)
		}
		seen[hash] = struct{}{}
	}
	return nil
}

// validateHydraFields validates the Hydra fields (ca, in, out, home) on a
// container spec (step or heal action).
func validateHydraFields(ca, in, out, home []string, prefix string) error {
	if err := ValidateHydraCAEntries(ca, prefix+".ca"); err != nil {
		return err
	}
	if err := ValidateHydraInEntries(in, prefix+".in"); err != nil {
		return err
	}
	if err := ValidateHydraOutEntries(out, prefix+".out"); err != nil {
		return err
	}
	if err := ValidateHydraHomeEntries(home, prefix+".home"); err != nil {
		return err
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
	if !shortHashPattern.MatchString(hash) {
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
