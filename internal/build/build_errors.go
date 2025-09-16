package build

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"
	"strings"
)

// ExecOutput captures full stdout/stderr from a command execution.
type ExecOutput struct {
	Stdout string
	Stderr string
}

// RunCmd runs the provided command and returns captured stdout/stderr.
func RunCmd(ctx context.Context, cmd *exec.Cmd) (ExecOutput, error) {
	var out, errb bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &errb
	e := cmd.Run()
	return ExecOutput{Stdout: out.String(), Stderr: errb.String()}, e
}

// BuildError represents a build failure with details and raw outputs.
type BuildError struct {
	Type    string
	Message string
	Details string
	Stdout  string
	Stderr  string
}

func (e *BuildError) Error() string {
	return fmt.Sprintf("%s: %s", e.Type, e.Message)
}

// FormatBuildError renders a human-readable error including details and optional raw output.
// maxBytes caps the length of stdout/stderr sections to avoid oversized messages (0 = unlimited).
func FormatBuildError(e *BuildError, includeRaw bool, maxBytes int) string {
	if e == nil {
		return ""
	}
	var b strings.Builder
	b.WriteString(e.Error())
	if e.Details != "" {
		b.WriteString(": ")
		b.WriteString(e.Details)
	}
	if includeRaw {
		if e.Stderr != "" {
			b.WriteString("\nstderr:\n")
			s := e.Stderr
			if maxBytes > 0 && len(s) > maxBytes {
				s = s[:maxBytes] + "…"
			}
			b.WriteString(s)
		}
		if e.Stdout != "" {
			b.WriteString("\nstdout:\n")
			s := e.Stdout
			if maxBytes > 0 && len(s) > maxBytes {
				s = s[:maxBytes] + "…"
			}
			b.WriteString(s)
		}
	}
	return b.String()
}
