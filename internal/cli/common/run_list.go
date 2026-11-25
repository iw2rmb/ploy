package common

import (
	"strings"

	domaintypes "github.com/iw2rmb/ploy/internal/domain/types"
)

func FormatRunListSpec(summary domaintypes.RunSummary) string {
	name := strings.TrimSpace(summary.SpecName)
	if qualified := qualifiedRunSpec(summary, name); qualified != "" {
		return qualified
	}
	if !summary.SpecID.IsZero() {
		return summary.SpecID.String()
	}
	return "-"
}

func FormatRunListSpecDetail(summary domaintypes.RunSummary) string {
	name := strings.TrimSpace(summary.SpecName)
	if name == "" {
		name = summary.SpecID.String()
	}
	if qualified := qualifiedRunSpec(summary, name); qualified != "" {
		return qualified
	}
	if name == "" {
		return "-"
	}
	return name
}

func qualifiedRunSpec(summary domaintypes.RunSummary, name string) string {
	domain := strings.Trim(strings.TrimSpace(summary.SpecSourceDomain), "/")
	repo := strings.Trim(strings.TrimSpace(summary.SpecSourceRepo), "/")
	if domain == "" || repo == "" || name == "" {
		return ""
	}
	return domain + "/" + repo + ":" + name
}

func FormatRunListRepo(repoURL, sourceCommitSHA string) string {
	repo := formatRepoNamespace(repoURL)
	sha := strings.TrimSpace(sourceCommitSHA)
	if len(sha) > 8 {
		sha = sha[:8]
	}
	if repo == "" {
		return "-"
	}
	if sha == "" {
		return repo
	}
	return repo + ":" + sha
}

func formatRepoNamespace(repoURL string) string {
	display := domaintypes.NormalizeRepoURLSchemless(repoURL)
	display = strings.Trim(display, "/")
	if display == "" {
		return ""
	}
	parts := strings.Split(display, "/")
	if len(parts) <= 2 {
		return display
	}
	return strings.Join(parts[1:], "/")
}
