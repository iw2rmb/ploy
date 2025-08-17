package builders

import (
	"fmt"
	"os/exec"
)

func BuildJail(app, srcDir, sha, outDir string) (string, error) {
	args := []string{"--app", app, "--src", srcDir, "--sha", sha, "--out-dir", outDir}
	cmd := exec.Command("./build/jail/build_jail.sh", args...)
	b, err := cmd.CombinedOutput()
	if err != nil { return "", fmt.Errorf("jail build failed: %v: %s", err, string(b)) }
	return string(bytesTrimSpace(b)), nil
}
