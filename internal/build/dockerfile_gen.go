package build

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/iw2rmb/ploy/internal/detect/project"
)

// generateDockerfileWithFacts writes a simple Dockerfile into srcDir based on detected project markers.
// Supports Go, Node.js, Python, .NET, and JVM via Gradle/Maven. For other stacks, returns an error.
func generateDockerfileWithFacts(srcDir string, facts project.BuildFacts) error {
	// Ensure we infer JVM build tool even if facts weren't populated upstream
	if facts.BuildTool == "" && facts.Language == "" {
		if fileExists(filepath.Join(srcDir, "build.gradle.kts")) || fileExists(filepath.Join(srcDir, "build.gradle")) {
			facts.Language = "java"
			facts.BuildTool = "gradle"
		} else if fileExists(filepath.Join(srcDir, "pom.xml")) {
			facts.Language = "java"
			facts.BuildTool = "maven"
		}
	}
	if facts.Language == "java" || facts.Language == "scala" || facts.BuildTool == "gradle" || facts.BuildTool == "maven" {
		facts.Language = "java"
		if facts.BuildTool == "" {
			if fileExists(filepath.Join(srcDir, "build.gradle.kts")) || fileExists(filepath.Join(srcDir, "build.gradle")) {
				facts.BuildTool = "gradle"
			} else if fileExists(filepath.Join(srcDir, "pom.xml")) {
				facts.BuildTool = "maven"
			} else {
				facts.BuildTool = "default"
			}
		}
		tool := strings.ToLower(strings.TrimSpace(facts.BuildTool))
		var opts []DockerfilePairOption
		switch tool {
		case "gradle", "maven":
			opts = append(opts, WithCopyInstruction("COPY --from=build /out/app.jar /app/app.jar"))
		case "default":
			opts = append(opts, WithCopyInstruction("COPY --from=build /out /app"))
		default:
			facts.BuildTool = "default"
			opts = append(opts, WithCopyInstruction("COPY --from=build /out /app"))
		}
		if err := writePairForFacts(srcDir, facts, opts...); err != nil {
			return err
		}
		return nil
	}

	// Go
	goMod := filepath.Join(srcDir, "go.mod")
	if _, err := os.Stat(goMod); err == nil {
		goFacts := facts
		goFacts.Language = "go"
		if goFacts.Versions.Go == "" {
			goFacts.Versions.Go = "1.22"
		}
		return writePairForFacts(srcDir, goFacts, WithCopyInstruction("COPY --from=build /out/app /app"))
	}
	// Node via template (npm)
	pkgJSON := filepath.Join(srcDir, "package.json")
	if _, err := os.Stat(pkgJSON); err == nil {
		nFacts := facts
		nFacts.Language = "node"
		if nFacts.Versions.Node == "" {
			nFacts.Versions.Node = "20"
		}
		return writePairForFacts(srcDir, nFacts, WithCopyInstruction("COPY --from=build /app /app"))
	}
	// Python via template
	// Use detected Python version for base image (python:<ver>-slim). Fallback to 3.12.
	// Also support minimal apps with app.py only (no requirements.txt/pyproject.toml).
	if facts.Language == "python" || fileExists(filepath.Join(srcDir, "requirements.txt")) || fileExists(filepath.Join(srcDir, "pyproject.toml")) || fileExists(filepath.Join(srcDir, "app.py")) {
		pyFacts := facts
		pyFacts.Language = "python"
		if pyFacts.Versions.Python == "" {
			pyFacts.Versions.Python = "3.12"
		}
		// normalize version to major.minor
		if parts := strings.Split(pyFacts.Versions.Python, "."); len(parts) >= 2 {
			pyFacts.Versions.Python = parts[0] + "." + parts[1]
		}
		hasGunicorn := pythonDepPresent(srcDir, "gunicorn")
		hasUvicorn := pythonDepPresent(srcDir, "uvicorn")
		gunicornCmd := ""
		uvicornCmd := ""
		if hasGunicorn {
			gunicornCmd = `["sh", "-lc", "exec gunicorn -b 0.0.0.0:$PORT app:app"]`
		}
		if hasUvicorn {
			uvicornCmd = `["sh", "-lc", "exec uvicorn app:app --host 0.0.0.0 --port $PORT"]`
		}
		opts := []DockerfilePairOption{WithPythonRuntimeCommands(gunicornCmd, uvicornCmd)}
		return writePairForFacts(srcDir, pyFacts, opts...)
	}
	// .NET
	// Detect .NET projects by presence of *.csproj
	if csproj := findFirstCsproj(srcDir); csproj != "" {
		dnFacts := facts
		dnFacts.Language = "dotnet"
		if dnFacts.Versions.Dotnet == "" {
			dnFacts.Versions.Dotnet = "8.0"
		}
		if parts := strings.Split(dnFacts.Versions.Dotnet, "."); len(parts) >= 2 {
			dnFacts.Versions.Dotnet = parts[0] + "." + parts[1]
		} else if len(dnFacts.Versions.Dotnet) == 1 {
			dnFacts.Versions.Dotnet = dnFacts.Versions.Dotnet + ".0"
		}
		projName := strings.TrimSuffix(filepath.Base(csproj), filepath.Ext(csproj))
		return writePairForFacts(srcDir, dnFacts, WithDotnetProjectName(projName))
	}
	return fmt.Errorf("unsupported autogeneration: no go.mod or package.json detected")
}

// fileExists wraps os.Stat for brevity
func fileExists(p string) bool { _, err := os.Stat(p); return err == nil }

// pythonDepPresent looks for a dependency name in common Python manifests
func pythonDepPresent(srcDir, name string) bool {
	// requirements.txt
	if b, err := os.ReadFile(filepath.Join(srcDir, "requirements.txt")); err == nil {
		if strings.Contains(strings.ToLower(string(b)), strings.ToLower(name)) {
			return true
		}
	}
	// Pipfile
	if b, err := os.ReadFile(filepath.Join(srcDir, "Pipfile")); err == nil {
		if strings.Contains(strings.ToLower(string(b)), strings.ToLower(name)) {
			return true
		}
	}
	// pyproject.toml
	if b, err := os.ReadFile(filepath.Join(srcDir, "pyproject.toml")); err == nil {
		s := strings.ToLower(string(b))
		if strings.Contains(s, "[project]") || strings.Contains(s, "[tool.poetry]") {
			if strings.Contains(s, name) {
				return true
			}
		}
	}
	return false
}

// findFirstCsproj returns the first *.csproj path in srcDir
func findFirstCsproj(srcDir string) string {
	entries, _ := os.ReadDir(srcDir)
	for _, e := range entries {
		if !e.IsDir() && strings.HasSuffix(strings.ToLower(e.Name()), ".csproj") {
			return filepath.Join(srcDir, e.Name())
		}
	}
	return ""
}
