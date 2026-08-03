package speccatalog

import (
	"fmt"
	"regexp"
	"strings"

	domaintypes "github.com/iw2rmb/ploy/internal/domain/types"
)

var selectorNameRE = regexp.MustCompile(`^[0-9a-z._-]+$`)

type selector struct {
	name   string
	domain string
	repo   string
}

type AmbiguousError struct {
	Selector string
	Choices  []Entry
}

type InvalidSelectorError struct {
	Selector string
}

func (e *InvalidSelectorError) Error() string {
	return fmt.Sprintf("invalid named spec selector: %s", e.Selector)
}

func (e *AmbiguousError) Error() string {
	choices := make([]string, 0, len(e.Choices))
	for _, choice := range e.Choices {
		choices = append(choices, choice.Source+":"+choice.Path)
	}
	return fmt.Sprintf("named spec selector %s is ambiguous: %s", e.Selector, strings.Join(choices, ", "))
}

func resolveEntries(entries []Entry, rawSelector string) (Entry, error) {
	parsed, err := parseSelector(rawSelector)
	if err != nil {
		return Entry{}, err
	}
	matches := make([]Entry, 0, 1)
	for _, entry := range entries {
		if entry.Name != parsed.name {
			continue
		}
		domain, repo := sourceParts(entry.Source)
		if parsed.domain != "" && domain != parsed.domain {
			continue
		}
		if parsed.repo != "" && repo != parsed.repo {
			continue
		}
		matches = append(matches, entry)
	}
	if len(matches) == 0 {
		return Entry{}, ErrNotFound
	}
	if len(matches) > 1 {
		sortEntries(matches)
		return Entry{}, &AmbiguousError{Selector: strings.TrimSpace(rawSelector), Choices: matches}
	}
	return matches[0], nil
}

func parseSelector(raw string) (selector, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" || strings.Contains(raw, "@") {
		return selector{}, &InvalidSelectorError{Selector: raw}
	}
	colon := strings.LastIndexByte(raw, ':')
	if colon < 0 {
		if !selectorNameRE.MatchString(raw) {
			return selector{}, &InvalidSelectorError{Selector: raw}
		}
		return selector{name: raw}, nil
	}

	qualifier := strings.Trim(strings.TrimSpace(raw[:colon]), "/")
	name := strings.TrimSpace(raw[colon+1:])
	parts := strings.Split(qualifier, "/")
	if qualifier == "" || !selectorNameRE.MatchString(name) || len(parts) < 2 {
		return selector{}, &InvalidSelectorError{Selector: raw}
	}
	for _, part := range parts {
		if part == "" {
			return selector{}, &InvalidSelectorError{Selector: raw}
		}
	}
	if len(parts) == 2 {
		return selector{name: name, repo: qualifier}, nil
	}
	return selector{name: name, domain: parts[0], repo: strings.Join(parts[1:], "/")}, nil
}

func sourceParts(source string) (string, string) {
	parts := strings.Split(strings.Trim(domaintypes.NormalizeRepoURLSchemless(source), "/"), "/")
	if len(parts) < 2 {
		return "", ""
	}
	return parts[0], strings.Join(parts[1:], "/")
}
