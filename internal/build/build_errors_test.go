package build

import (
	"context"
	"os/exec"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestBuildError_ErrorAndFormat(t *testing.T) {
	e := &BuildError{Type: "compile", Message: "failed", Details: "line 10", Stdout: "out", Stderr: "err"}

	s := e.Error()
	require.Equal(t, "compile: failed", s)

	msg := FormatBuildError(e, false, 0)
	require.True(t, strings.Contains(msg, "compile: failed: line 10"))
	require.False(t, strings.Contains(msg, "stderr:"))

	msg = FormatBuildError(e, true, 0)
	require.True(t, strings.Contains(msg, "stderr:"))
	require.True(t, strings.Contains(msg, "stdout:"))
	require.True(t, strings.Contains(msg, "err"))
	require.True(t, strings.Contains(msg, "out"))

	msg = FormatBuildError(e, true, 2)
	require.True(t, strings.Contains(msg, "er…"))
}

func TestRunCmd_CapturesStdoutAndStderr(t *testing.T) {
	ctx := context.Background()
	cmd := exec.CommandContext(ctx, "bash", "-lc", "echo out; echo err 1>&2")
	out, err := RunCmd(ctx, cmd)
	require.NoError(t, err)
	require.Equal(t, "out\n", out.Stdout)
	require.Equal(t, "err\n", out.Stderr)

	cmd = exec.CommandContext(ctx, "bash", "-lc", "echo fail 1>&2; exit 7")
	o, err := RunCmd(ctx, cmd)
	require.Error(t, err)
	require.Equal(t, "", o.Stdout)
	require.Equal(t, "fail\n", o.Stderr)
}
