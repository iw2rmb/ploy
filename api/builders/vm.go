package builders

import (
	"fmt"
	"os"
	"os/exec"
)

func BuildVM(app, sha, outDir string, envVars map[string]string) (string, error) {
	args := []string{"--app", app, "--sha", sha, "--out-dir", outDir}
	cmd := exec.Command("./scripts/build/packer/build_vm.sh", args...)
	
	// Add environment variables to the build process
	env := os.Environ()
	for k, v := range envVars {
		env = append(env, fmt.Sprintf("%s=%s", k, v))
	}
	cmd.Env = env
	
	b, err := cmd.CombinedOutput()
	if err != nil { return "", fmt.Errorf("vm build failed: %v: %s", err, string(b)) }
	return bytesTrimSpace(b), nil
}
