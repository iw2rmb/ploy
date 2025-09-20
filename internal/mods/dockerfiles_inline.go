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

// dockerfileExtrasForLaneD renders the build/deploy Dockerfiles for lane D without writing them to disk.
// Returns nil when both Dockerfiles already exist in the repository.
func dockerfileExtrasForLaneD(repoPath string) (map[string][]byte, error) {
	if strings.TrimSpace(repoPath) == "" {
		return nil, fmt.Errorf("repo path is empty")
	}
	buildPath := filepath.Join(repoPath, "build.Dockerfile")
	deployPath := filepath.Join(repoPath, "deploy.Dockerfile")
	buildExists := fileExists(buildPath)
	deployExists := fileExists(deployPath)
	if buildExists && deployExists {
		return nil, nil
	}

	detect := lanedetect.Detect(repoPath)
	facts := project.ComputeFacts(repoPath, detect.Language)
	lang := strings.ToLower(strings.TrimSpace(facts.Language))

	if lang == "" {
		switch {
		case fileExists(filepath.Join(repoPath, "go.mod")):
			lang = "go"
		case fileExists(filepath.Join(repoPath, "package.json")):
			lang = "node"
		case findDotnetProjectName(repoPath) != "":
			lang = "dotnet"
		case fileExists(filepath.Join(repoPath, "requirements.txt")) || fileExists(filepath.Join(repoPath, "pyproject.toml")) || fileExists(filepath.Join(repoPath, "app.py")):
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
			return nil, fmt.Errorf("dotnet project file not found")
		}
	default:
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("render dockerfile pair: %w", err)
	}
	buildContent := strings.TrimSpace(pair.Build)
	deployContent := strings.TrimSpace(pair.Deploy)
	if buildContent == "" || deployContent == "" {
		return nil, fmt.Errorf("empty dockerfile pair for language %s", lang)
	}

	extras := make(map[string][]byte, 2)
	if !buildExists {
		extras["build.Dockerfile"] = []byte(buildContent + "\n")
	}
	if !deployExists {
		extras["deploy.Dockerfile"] = []byte(deployContent + "\n")
	}
	if len(extras) == 0 {
		return nil, nil
	}
	return extras, nil
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
