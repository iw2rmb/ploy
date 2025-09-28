package buildgate

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"
)

type StaticCheckRegistry struct {
	entries map[string]staticCheckAdapterEntry
}

type staticCheckAdapterEntry struct {
	adapter StaticCheckAdapter
	meta    StaticCheckAdapterMetadata
}

// NewStaticCheckRegistry constructs an empty static check registry.
func NewStaticCheckRegistry() *StaticCheckRegistry {
	return &StaticCheckRegistry{entries: make(map[string]staticCheckAdapterEntry)}
}

// Register installs an adapter for the provided language.
func (r *StaticCheckRegistry) Register(adapter StaticCheckAdapter) error {
	if r == nil {
		return ErrStaticCheckRegistryNil
	}
	if adapter == nil {
		return errors.New("buildgate: static check adapter is nil")
	}
	meta, err := sanitizeAdapterMetadata(adapter.Metadata())
	if err != nil {
		return fmt.Errorf("buildgate: invalid static check adapter metadata: %w", err)
	}
	if _, exists := r.entries[meta.Language]; exists {
		return fmt.Errorf("buildgate: static check adapter already registered for language %q", meta.Language)
	}
	r.entries[meta.Language] = staticCheckAdapterEntry{adapter: adapter, meta: meta}
	return nil
}

// Execute runs the configured adapters based on lane defaults and manifest overrides.
func (r *StaticCheckRegistry) Execute(ctx context.Context, spec StaticCheckSpec) ([]StaticCheckReport, error) {
	if r == nil {
		return nil, ErrStaticCheckRegistryNil
	}
	if ctx == nil {
		ctx = context.Background()
	}
	invocations := make(map[string]staticCheckInvocation)

	for language, cfg := range spec.LaneDefaults {
		normalized := normalizeLanguage(language)
		if normalized == "" {
			continue
		}
		entry, ok := r.entries[normalized]
		if !ok {
			return nil, fmt.Errorf("%w: %s", ErrStaticCheckAdapterNotFound, normalized)
		}
		severity, err := normalizeSeverityLevel(cfg.FailOnSeverity)
		if err != nil {
			return nil, fmt.Errorf("buildgate: lane %s severity: %w", normalized, err)
		}
		invocations[normalized] = staticCheckInvocation{
			entry:   entry,
			enabled: cfg.Enabled,
			failOn:  severity,
			options: copyOptions(cfg.Options),
		}
	}

	for language, override := range spec.Manifest.Languages {
		normalized := normalizeLanguage(language)
		if normalized == "" {
			continue
		}
		entry, ok := r.entries[normalized]
		if !ok {
			if override.Enabled != nil && !*override.Enabled {
				continue
			}
			if override.Enabled != nil && *override.Enabled {
				return nil, fmt.Errorf("%w: %s", ErrStaticCheckAdapterNotFound, normalized)
			}
			continue
		}
		invocation, exists := invocations[normalized]
		if !exists {
			invocation = staticCheckInvocation{
				entry:   entry,
				options: make(map[string]string),
			}
		}
		if override.Enabled != nil {
			invocation.enabled = *override.Enabled
		} else if !exists {
			// If the adapter was not part of the lane defaults and the manifest
			// did not explicitly enable it, skip by leaving enabled false.
			invocation.enabled = false
		}
		if override.FailOnSeverity != "" {
			level, err := parseSeverityLevel(override.FailOnSeverity)
			if err != nil {
				return nil, fmt.Errorf("buildgate: manifest %s severity: %w", normalized, err)
			}
			invocation.failOn = level
		}
		if len(override.Options) > 0 {
			if invocation.options == nil {
				invocation.options = make(map[string]string)
			}
			for k, v := range override.Options {
				invocation.options[strings.TrimSpace(k)] = v
			}
		}
		invocations[normalized] = invocation
	}

	skip := make(map[string]struct{})
	for _, language := range spec.SkipLanguages {
		normalized := normalizeLanguage(language)
		if normalized != "" {
			skip[normalized] = struct{}{}
		}
	}

	languages := make([]string, 0, len(invocations))
	for language, invocation := range invocations {
		if _, shouldSkip := skip[language]; shouldSkip {
			continue
		}
		if !invocation.enabled {
			continue
		}
		languages = append(languages, language)
	}
	sort.Strings(languages)

	reports := make([]StaticCheckReport, 0, len(languages))
	for _, language := range languages {
		invocation := invocations[language]
		entry := invocation.entry
		failOn := invocation.failOn
		if failOn == "" {
			failOn = entry.meta.DefaultSeverity
			if failOn == "" {
				failOn = SeverityError
			}
		}
		request := StaticCheckRequest{
			FailOnSeverity: failOn,
			Options:        copyOptions(invocation.options),
		}
		result, err := entry.adapter.Run(ctx, request)
		if err != nil {
			return nil, fmt.Errorf("run static check %s/%s: %w", entry.meta.Language, entry.meta.Tool, err)
		}
		report := StaticCheckReport{
			Language: entry.meta.Language,
			Tool:     entry.meta.Tool,
		}
		passed := true
		for _, failure := range result.Failures {
			normalizedFailure := StaticCheckFailure{
				RuleID:   strings.TrimSpace(failure.RuleID),
				File:     strings.TrimSpace(failure.File),
				Line:     clampNonNegative(failure.Line),
				Column:   clampNonNegative(failure.Column),
				Severity: normalizeSeverity(failure.Severity),
				Message:  strings.TrimSpace(failure.Message),
			}
			if severityGreaterOrEqual(SeverityLevel(normalizedFailure.Severity), failOn) {
				passed = false
			}
			report.Failures = append(report.Failures, normalizedFailure)
		}
		report.Passed = passed
		reports = append(reports, report)
	}

	return reports, nil
}

type staticCheckInvocation struct {
	entry   staticCheckAdapterEntry
	enabled bool
	failOn  SeverityLevel
	options map[string]string
}
