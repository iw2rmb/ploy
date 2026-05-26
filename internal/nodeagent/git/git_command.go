package git

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"
)

// runGitCommand executes a git command in the specified directory with custom environment.
func runGitCommand(ctx context.Context, dir string, env []string, args ...string) error {
	cmd := exec.CommandContext(ctx, "git", args...)
	if dir != "" {
		cmd.Dir = dir
	}
	cmd.Env = append(os.Environ(), "GIT_TERMINAL_PROMPT=0", "GIT_ASKPASS=echo")
	cmd.Env = append(cmd.Env, env...)

	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("git %s failed: %w (output=%s)", strings.Join(args, " "), err, strings.TrimSpace(string(output)))
	}
	return nil
}
