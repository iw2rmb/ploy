package python

import (
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"strings"

	analysis "github.com/iw2rmb/ploy/api/analysis"
)

func (a *PylintAnalyzer) runPylint(ctx context.Context, rootPath string, files []string) ([]analysis.Issue, error) {
	args := []string{
		"--output-format", a.config.OutputFormat,
		"--jobs", fmt.Sprintf("%d", a.config.Jobs),
	}

	if a.config.RCFile != "" {
		args = append(args, "--rcfile", a.config.RCFile)
	}
	if len(a.config.DisableRules) > 0 {
		args = append(args, "--disable", strings.Join(a.config.DisableRules, ","))
	}
	if len(a.config.EnableRules) > 0 {
		args = append(args, "--enable", strings.Join(a.config.EnableRules, ","))
	}
	args = append(args, files...)

	cmd := exec.CommandContext(ctx, a.config.PylintPath, args...)
	cmd.Dir = rootPath

	output, err := cmd.Output()
	if err != nil {
		if ctx.Err() != nil {
			return nil, fmt.Errorf("pylint execution cancelled: %w", ctx.Err())
		}
		if exitErr, ok := err.(*exec.ExitError); ok && len(exitErr.Stderr) > 0 {
			a.logger.Warnf("Pylint stderr: %s", string(exitErr.Stderr))
		}
	}

	return a.parsePylintOutput(string(output)), nil
}

func (a *PylintAnalyzer) parsePylintOutput(output string) []analysis.Issue {
	issues := []analysis.Issue{}
	if output == "" {
		return issues
	}

	var messages []PylintMessage
	if err := json.Unmarshal([]byte(output), &messages); err != nil {
		a.logger.Warnf("Failed to parse Pylint JSON output: %v", err)
		return issues
	}

	for i, msg := range messages {
		issues = append(issues, analysis.Issue{
			ID:       fmt.Sprintf("pylint-%d", i),
			Severity: a.mapSeverity(msg.Type),
			Category: a.categorizeMessage(msg.MessageID),
			RuleName: msg.Symbol,
			Message:  msg.Message,
			File:     msg.Path,
			Line:     msg.Line,
			Column:   msg.Column,
		})
	}

	return issues
}

func (a *PylintAnalyzer) mapSeverity(pylintSeverity string) analysis.SeverityLevel {
	if mapped, ok := a.config.SeverityMapping[pylintSeverity]; ok {
		switch mapped {
		case "critical":
			return analysis.SeverityCritical
		case "high":
			return analysis.SeverityHigh
		case "medium":
			return analysis.SeverityMedium
		case "low":
			return analysis.SeverityLow
		case "info":
			return analysis.SeverityInfo
		}
	}

	switch strings.ToLower(pylintSeverity) {
	case "fatal":
		return analysis.SeverityCritical
	case "error":
		return analysis.SeverityHigh
	case "warning":
		return analysis.SeverityMedium
	case "convention", "refactor":
		return analysis.SeverityLow
	case "info", "information":
		return analysis.SeverityInfo
	default:
		return analysis.SeverityInfo
	}
}

func (a *PylintAnalyzer) categorizeMessage(messageID string) analysis.IssueCategory {
	if messageID == "" {
		return analysis.CategoryMaintenance
	}

	prefix := strings.ToUpper(string(messageID[0]))

	switch prefix {
	case "E", "F":
		return analysis.CategoryBug
	case "W":
		if strings.HasPrefix(messageID, "W06") {
			return analysis.CategoryDeprecation
		}
		return analysis.CategoryMaintenance
	case "C":
		return analysis.CategoryStyle
	case "R":
		return analysis.CategoryComplexity
	case "I":
		return analysis.CategoryStyle
	default:
		return analysis.CategoryMaintenance
	}
}
