package hook

import (
	"strings"
	"unicode"
)

// CompareVersions compares two dependency versions for a stack identity.
// Returns -1 when a < b, 0 when equal, and 1 when a > b.
func CompareVersions(lang, tool, a, b string) int {
	lang = strings.ToLower(strings.TrimSpace(lang))
	tool = strings.ToLower(strings.TrimSpace(tool))
	switch {
	case lang == "java" && (tool == "maven" || tool == "gradle"):
		return compareQualifierAware(a, b)
	default:
		return compareLoose(a, b)
	}
}

// MatchVersionConstraint checks whether version satisfies a single comparator
// expression (for example: ">=1.2.0", "<2", "1.0.0", "!=1.3.0").
func MatchVersionConstraint(lang, tool, version, constraint string) bool {
	constraint = strings.TrimSpace(constraint)
	if constraint == "" || constraint == "*" {
		return true
	}
	version = strings.TrimSpace(version)
	if version == "" {
		return false
	}

	op := "="
	target := constraint
	for _, candidate := range []string{">=", "<=", "!=", "==", ">", "<", "="} {
		if strings.HasPrefix(constraint, candidate) {
			op = candidate
			target = strings.TrimSpace(strings.TrimPrefix(constraint, candidate))
			break
		}
	}
	if target == "" {
		return false
	}

	cmp := CompareVersions(lang, tool, version, target)
	switch op {
	case ">":
		return cmp > 0
	case ">=":
		return cmp >= 0
	case "<":
		return cmp < 0
	case "<=":
		return cmp <= 0
	case "!=":
		return cmp != 0
	case "=", "==":
		return cmp == 0
	default:
		return false
	}
}

func compareQualifierAware(a, b string) int {
	at := tokenize(a)
	bt := tokenize(b)
	maxLen := len(at)
	if len(bt) > maxLen {
		maxLen = len(bt)
	}
	for i := 0; i < maxLen; i++ {
		if i >= len(at) {
			return -tailEffect(bt[i:])
		}
		if i >= len(bt) {
			return tailEffect(at[i:])
		}
		av := at[i]
		bv := bt[i]
		if av.kind == tokenNumber && bv.kind == tokenNumber {
			if cmp := compareNumberToken(av.text, bv.text); cmp != 0 {
				return cmp
			}
			continue
		}
		if av.kind == tokenNumber && bv.kind == tokenWord {
			return 1
		}
		if av.kind == tokenWord && bv.kind == tokenNumber {
			return -1
		}
		if av.kind == tokenWord && bv.kind == tokenWord {
			ar, aKnown := qualifierRank(av.text)
			br, bKnown := qualifierRank(bv.text)
			if aKnown && bKnown {
				if ar < br {
					return -1
				}
				if ar > br {
					return 1
				}
				continue
			}
			if av.text < bv.text {
				return -1
			}
			if av.text > bv.text {
				return 1
			}
		}
	}
	return 0
}

func tailEffect(tokens []versionToken) int {
	if len(tokens) == 0 {
		return 0
	}
	seenPositive := false
	for _, t := range tokens {
		switch t.kind {
		case tokenNumber:
			if t.text != "0" {
				seenPositive = true
			}
		case tokenWord:
			rank, known := qualifierRank(t.text)
			if known {
				if rank < 0 {
					return -1
				}
				if rank > 0 {
					seenPositive = true
				}
				continue
			}
			seenPositive = true
		}
	}
	if seenPositive {
		return 1
	}
	return 0
}

func qualifierRank(token string) (int, bool) {
	switch strings.ToLower(strings.TrimSpace(token)) {
	case "snapshot":
		return -5, true
	case "alpha", "a":
		return -4, true
	case "beta", "b":
		return -3, true
	case "milestone", "m":
		return -2, true
	case "cr", "rc":
		return -1, true
	case "ga", "final", "release":
		return 0, true
	case "sp":
		return 1, true
	default:
		return 0, false
	}
}

func compareLoose(a, b string) int {
	at := tokenize(a)
	bt := tokenize(b)
	maxLen := len(at)
	if len(bt) > maxLen {
		maxLen = len(bt)
	}
	for i := 0; i < maxLen; i++ {
		if i >= len(at) {
			return -1
		}
		if i >= len(bt) {
			return 1
		}
		av := at[i]
		bv := bt[i]
		if av.kind == tokenNumber && bv.kind == tokenNumber {
			if cmp := compareNumberToken(av.text, bv.text); cmp != 0 {
				return cmp
			}
			continue
		}
		if av.text < bv.text {
			return -1
		}
		if av.text > bv.text {
			return 1
		}
	}
	return 0
}

const (
	tokenNumber = iota + 1
	tokenWord
)

type versionToken struct {
	kind int
	text string
}

func tokenize(v string) []versionToken {
	v = strings.ToLower(strings.TrimSpace(v))
	if v == "" {
		return nil
	}

	tokens := make([]versionToken, 0)
	var current strings.Builder
	tokenKind := 0
	flush := func() {
		if current.Len() == 0 {
			return
		}
		text := current.String()
		if tokenKind == tokenNumber {
			text = trimLeadingZeros(text)
		}
		tokens = append(tokens, versionToken{kind: tokenKind, text: text})
		current.Reset()
		tokenKind = 0
	}

	for _, r := range v {
		switch {
		case unicode.IsDigit(r):
			if tokenKind != 0 && tokenKind != tokenNumber {
				flush()
			}
			tokenKind = tokenNumber
			current.WriteRune(r)
		case unicode.IsLetter(r):
			if tokenKind != 0 && tokenKind != tokenWord {
				flush()
			}
			tokenKind = tokenWord
			current.WriteRune(r)
		default:
			flush()
		}
	}
	flush()
	return tokens
}

func trimLeadingZeros(s string) string {
	trimmed := strings.TrimLeft(s, "0")
	if trimmed == "" {
		return "0"
	}
	return trimmed
}

func compareNumberToken(a, b string) int {
	if len(a) < len(b) {
		return -1
	}
	if len(a) > len(b) {
		return 1
	}
	if a < b {
		return -1
	}
	if a > b {
		return 1
	}
	return 0
}
