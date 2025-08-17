package nomad

import (
	"os"
	"os/exec"
)

func Submit(jobPath string) error {
	cmd := exec.Command("nomad", "job", "run", jobPath)
	cmd.Stdout = os.Stdout; cmd.Stderr = os.Stderr
	return cmd.Run()
}
