package builders

import (
	"fmt"
	"os"
	"os/exec"
)

func BuildJail(app, srcDir, sha, outDir string, envVars map[string]string) (string, error) {
	args := []string{"--app", app, "--src", srcDir, "--sha", sha, "--out-dir", outDir}
	cmd := exec.Command("./build/jail/build_jail.sh", args...)
	
	// Add environment variables to the build process
	env := os.Environ()
	for k, v := range envVars {
		env = append(env, fmt.Sprintf("%s=%s", k, v))
	}
	cmd.Env = env
	
	b, err := cmd.CombinedOutput()
	if err != nil { return "", fmt.Errorf("jail build failed: %v: %s", err, string(b)) }
	return bytesTrimSpace(b), nil
}
