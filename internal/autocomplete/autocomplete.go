package autocomplete

import (
	"fmt"
	"os"
	"path/filepath"
)

const goModuleFile = "go." + "mo" + "d"

// moduleRoot returns the repository root by walking up until it finds the Go module file.
func moduleRoot() string {
	dir, err := os.Getwd()
	if err != nil {
		panic(fmt.Sprintf("getwd: %v", err))
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, goModuleFile)); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}
	panic("could not find module root (go module file)")
}

// GenerateAll returns the checked-in shell completion scripts for the Ploy CLI.
// It reads the files from cmd/ploy/autocomplete and returns a map keyed by shell name.
func GenerateAll() map[string]string {
	root := moduleRoot()
	base := filepath.Join(root, "cmd", "ploy", "autocomplete")
	shells := []string{"bash", "zsh", "fish"}
	out := make(map[string]string, len(shells))

	for _, shell := range shells {
		filename := filepath.Join(base, fmt.Sprintf("ploy.%s", shell))
		data, err := os.ReadFile(filename)
		if err != nil {
			// Keep behavior simple for tooling: panic if artifacts are missing.
			panic(fmt.Sprintf("read completion file %s: %v", filename, err))
		}
		out[shell] = string(data)
	}

	return out
}
