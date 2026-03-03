package contracts

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"regexp"
	"strings"
)

const (
	ORWRecipeGroupEnv     = "RECIPE_GROUP"
	ORWRecipeArtifactEnv  = "RECIPE_ARTIFACT"
	ORWRecipeVersionEnv   = "RECIPE_VERSION"
	ORWRecipeClassnameEnv = "RECIPE_CLASSNAME"

	ORWReposEnv             = "ORW_REPOS"
	ORWRepoUsernameEnv      = "ORW_REPO_USERNAME"
	ORWRepoPasswordEnv      = "ORW_REPO_PASSWORD"
	ORWActiveRecipesEnv     = "ORW_ACTIVE_RECIPES"
	ORWFailOnUnsupportedEnv = "ORW_FAIL_ON_UNSUPPORTED"
)

const (
	ORWCLIReportFileName   = "report.json"
	ORWCLITransformLogName = "transform.log"
)

var orwCLIReasonPattern = regexp.MustCompile(`^[a-z0-9][a-z0-9-]{0,127}$`)

// ORWCLIErrorKind defines deterministic failure classes from orw-cli runtime.
type ORWCLIErrorKind string

const (
	ORWCLIErrorKindInput       ORWCLIErrorKind = "input"
	ORWCLIErrorKindResolution  ORWCLIErrorKind = "resolution"
	ORWCLIErrorKindExecution   ORWCLIErrorKind = "execution"
	ORWCLIErrorKindUnsupported ORWCLIErrorKind = "unsupported"
	ORWCLIErrorKindInternal    ORWCLIErrorKind = "internal"
)

const (
	ORWCLIReasonTypeAttributionUnavailable = "type-attribution-unavailable"
)

// Valid reports whether the error kind is recognized.
func (k ORWCLIErrorKind) Valid() bool {
	switch k {
	case ORWCLIErrorKindInput,
		ORWCLIErrorKindResolution,
		ORWCLIErrorKindExecution,
		ORWCLIErrorKindUnsupported,
		ORWCLIErrorKindInternal:
		return true
	default:
		return false
	}
}

// ORWCLIRecipeCoordinates defines recipe artifact coordinates.
type ORWCLIRecipeCoordinates struct {
	Group     string
	Artifact  string
	Version   string
	Classname string
}

// Validate ensures all required fields are present.
func (c ORWCLIRecipeCoordinates) Validate() error {
	if strings.TrimSpace(c.Group) == "" {
		return fmt.Errorf("%s is required", ORWRecipeGroupEnv)
	}
	if strings.TrimSpace(c.Artifact) == "" {
		return fmt.Errorf("%s is required", ORWRecipeArtifactEnv)
	}
	if strings.TrimSpace(c.Version) == "" {
		return fmt.Errorf("%s is required", ORWRecipeVersionEnv)
	}
	if strings.TrimSpace(c.Classname) == "" {
		return fmt.Errorf("%s is required", ORWRecipeClassnameEnv)
	}
	return nil
}

// Coords returns Maven coordinates in group:artifact:version form.
func (c ORWCLIRecipeCoordinates) Coords() string {
	return fmt.Sprintf("%s:%s:%s", c.Group, c.Artifact, c.Version)
}

// ORWCLIInput captures normalized runtime inputs derived from env variables.
type ORWCLIInput struct {
	Recipe            ORWCLIRecipeCoordinates
	Repositories      []string
	RepoUsername      string
	RepoPassword      string
	ActiveRecipes     []string
	FailOnUnsupported bool
}

// ParseORWCLIInputFromEnv parses and validates the ORW runtime env contract.
func ParseORWCLIInputFromEnv(env map[string]string) (ORWCLIInput, error) {
	lookup := func(key string) string {
		return strings.TrimSpace(env[key])
	}

	in := ORWCLIInput{
		Recipe: ORWCLIRecipeCoordinates{
			Group:     lookup(ORWRecipeGroupEnv),
			Artifact:  lookup(ORWRecipeArtifactEnv),
			Version:   lookup(ORWRecipeVersionEnv),
			Classname: lookup(ORWRecipeClassnameEnv),
		},
		RepoUsername: lookup(ORWRepoUsernameEnv),
		RepoPassword: lookup(ORWRepoPasswordEnv),
	}

	if err := in.Recipe.Validate(); err != nil {
		return ORWCLIInput{}, err
	}

	var err error
	in.Repositories = parseCommaList(lookup(ORWReposEnv))
	in.ActiveRecipes = parseCommaList(lookup(ORWActiveRecipesEnv))
	in.FailOnUnsupported, err = parseBoolWithDefault(lookup(ORWFailOnUnsupportedEnv), true)
	if err != nil {
		return ORWCLIInput{}, fmt.Errorf("%s: %w", ORWFailOnUnsupportedEnv, err)
	}

	if (in.RepoUsername == "") != (in.RepoPassword == "") {
		return ORWCLIInput{}, fmt.Errorf("%s and %s must be provided together", ORWRepoUsernameEnv, ORWRepoPasswordEnv)
	}

	return in, nil
}

