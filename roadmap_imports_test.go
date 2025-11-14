package ploy_test

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// TestRoadmap exercises ROADMAP.md:205 — verify import graph is acyclic
// without using repo-wide patterns (no './...'). It builds a small,
// explicit set of packages to surface any import cycles.
func TestRoadmap(t *testing.T) {
	t.Run("roadmap", func(t *testing.T) {
		modulePath, err := readModulePath("go.mod")
		if err != nil {
			t.Fatalf("read module path: %v", err)
		}

		// Explicit package list (no './...')
		pkgs := []string{
			filepath.ToSlash(filepath.Join(modulePath, "cmd/ploy")),
			filepath.ToSlash(filepath.Join(modulePath, "cmd/ployd")),
			filepath.ToSlash(filepath.Join(modulePath, "cmd/ployd-node")),
		}

		// Include key internal workflow packages (most cycle-prone)
		wfRoots := []string{
			"internal/workflow/aster",
			"internal/workflow/contracts",
			"internal/workflow/knowledgebase",
			"internal/workflow/manifests",
			"internal/workflow/mods/plan",
			"internal/workflow/runtime/step",
		}
		for _, rel := range wfRoots {
			pkgs = append(pkgs, filepath.ToSlash(filepath.Join(modulePath, rel)))
		}

		// Build each package individually; fail on any cycle indication.
		for _, pkg := range pkgs {
			pkg := pkg
			t.Run(fmt.Sprintf("build:%s", pkg), func(t *testing.T) {
				t.Parallel()
				ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
				defer cancel()
				cmd := exec.CommandContext(ctx, "go", "build", pkg)
				out, err := cmd.CombinedOutput()
				if err != nil {
					// Detect cycle-related errors explicitly; otherwise surface build error.
					if hasCycleMessage(string(out)) {
						t.Fatalf("import cycle detected while building %s:\n%s", pkg, string(out))
					}
					t.Fatalf("build failed for %s: %v\n%s", pkg, err, string(out))
				}
			})
		}
	})
}

func readModulePath(modFile string) (string, error) {
	f, err := os.Open(modFile)
	if err != nil {
		return "", err
	}
	defer f.Close()
	s := bufio.NewScanner(f)
	for s.Scan() {
		line := strings.TrimSpace(s.Text())
		if strings.HasPrefix(line, "module ") {
			return strings.TrimSpace(strings.TrimPrefix(line, "module ")), nil
		}
	}
	if err := s.Err(); err != nil {
		return "", err
	}
	return "", errors.New("module path not found")
}

func hasCycleMessage(out string) bool {
	lower := strings.ToLower(out)
	return strings.Contains(lower, "import cycle") || strings.Contains(lower, "would create an import cycle")
}
