package build

import (
	"fmt"
	"strings"

	"github.com/iw2rmb/ploy/internal/build/templates"
	project "github.com/iw2rmb/ploy/internal/detect/project"
)

// DockerfilePair represents build/deploy Dockerfile contents for multi-step pipelines.
type DockerfilePair struct {
	Build  string
	Deploy string
}

// dockerfilePairConfig holds render options.
type dockerfilePairConfig struct {
	copyInstruction string
	pythonRuntime   *pythonRuntimeConfig
	dotnetProject   string
}

// DockerfilePairOption mutates render options.
type DockerfilePairOption func(*dockerfilePairConfig)

// WithCopyInstruction overrides the COPY directive used in the deploy template.
func WithCopyInstruction(copy string) DockerfilePairOption {
	return func(cfg *dockerfilePairConfig) {
		cfg.copyInstruction = copy
	}
}

type pythonRuntimeConfig struct {
	GunicornCommand string
	UvicornCommand  string
}

// WithPythonRuntimeCommands injects Gunicorn/Uvicorn command strings for Python deploy templates.
func WithPythonRuntimeCommands(gunicornCmd, uvicornCmd string) DockerfilePairOption {
	return func(cfg *dockerfilePairConfig) {
		cfg.pythonRuntime = &pythonRuntimeConfig{
			GunicornCommand: gunicornCmd,
			UvicornCommand:  uvicornCmd,
		}
	}
}

// WithDotnetProjectName sets the project name used in .NET deploy templates.
func WithDotnetProjectName(name string) DockerfilePairOption {
	return func(cfg *dockerfilePairConfig) {
		cfg.dotnetProject = name
	}
}

