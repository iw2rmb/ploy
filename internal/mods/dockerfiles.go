package mods

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	build "github.com/iw2rmb/ploy/internal/build"
	project "github.com/iw2rmb/ploy/internal/detect/project"
	lanedetect "github.com/iw2rmb/ploy/internal/lane"
)

// ensureDockerfilePair ensures build/deploy Dockerfiles exist for lane D builds.
func ensureDockerfilePair(repoPath string) error {
	if strings.TrimSpace(repoPath) == "" {
		return fmt.Errorf("repo path is empty")
	}
	buildPath := filepath.Join(repoPath, "build.Dockerfile")
	deployPath := filepath.Join(repoPath, "deploy.Dockerfile")
	buildExists := fileExists(buildPath)
	deployExists := fileExists(deployPath)
	if buildExists && deployExists {
		return nil
	}

	detect := lanedetect.Detect(repoPath)
	facts := project.ComputeFacts(repoPath, detect.Language)
	lang := strings.ToLower(strings.TrimSpace(facts.Language))

	// Fallback to hints based on files when language detection is empty
	if lang == "" {
		if fileExists(filepath.Join(repoPath, "go.mod")) {
			lang = "go"
		} else if fileExists(filepath.Join(repoPath, "package.json")) {
			lang = "node"
		} else if findDotnetProjectName(repoPath) != "" {
			lang = "dotnet"
		} else if fileExists(filepath.Join(repoPath, "requirements.txt")) || fileExists(filepath.Join(repoPath, "pyproject.toml")) || fileExists(filepath.Join(repoPath, "app.py")) {
			lang = "python"
		}
	}

	var (
		pair build.DockerfilePair
		err  error
	)

	switch lang {
	case "java", "kotlin", "scala":
		pair, err = build.RenderDockerfilePair(facts)
	case "go":
		facts.Language = "go"
		if facts.Versions.Go == "" {
			facts.Versions.Go = "1.22"
		}
		pair, err = build.RenderDockerfilePair(facts, build.WithCopyInstruction("COPY --from=build /out/app /app"))
	case "node", "javascript", "typescript":
		facts.Language = "node"
		if facts.Versions.Node == "" {
			facts.Versions.Node = "20"
		}
		pair, err = build.RenderDockerfilePair(facts, build.WithCopyInstruction("COPY --from=build /app /app"))
	case "python":
		facts.Language = "python"
		if facts.Versions.Python == "" {
			facts.Versions.Python = "3.12"
		}
		gunicorn, uvicorn := detectPythonServerCommands(repoPath)
		pair, err = build.RenderDockerfilePair(facts, build.WithPythonRuntimeCommands(gunicorn, uvicorn))
	case "dotnet", "csharp", "fsharp", ".net":
		facts.Language = "dotnet"
		if facts.Versions.Dotnet == "" {
			facts.Versions.Dotnet = "8.0"
		}
		if projectName := findDotnetProjectName(repoPath); projectName != "" {
			pair, err = build.RenderDockerfilePair(facts, build.WithDotnetProjectName(projectName))
		} else {
			return fmt.Errorf("dotnet project file not found")
		}
	default:
		return nil
	}
	if err != nil {
		return fmt.Errorf("render dockerfile pair: %w", err)
	}
	if strings.TrimSpace(pair.Build) == "" || strings.TrimSpace(pair.Deploy) == "" {
		return fmt.Errorf("empty dockerfile pair for language %s", lang)
	}

	if !buildExists {
		if err := os.WriteFile(buildPath, []byte(strings.TrimSpace(pair.Build)+"\n"), 0o644); err != nil {
			return fmt.Errorf("write build.Dockerfile: %w", err)
		}
	}
	if !deployExists {
		if err := os.WriteFile(deployPath, []byte(strings.TrimSpace(pair.Deploy)+"\n"), 0o644); err != nil {
			return fmt.Errorf("write deploy.Dockerfile: %w", err)
		}
	}
	return nil
}

func fileExists(path string) bool {
	if strings.TrimSpace(path) == "" {
		return false
	}
	if _, err := os.Stat(path); err == nil {
		return true
	}
	return false
}

func detectPythonServerCommands(repoPath string) (gunicornCmd, uvicornCmd string) {
	if pythonDepPresent(repoPath, "gunicorn") {
		gunicornCmd = `["sh", "-lc", "exec gunicorn -b 0.0.0.0:$PORT app:app"]`
	}
	if pythonDepPresent(repoPath, "uvicorn") {
		uvicornCmd = `["sh", "-lc", "exec uvicorn app:app --host 0.0.0.0 --port $PORT"]`
	}
	return
}

func pythonDepPresent(repoPath, name string) bool {
	paths := []string{"requirements.txt", "Pipfile", "pyproject.toml"}
	needle := strings.ToLower(name)
	for _, p := range paths {
		b, err := os.ReadFile(filepath.Join(repoPath, p))
		if err != nil {
			continue
		}
		if strings.Contains(strings.ToLower(string(b)), needle) {
			return true
		}
	}
	return false
}

func findDotnetProjectName(repoPath string) string {
	entries, err := os.ReadDir(repoPath)
	if err != nil {
		return ""
	}
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := entry.Name()
		if strings.HasSuffix(strings.ToLower(name), ".csproj") {
			return strings.TrimSuffix(name, filepath.Ext(name))
		}
	}
	return ""
}
