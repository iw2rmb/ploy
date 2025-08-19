package builders

import (
	"fmt"
	"os"
	"os/exec"
)

func BuildOCI(app, srcDir, tag string, envVars map[string]string) (string, error) {
	args := []string{"--app", app, "--src", srcDir, "--tag", tag}
	cmd := exec.Command("./build/oci/build_oci.sh", args...)
	
	// Add environment variables to the build process
	env := os.Environ()
	for k, v := range envVars {
		env = append(env, fmt.Sprintf("%s=%s", k, v))
	}
	cmd.Env = env
	
	b, err := cmd.CombinedOutput()
	if err != nil { return "", fmt.Errorf("oci build failed: %v: %s", err, string(b)) }
	return string(bytesTrimSpace(b)), nil
}
