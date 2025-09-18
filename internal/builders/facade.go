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

func BuildOSVJava(req JavaOSVRequest) (string, error) {
	img := filepath.Join(req.OutDir, "app-osv.img")
	if err := touch(img, fmt.Sprintf("osv jvm image %s %s", req.App, req.MainClass)); err != nil {
		return "", err
	}
	return img, nil
}

func BuildOCI(app, srcDir, tag string, envVars map[string]string) (string, error) {
	// On Dev, API runs as a Nomad job without Docker. Building must be handled by a
	// separate builder job. For now, return the tag for downstream orchestration.
	// Push verification will detect missing tags and surface readable errors.
	if tag == "" {
		return "", fmt.Errorf("empty image tag")
	}
	return tag, nil
}

func BuildVM(app, sha, outDir string, envVars map[string]string) (string, error) {
	img := filepath.Join(outDir, "vm.img")
	if err := touch(img, fmt.Sprintf("vm image for %s@%s", app, sha)); err != nil {
		return "", err
	}
	return img, nil
}
