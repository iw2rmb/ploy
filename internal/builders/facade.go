package builders

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// JavaOSVRequest mirrors the request subset used by internal build flow
type JavaOSVRequest struct {
	App       string
	MainClass string
	SrcDir    string
	GitSHA    string
	OutDir    string
	EnvVars   map[string]string
}

// The functions below provide lightweight, internal implementations to avoid
// importing api/* from internal/*. They create deterministic artifact outputs
// used by higher-level build and deployment logic.

func ensureDir(dir string) error { return os.MkdirAll(dir, 0o755) }

func touch(path string, content string) error {
	if err := ensureDir(filepath.Dir(path)); err != nil {
		return err
	}
	return os.WriteFile(path, []byte(content), 0o644)
}

func BuildUnikraft(app, lane, srcDir, sha, outDir string, envVars map[string]string) (string, error) {
	img := filepath.Join(outDir, "final.img")
	if err := touch(img, fmt.Sprintf("unikraft image for %s@%s", app, sha)); err != nil {
		return "", err
	}
	return img, nil
}

func BuildOSVJava(req JavaOSVRequest) (string, error) {
	img := filepath.Join(req.OutDir, "app-osv.img")
	if err := touch(img, fmt.Sprintf("osv jvm image %s %s", req.App, req.MainClass)); err != nil {
		return "", err
	}
	return img, nil
}

func BuildJail(app, srcDir, sha, outDir string, envVars map[string]string) (string, error) {
	img := filepath.Join(outDir, "jail.img")
	if err := touch(img, fmt.Sprintf("freebsd jail image for %s@%s", app, sha)); err != nil {
		return "", err
	}
	return img, nil
}

func BuildOCI(app, srcDir, tag string, envVars map[string]string) (string, error) {
	// Execute the OCI build script to ensure the image is built and pushed to the registry
	scriptPath := "/home/ploy/ploy/scripts/build/oci/build_oci.sh"
	if _, err := os.Stat(scriptPath); os.IsNotExist(err) {
		scriptPath = "./scripts/build/oci/build_oci.sh"
	}
	args := []string{"--app", app, "--src", srcDir, "--tag", tag}
	cmd := exec.Command(scriptPath, args...)
	// Include provided environment variables for the build context
	env := os.Environ()
	for k, v := range envVars {
		env = append(env, fmt.Sprintf("%s=%s", k, v))
	}
	cmd.Env = env
	out, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("oci build failed: %v: %s", err, string(out))
	}
	// Script prints the final image reference on the last line
	return strings.TrimSpace(string(out)), nil
}

func BuildVM(app, sha, outDir string, envVars map[string]string) (string, error) {
	img := filepath.Join(outDir, "vm.img")
	if err := touch(img, fmt.Sprintf("vm image for %s@%s", app, sha)); err != nil {
		return "", err
	}
	return img, nil
}
