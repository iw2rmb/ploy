package unit

import (
	"archive/tar"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

// Test that setup-workspace.sh prefers CONTEXT_DIR (defaults to /workspace/context)
// and creates an input.tar containing the project build root at archive root.
func TestOpenRewriteSetupWorkspacePrefersContextDir(t *testing.T) {
	t.Parallel()

	// Create a temporary fake context with a Maven project
	baseDir := t.TempDir()
	contextDir := filepath.Join(baseDir, "context")
	if err := os.MkdirAll(filepath.Clean(filepath.Join(contextDir, "src", "main", "java")), 0o755); err != nil {
		t.Fatalf("failed to create context dir: %v", err)
	}
	// Minimal pom.xml to mark build root
	pom := []byte("<project><modelVersion>4.0.0</modelVersion><groupId>x</groupId><artifactId>y</artifactId><version>1.0</version></project>")
	if err := os.WriteFile(filepath.Join(contextDir, "pom.xml"), pom, 0o644); err != nil {
		t.Fatalf("failed to write pom.xml: %v", err)
	}
	// Add one source file
	javaFile := filepath.Join(contextDir, "src", "main", "java", "App.java")
	if err := os.WriteFile(javaFile, []byte("class App {}"), 0o644); err != nil {
		t.Fatalf("failed to write App.java: %v", err)
	}

	// Workspace output directory (override /workspace)
	workspaceDir := filepath.Join(baseDir, "ws")
	if err := os.MkdirAll(workspaceDir, 0o755); err != nil {
		t.Fatalf("failed to create workspace dir: %v", err)
	}

	// Run the setup script with overrides so it uses our temp paths and doesn't exec the runner
	// Resolve script path relative to repo root
	cwd, err := os.Getwd()
	if err != nil {
		t.Fatalf("failed to get working directory: %v", err)
	}
	repoRoot := filepath.Clean(filepath.Join(cwd, "..", ".."))
	scriptPath := filepath.Join(repoRoot, "services", "openrewrite-jvm", "setup-workspace.sh")

	cmd := exec.Command("bash", scriptPath)
	cmd.Env = append(os.Environ(),
		"CONTEXT_DIR="+contextDir,
		"WORKSPACE_DIR="+workspaceDir,
		"SKIP_EXEC_OPENREWRITE=1",
	)
	// Run from repo root so relative paths in the script work as expected
	cmd.Dir = repoRoot
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("setup-workspace.sh failed: %v\nOutput:\n%s", err, string(out))
	}

	// Validate that input.tar exists in the overridden workspace
	tarPath := filepath.Join(workspaceDir, "input.tar")
	st, err := os.Stat(tarPath)
	if err != nil {
		t.Fatalf("input.tar not found at %s: %v\nOutput:\n%s", tarPath, err, string(out))
	}
	if st.Size() == 0 {
		t.Fatalf("input.tar is empty at %s", tarPath)
	}

	// Open tar and ensure pom.xml exists at the archive root (project root)
	f, err := os.Open(tarPath)
	if err != nil {
		t.Fatalf("failed to open input.tar: %v", err)
	}
	defer func() { _ = f.Close() }()

	tr := tar.NewReader(f)
	foundPom := false
	foundJava := false
	for {
		hdr, err := tr.Next()
		if err != nil {
			break
		}
		switch hdr.Name {
		case "./pom.xml", "pom.xml":
			foundPom = true
		case "./src/main/java/App.java", "src/main/java/App.java":
			foundJava = true
		}
		if foundPom && foundJava {
			break
		}
	}
	if !foundPom {
		t.Fatalf("pom.xml not found at tar root; script may not have selected build root correctly. Output:\n%s", string(out))
	}
	if !foundJava {
		t.Fatalf("Java source not found in tar; Output:\n%s", string(out))
	}
}
