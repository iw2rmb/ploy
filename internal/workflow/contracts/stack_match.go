package contracts

import "strings"

// StackFieldsMatch compares two (language, tool, release) tuples.
// Language and tool are compared case-insensitively after trimming whitespace.
// Release is compared after trimming only (case-sensitive).
// Empty "want" fields are treated as wildcards (always match).
func StackFieldsMatch(lang, tool, release, wantLang, wantTool, wantRelease string) bool {
	if wantLang != "" && strings.TrimSpace(strings.ToLower(lang)) != strings.TrimSpace(strings.ToLower(wantLang)) {
		return false
	}
	if wantTool != "" && strings.TrimSpace(strings.ToLower(tool)) != strings.TrimSpace(strings.ToLower(wantTool)) {
		return false
	}
	if wantRelease != "" && strings.TrimSpace(release) != strings.TrimSpace(wantRelease) {
		return false
	}
	return true
}
