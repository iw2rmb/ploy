package builders

import (
	"fmt"
	"os/exec"
)

func BuildVM(app, sha, outDir string) (string, error) {
	args := []string{"--app", app, "--sha", sha, "--out-dir", outDir}
	cmd := exec.Command("./build/packer/build_vm.sh", args...)
	b, err := cmd.CombinedOutput()
	if err != nil { return "", fmt.Errorf("vm build failed: %v: %s", err, string(b)) }
	return string(bytesTrimSpace(b)), nil
}