// ORWCLIReport defines the runtime output written to /out/report.json.
type ORWCLIReport struct {
	Success   bool            `json:"success"`
	ErrorKind ORWCLIErrorKind `json:"error_kind,omitempty"`
	Reason    string          `json:"reason,omitempty"`
	Message   string          `json:"message,omitempty"`
}

// Validate ensures report.json follows the deterministic ORW contract.
func (r ORWCLIReport) Validate() error {
	r.Reason = strings.TrimSpace(r.Reason)
	r.Message = strings.TrimSpace(r.Message)

	if r.Success {
		if r.ErrorKind != "" {
			return fmt.Errorf("error_kind must be empty when success=true")
		}
		if r.Reason != "" {
			return fmt.Errorf("reason must be empty when success=true")
		}
		return nil
	}

	if !r.ErrorKind.Valid() {
		return fmt.Errorf("error_kind invalid: %q", r.ErrorKind)
	}
	if r.Message == "" {
		return fmt.Errorf("message is required when success=false")
	}
	if strings.ContainsAny(r.Message, "\n\r") {
		return fmt.Errorf("message must be single-line")
	}

	if r.Reason != "" {
		if !orwCLIReasonPattern.MatchString(r.Reason) {
			return fmt.Errorf("reason invalid: %q", r.Reason)
		}
	}
	if r.ErrorKind == ORWCLIErrorKindUnsupported {
		if r.Reason == "" {
			return fmt.Errorf("reason is required when error_kind=%q", ORWCLIErrorKindUnsupported)
		}
		if r.Reason != ORWCLIReasonTypeAttributionUnavailable {
			return fmt.Errorf("reason invalid for error_kind=%q: %q", ORWCLIErrorKindUnsupported, r.Reason)
		}
	}

	return nil
}

// ParseORWCLIReport decodes report.json using strict schema validation.
func ParseORWCLIReport(data []byte) (ORWCLIReport, error) {
	if len(bytes.TrimSpace(data)) == 0 {
		return ORWCLIReport{}, fmt.Errorf("report is empty")
	}

	var raw struct {
		Success   *bool  `json:"success"`
		ErrorKind string `json:"error_kind,omitempty"`
		Reason    string `json:"reason,omitempty"`
		Message   string `json:"message,omitempty"`
	}

	dec := json.NewDecoder(bytes.NewReader(data))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&raw); err != nil {
		return ORWCLIReport{}, fmt.Errorf("decode report: %w", err)
	}
	if err := dec.Decode(&struct{}{}); err != io.EOF {
		return ORWCLIReport{}, fmt.Errorf("decode report: unexpected trailing JSON")
	}
	if raw.Success == nil {
		return ORWCLIReport{}, fmt.Errorf("success is required")
	}

	report := ORWCLIReport{
		Success:   *raw.Success,
		ErrorKind: ORWCLIErrorKind(strings.TrimSpace(raw.ErrorKind)),
		Reason:    strings.TrimSpace(raw.Reason),
		Message:   strings.TrimSpace(raw.Message),
	}
	if err := report.Validate(); err != nil {
		return ORWCLIReport{}, err
	}
	return report, nil
}

func parseCommaList(raw string) []string {
	if raw == "" {
		return nil
	}
	parts := strings.Split(raw, ",")
	out := make([]string, 0, len(parts))
	for _, part := range parts {
		trimmed := strings.TrimSpace(part)
		if trimmed == "" {
			continue
		}
		out = append(out, trimmed)
	}
	return out
}

func parseBoolWithDefault(raw string, defaultValue bool) (bool, error) {
	if raw == "" {
		return defaultValue, nil
	}
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "1", "true", "yes", "on":
		return true, nil
	case "0", "false", "no", "off":
		return false, nil
	default:
		return false, fmt.Errorf("invalid boolean %q", raw)
	}
}
