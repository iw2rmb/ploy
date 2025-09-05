package builders

import (
	"fmt"
	"os"
	"os/exec"
	"strings"
)

func BuildUnikraft(app, lane, srcDir, sha, outDir string, envVars map[string]string) (string, error) {
	// Use absolute path to the build script in the ploy repository
	scriptPath := "/home/ploy/ploy/scripts/build/kraft/build_unikraft.sh"

	// Fall back to relative path if absolute doesn't exist (for local development)
	if _, err := os.Stat(scriptPath); os.IsNotExist(err) {
		scriptPath = "./scripts/build/kraft/build_unikraft.sh"
	}

	args := []string{"--app", app, "--app-dir", srcDir, "--lane", lane, "--sha", sha, "--out-dir", outDir}
	cmd := exec.Command(scriptPath, args...)

	// Add environment variables to the build process
	env := os.Environ()
	for k, v := range envVars {
		env = append(env, fmt.Sprintf("%s=%s", k, v))
	}
	cmd.Env = env

	b, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("unikraft build failed: %v: %s", err, string(b))
	}

	// Extract the artifact path from the output (should be the last line that looks like a file path)
	output := string(b)
	lines := strings.Split(output, "\n")
	for i := len(lines) - 1; i >= 0; i-- {
		line := strings.TrimSpace(lines[i])
		if line != "" && strings.Contains(line, ".img") && !strings.Contains(line, ":") {
			return line, nil
		}
	}

	return bytesTrimSpace(b), nil
}
