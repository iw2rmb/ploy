//go:build windows

package handlers

import (
	"os/exec"
	"time"
)

func configureProcessGroupCancel(cmd *exec.Cmd) {
	cmd.WaitDelay = 2 * time.Second
}
