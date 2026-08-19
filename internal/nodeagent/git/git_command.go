package git

import (
	"bytes"
	"context"
	"fmt"
	"strings"

	"github.com/iw2rmb/ploy/internal/gitexec"
)

// runGitCommand executes a git command in the specified directory with custom environment.
func runGitCommand(ctx context.Context, dir string, env []string, args ...string) error {
	result, err := gitexec.Execute(ctx, gitexec.Request{Dir: dir, Env: env, Args: args})
	if err != nil {
		output := bytes.Join([][]byte{result.Stdout, result.Stderr}, nil)
		return fmt.Errorf("git %s failed: %w (output=%s)", strings.Join(args, " "), err, strings.TrimSpace(string(output)))
	}
	return nil
}
