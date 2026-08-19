package gitexec

import (
	"bytes"
	"context"
	"os"
	"os/exec"
)

// Request describes one non-interactive Git process invocation.
type Request struct {
	Dir   string
	Env   []string
	Args  []string
	Stdin []byte
}

// Result contains the process output streams captured separately.
type Result struct {
	Stdout []byte
	Stderr []byte
}

// Runner executes Git requests.
type Runner interface {
	Run(context.Context, Request) (Result, error)
}

// ExecRunner executes Git through os/exec.
type ExecRunner struct{}

// Run executes req and returns the raw process error with any captured output.
func (ExecRunner) Run(ctx context.Context, req Request) (Result, error) {
	cmd := exec.CommandContext(ctx, "git", req.Args...)
	cmd.Dir = req.Dir
	cmd.Env = append(os.Environ(), req.Env...)
	cmd.Env = append(cmd.Env, "GIT_TERMINAL_PROMPT=0", "GIT_ASKPASS=echo")
	if req.Stdin != nil {
		cmd.Stdin = bytes.NewReader(req.Stdin)
	}

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	configureProcessGroupCancel(cmd)
	err := cmd.Run()
	return Result{Stdout: stdout.Bytes(), Stderr: stderr.Bytes()}, err
}

// Execute runs req with ExecRunner.
func Execute(ctx context.Context, req Request) (Result, error) {
	return (ExecRunner{}).Run(ctx, req)
}
