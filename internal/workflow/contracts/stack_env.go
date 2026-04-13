package contracts

import "strings"

const (
	PLOYStackLanguageEnv = "PLOY_STACK_LANGUAGE"
	PLOYStackToolEnv     = "PLOY_STACK_TOOL"
	PLOYStackReleaseEnv  = "PLOY_STACK_RELEASE"
)

// NormalizeStackExpectation returns a trimmed copy of expect.
// Nil or fully-empty values normalize to nil.
func NormalizeStackExpectation(expect *StackExpectation) *StackExpectation {
	if expect == nil {
		return nil
	}
	out := &StackExpectation{
		Language: strings.TrimSpace(expect.Language),
		Tool:     strings.TrimSpace(expect.Tool),
		Release:  strings.TrimSpace(expect.Release),
	}
	if out.Language == "" && out.Tool == "" && out.Release == "" {
		return nil
	}
	return out
}
