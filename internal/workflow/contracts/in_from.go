package contracts

import (
	"fmt"
	"path"
	"strings"

	domaintypes "github.com/iw2rmb/ploy/internal/domain/types"
)

// InFromRef defines one cross-step source reference.
// Example:
//
//	from: extract-usage@mig://out/dependency-usage.nofilter.json
//	from: pre_gate://out/sbom.dependencies.txt
//	to:   /in/dependency-usage.nofilter.json
type InFromRef struct {
	From string `json:"from,omitempty" yaml:"from,omitempty"`
	To   string `json:"to,omitempty" yaml:"to,omitempty"`
}

// InFromURI is the parsed representation of an in_from `from` URI.
type InFromURI struct {
	SourceName string
	SourceType domaintypes.JobType
	OutPath    string
}

// ResolvedInFromRef is claim-time resolved input materialization metadata.
// SourceArtifactID points to the artifact bundle that contains SourceOutPath.
type ResolvedInFromRef struct {
	From             string `json:"from,omitempty"`
	To               string `json:"to,omitempty"`
	SourceStepName   string `json:"source_step_name,omitempty"`
	SourceOutPath    string `json:"source_out_path,omitempty"`
	SourceArtifactID string `json:"source_artifact_id,omitempty"`
}

// ParseInFromURI parses one of:
//   - <type>://out/<path>
//   - <name>@<type>://out/<path>
//   - <step-name>://out/<path> (legacy alias of <step-name>@mig://...)
func ParseInFromURI(raw string) (InFromURI, error) {
	s := strings.TrimSpace(raw)
	if s == "" {
		return InFromURI{}, fmt.Errorf("must not be empty")
	}

	idx := strings.Index(s, "://")
	if idx <= 0 {
		return InFromURI{}, fmt.Errorf("expected <type>://out/<path> or <name>@<type>://out/<path>")
	}
	selector := strings.TrimSpace(s[:idx])
	if selector == "" {
		return InFromURI{}, fmt.Errorf("selector is empty")
	}

	body := strings.TrimSpace(s[idx+3:])
	body = strings.TrimPrefix(body, "/")
	if !strings.HasPrefix(body, "out/") {
		return InFromURI{}, fmt.Errorf("path must start with out/")
	}

	cleaned := path.Clean("/" + body)
	if !strings.HasPrefix(cleaned, "/out/") || cleaned == "/out" {
		return InFromURI{}, fmt.Errorf("path must stay under /out")
	}
	if base := path.Base(cleaned); base == "." || base == "/" {
		return InFromURI{}, fmt.Errorf("path must reference a file")
	}

	sourceName := ""
	typeRaw := selector
	if strings.Contains(selector, "@") {
		parts := strings.Split(selector, "@")
		if len(parts) != 2 {
			return InFromURI{}, fmt.Errorf("selector must contain at most one @")
		}
		sourceName = strings.TrimSpace(parts[0])
		typeRaw = strings.TrimSpace(parts[1])
		if sourceName == "" {
			return InFromURI{}, fmt.Errorf("selector name is empty")
		}
		if typeRaw == "" {
			return InFromURI{}, fmt.Errorf("selector type is empty")
		}
	}

	sourceType := domaintypes.JobType(strings.TrimSpace(typeRaw))
	if err := sourceType.Validate(); err != nil {
		// Backward compatibility for legacy "<step-name>://out/..." references.
		if sourceName == "" {
			sourceName = strings.TrimSpace(typeRaw)
			sourceType = domaintypes.JobTypeMig
		} else {
			return InFromURI{}, fmt.Errorf("selector type %q is invalid: %w", strings.TrimSpace(typeRaw), err)
		}
	}

	return InFromURI{
		SourceName: sourceName,
		SourceType: sourceType,
		OutPath:    cleaned,
	}, nil
}

// NormalizeInFromTarget normalizes a destination to canonical /in/<path>.
// When raw is empty, defaults to /in/<basename(sourceOutPath)>.
func NormalizeInFromTarget(raw, sourceOutPath string) (string, error) {
	target := strings.TrimSpace(raw)
	if target == "" {
		base := path.Base(strings.TrimSpace(sourceOutPath))
		if base == "" || base == "." || base == "/" {
			return "", fmt.Errorf("cannot derive default target from source path")
		}
		return "/in/" + base, nil
	}

	cleaned := strings.TrimPrefix(target, "/")
	if strings.HasPrefix(cleaned, "in/") {
		cleaned = strings.TrimPrefix(cleaned, "in/")
	}
	cleaned = path.Clean(cleaned)
	if cleaned == "" || cleaned == "." {
		return "", fmt.Errorf("target is empty")
	}

	normalized := "/in/" + cleaned
	normalized = path.Clean(normalized)
	if !strings.HasPrefix(normalized, "/in/") || normalized == "/in" {
		return "", fmt.Errorf("target must stay under /in")
	}
	return normalized, nil
}

func validateInFromReferences(steps []MigStep) error {
	if len(steps) == 0 {
		return nil
	}

	hasInFrom := false
	requiresNamedMigStepLookup := false
	for i := range steps {
		if len(steps[i].InFrom) > 0 {
			hasInFrom = true
		}
		for j := range steps[i].InFrom {
			parsed, err := ParseInFromURI(steps[i].InFrom[j].From)
			if err != nil {
				continue
			}
			if parsed.SourceType == domaintypes.JobTypeMig && parsed.SourceName != "" {
				requiresNamedMigStepLookup = true
			}
		}
	}
	if !hasInFrom {
		return nil
	}

	stepNameToIndex := map[string]int{}
	if requiresNamedMigStepLookup {
		stepNameToIndex = make(map[string]int, len(steps))
		for i := range steps {
			name := strings.TrimSpace(steps[i].Name)
			if name == "" {
				continue
			}
			if prev, exists := stepNameToIndex[name]; exists {
				return fmt.Errorf("steps[%d].name: duplicate step name %q (already used at steps[%d])", i, name, prev)
			}
			stepNameToIndex[name] = i
		}
	}

	for i := range steps {
		step := steps[i]
		for j := range step.InFrom {
			ref := step.InFrom[j]

			parsed, err := ParseInFromURI(ref.From)
			if err != nil {
				return fmt.Errorf("steps[%d].in_from[%d].from: %w", i, j, err)
			}
			if _, err := NormalizeInFromTarget(ref.To, parsed.OutPath); err != nil {
				return fmt.Errorf("steps[%d].in_from[%d].to: %w", i, j, err)
			}

			if parsed.SourceName != "" && parsed.SourceType != domaintypes.JobTypeMig {
				return fmt.Errorf("steps[%d].in_from[%d].from: named selector is only supported for type %q", i, j, domaintypes.JobTypeMig)
			}
			if parsed.SourceType != domaintypes.JobTypeMig || parsed.SourceName == "" {
				continue
			}

			sourceIdx, ok := stepNameToIndex[parsed.SourceName]
			if !ok {
				return fmt.Errorf("steps[%d].in_from[%d].from: unknown step name %q", i, j, parsed.SourceName)
			}
			if sourceIdx >= i {
				return fmt.Errorf("steps[%d].in_from[%d].from: must reference an earlier step (source index %d, current index %d)", i, j, sourceIdx, i)
			}
		}
	}

	return nil
}
