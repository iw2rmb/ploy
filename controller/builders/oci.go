package builders

import (
	"fmt"
	"os/exec"
)

func BuildOCI(app, srcDir, tag string) (string, error) {
	args := []string{"--app", app, "--src", srcDir, "--tag", tag}
	cmd := exec.Command("./build/oci/build_oci.sh", args...)
	b, err := cmd.CombinedOutput()
	if err != nil { return "", fmt.Errorf("oci build failed: %v: %s", err, string(b)) }
	return string(bytesTrimSpace(b)), nil
}
