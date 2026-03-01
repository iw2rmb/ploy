package stackdetect

import (
	"regexp"
	"strings"
)

// canonicalizeVersion trims whitespace, applies the regex, and returns the
// first submatch group. Returns fallback when the regex does not match.
func canonicalizeVersion(v string, re *regexp.Regexp, fallback string) string {
	m := re.FindStringSubmatch(strings.TrimSpace(v))
	if m == nil {
		return fallback
	}
	return m[1]
}
