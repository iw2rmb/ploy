package builders

import (
	"fmt"
	"os"
	"os/exec"
)

func BuildUnikraft(app, lane, srcDir, sha, outDir string, envVars map[string]string) (string, error) {
	args := []string{"--app", app, "--app-dir", srcDir, "--lane", lane, "--sha", sha, "--out-dir", outDir}
	cmd := exec.Command("./build/kraft/build_unikraft.sh", args...)
	
	// Add environment variables to the build process
	env := os.Environ()
	for k, v := range envVars {
		env = append(env, fmt.Sprintf("%s=%s", k, v))
	}
	cmd.Env = env
	
	b, err := cmd.CombinedOutput()
	if err != nil { return "", fmt.Errorf("unikraft build failed: %v: %s", err, string(b)) }
	return bytesTrimSpace(b), nil
}
