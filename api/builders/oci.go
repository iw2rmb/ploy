package builders

import (
	"fmt"
	"os"
	"os/exec"
)

func BuildOCI(app, srcDir, tag string, envVars map[string]string) (string, error) {
	// Use absolute path to the build script in the ploy repository
	scriptPath := "/home/ploy/ploy/scripts/build/oci/build_oci.sh"
	
	// Fall back to relative path if absolute doesn't exist (for local development)
	if _, err := os.Stat(scriptPath); os.IsNotExist(err) {
		scriptPath = "./scripts/build/oci/build_oci.sh"
	}
	
	args := []string{"--app", app, "--src", srcDir, "--tag", tag}
	cmd := exec.Command(scriptPath, args...)
	
	// Add environment variables to the build process
	env := os.Environ()
	for k, v := range envVars {
		env = append(env, fmt.Sprintf("%s=%s", k, v))
	}
	cmd.Env = env
	
	b, err := cmd.CombinedOutput()
	if err != nil { return "", fmt.Errorf("oci build failed: %v: %s", err, string(b)) }
	return bytesTrimSpace(b), nil
}
