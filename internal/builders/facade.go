package builders

import (
	"fmt"
	"os"
	"path/filepath"
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
	// Return the tag to indicate the built image reference
	return tag, nil
}

func BuildVM(app, sha, outDir string, envVars map[string]string) (string, error) {
	img := filepath.Join(outDir, "vm.img")
	if err := touch(img, fmt.Sprintf("vm image for %s@%s", app, sha)); err != nil {
		return "", err
	}
	return img, nil
}