// RenderDockerfilePair renders build and deploy Dockerfiles for the provided facts.
// Currently supports JVM applications (Gradle, Maven, generic Java fallback).
func RenderDockerfilePair(facts project.BuildFacts, opts ...DockerfilePairOption) (DockerfilePair, error) {
	cfg := dockerfilePairConfig{copyInstruction: "COPY app.jar /app/app.jar"}
	for _, opt := range opts {
		opt(&cfg)
	}

	lang := strings.ToLower(strings.TrimSpace(facts.Language))
	ft := strings.ToLower(strings.TrimSpace(facts.BuildTool))

	type templateSet struct {
		Build  string
		Deploy string
	}

	switch lang {
	case "java", "scala", "kotlin":
		if ft == "" {
			switch {
			case strings.EqualFold(facts.BuildTool, "gradle"):
				ft = "gradle"
			case strings.EqualFold(facts.BuildTool, "maven"):
				ft = "maven"
			default:
				ft = "default"
			}
		}
		javaVersion := strings.TrimSpace(facts.Versions.Java)
		if javaVersion == "" {
			javaVersion = "17"
		}
		if idx := strings.Index(javaVersion, "."); idx > 0 {
			javaVersion = javaVersion[:idx]
		}
		mainClass := strings.TrimSpace(facts.MainClass)
		var tpl templateSet
		switch ft {
		case "gradle":
			tpl = templateSet{
				Build:  "dockerfiles/java/gradle.build.Dockerfile.tmpl",
				Deploy: "dockerfiles/java/gradle.deploy.Dockerfile.tmpl",
			}
		case "maven":
			tpl = templateSet{
				Build:  "dockerfiles/java/maven.build.Dockerfile.tmpl",
				Deploy: "dockerfiles/java/maven.deploy.Dockerfile.tmpl",
			}
		case "default":
			tpl = templateSet{
				Build:  "dockerfiles/java/default.build.Dockerfile.tmpl",
				Deploy: "dockerfiles/java/default.deploy.Dockerfile.tmpl",
			}
			if cfg.copyInstruction == "COPY app.jar /app/app.jar" {
				cfg.copyInstruction = "COPY --from=build /out /app"
			}
		default:
			return DockerfilePair{}, fmt.Errorf("dockerfile pair unsupported for build tool=%s", ft)
		}
		buildData := map[string]string{"JavaVersion": javaVersion}
		deployData := map[string]string{
			"JavaVersion":     javaVersion,
			"MainClass":       mainClass,
			"CopyInstruction": cfg.copyInstruction,
		}
		build, err := templates.Render(tpl.Build, buildData)
		if err != nil {
			return DockerfilePair{}, err
		}
		deploy, err := templates.Render(tpl.Deploy, deployData)
		if err != nil {
			return DockerfilePair{}, err
		}
		return DockerfilePair{Build: build, Deploy: deploy}, nil
	case "go":
		goVersion := strings.TrimSpace(facts.Versions.Go)
		if goVersion == "" {
			goVersion = "1.22"
		}
		data := map[string]string{
			"GoVersion":       goVersion,
			"GoOS":            "linux",
			"GoArch":          "amd64",
			"BuildTarget":     "./...",
			"CopyInstruction": cfg.copyInstruction,
		}
		build, err := templates.Render("dockerfiles/go/default.build.Dockerfile.tmpl", data)
		if err != nil {
			return DockerfilePair{}, err
		}
		deploy, err := templates.Render("dockerfiles/go/default.deploy.Dockerfile.tmpl", data)
		if err != nil {
			return DockerfilePair{}, err
		}
		return DockerfilePair{Build: build, Deploy: deploy}, nil
	case "node", "javascript", "typescript":
		nodeVersion := strings.TrimSpace(facts.Versions.Node)
		if nodeVersion == "" {
			nodeVersion = "20"
		}
		data := map[string]string{
			"NodeVersion":     nodeVersion,
			"CopyInstruction": cfg.copyInstruction,
		}
		build, err := templates.Render("dockerfiles/node/npm.build.Dockerfile.tmpl", data)
		if err != nil {
			return DockerfilePair{}, err
		}
		deploy, err := templates.Render("dockerfiles/node/npm.deploy.Dockerfile.tmpl", data)
		if err != nil {
			return DockerfilePair{}, err
		}
		return DockerfilePair{Build: build, Deploy: deploy}, nil
	case "python":
		pythonVersion := strings.TrimSpace(facts.Versions.Python)
		if pythonVersion == "" {
			pythonVersion = "3.12"
		}
		if parts := strings.Split(pythonVersion, "."); len(parts) >= 2 {
			pythonVersion = parts[0] + "." + parts[1]
		}
		buildData := map[string]string{
			"PythonVersion": pythonVersion,
		}
		deployData := map[string]string{
			"PythonVersion": pythonVersion,
		}
		if cfg.pythonRuntime != nil {
			deployData["GunicornCommand"] = cfg.pythonRuntime.GunicornCommand
			deployData["UvicornCommand"] = cfg.pythonRuntime.UvicornCommand
		}
		build, err := templates.Render("dockerfiles/python/default.build.Dockerfile.tmpl", buildData)
		if err != nil {
			return DockerfilePair{}, err
		}
		deploy, err := templates.Render("dockerfiles/python/default.deploy.Dockerfile.tmpl", deployData)
		if err != nil {
			return DockerfilePair{}, err
		}
		return DockerfilePair{Build: build, Deploy: deploy}, nil
	case "dotnet", "csharp", "fsharp":
		dotnetVersion := strings.TrimSpace(facts.Versions.Dotnet)
		if dotnetVersion == "" {
			dotnetVersion = "8.0"
		}
		if parts := strings.Split(dotnetVersion, "."); len(parts) >= 2 {
			dotnetVersion = parts[0] + "." + parts[1]
		} else if len(dotnetVersion) == 1 {
			dotnetVersion = dotnetVersion + ".0"
		}
		projectName := strings.TrimSpace(cfg.dotnetProject)
		if projectName == "" {
			return DockerfilePair{}, fmt.Errorf("dotnet project name required")
		}
		buildData := map[string]string{"DotnetVersion": dotnetVersion}
		deployData := map[string]string{
			"DotnetVersion": dotnetVersion,
			"ProjectName":   projectName,
		}
		build, err := templates.Render("dockerfiles/dotnet/default.build.Dockerfile.tmpl", buildData)
		if err != nil {
			return DockerfilePair{}, err
		}
		deploy, err := templates.Render("dockerfiles/dotnet/default.deploy.Dockerfile.tmpl", deployData)
		if err != nil {
			return DockerfilePair{}, err
		}
		return DockerfilePair{Build: build, Deploy: deploy}, nil
	}

	return DockerfilePair{}, fmt.Errorf("dockerfile pair unsupported for language=%s", lang)
}

func combineDockerfilePair(pair DockerfilePair) (string, error) {
	build := strings.TrimSpace(pair.Build)
	deploy := strings.TrimSpace(pair.Deploy)
	if build == "" && deploy == "" {
		return "", fmt.Errorf("rendered dockerfile pair empty")
	}
	var out strings.Builder
	if build != "" {
		out.WriteString(build)
		out.WriteString("\n\n")
	}
	if deploy != "" {
		out.WriteString(deploy)
		out.WriteString("\n")
	}
	return out.String(), nil
}
