package build

import (
    "fmt"
    "os"
    "path/filepath"
    "strings"

    "github.com/iw2rmb/ploy/internal/build/templates"
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
    // Java/Scala (JVM) via Gradle/Maven using embedded templates; fallback to inline if template missing
    if facts.Language == "java" || facts.Language == "scala" || facts.BuildTool == "gradle" || facts.BuildTool == "maven" {
        v := facts.Versions.Java
        if v == "" { v = "17" }
        if i := strings.Index(v, "."); i > 0 { v = v[:i] }
        type data struct { JavaVersion string; MainClass string }
        d := data{JavaVersion: v, MainClass: facts.MainClass}
        var path string
        switch facts.BuildTool {
        case "gradle":
            path = "dockerfiles/java/gradle.Dockerfile.tmpl"
        case "maven":
            path = "dockerfiles/java/maven.Dockerfile.tmpl"
        default:
            path = "dockerfiles/java/default.Dockerfile.tmpl"
        }
        if path != "" {
            if rendered, err := templates.Render(path, d); err == nil {
                return os.WriteFile(filepath.Join(srcDir, "Dockerfile"), []byte(rendered), 0644)
            }
        }
        // Fallback to previous inline generation if template missing
        var dockerfile string
        switch facts.BuildTool {
        case "gradle":
            entry := "ENTRYPOINT [\\\"java\\\",\\\"-jar\\\",\\\"/app/app.jar\\\"]"
            if facts.MainClass != "" { entry = fmt.Sprintf("ENTRYPOINT [\\\\\\\"java\\\\\\\",\\\\\\\"-cp\\\\\\\",\\\\\\\"/app/app.jar\\\\\\\",\\\\\\\"%s\\\\\\\"]", facts.MainClass) }
            dockerfile = fmt.Sprintf(`FROM gradle:8-jdk%[1]s AS build
WORKDIR /src
COPY . .
RUN chmod +x ./gradlew || true \
 && ( ./gradlew -x test clean build || gradle -x test clean build )

FROM eclipse-temurin:%[1]s-jre
WORKDIR /app
COPY --from=build /src/build/libs/*.jar /app/app.jar
ENV PORT=8080
EXPOSE 8080
%s
`, v, entry)
        case "maven":
            entry := "ENTRYPOINT [\\\"java\\\",\\\"-jar\\\",\\\"/app/app.jar\\\"]"
            if facts.MainClass != "" { entry = fmt.Sprintf("ENTRYPOINT [\\\\\\\"java\\\\\\\",\\\\\\\"-cp\\\\\\\",\\\\\\\"/app/app.jar\\\\\\\",\\\\\\\"%s\\\\\\\"]", facts.MainClass) }
            dockerfile = fmt.Sprintf(`FROM maven:3-eclipse-temurin-%[1]s AS build
WORKDIR /src
COPY . .
RUN chmod +x ./mvnw || true \
 && ( ./mvnw -B -DskipTests package || mvn -B -DskipTests package )

FROM eclipse-temurin:%[1]s-jre
WORKDIR /app
COPY --from=build /src/target/*.jar /app/app.jar
ENV PORT=8080
EXPOSE 8080
%s
`, v, entry)
        default:
            // Fallback default: compile sources with javac into classes and run MainClass
            entry := "CMD [\\\"sh\\\",\\\"-lc\\\",\\\"echo 'Set MainClass to run application' && sleep 30\\\"]"
            if facts.MainClass != "" { entry = fmt.Sprintf("ENTRYPOINT [\\\\\\\"java\\\\\\\",\\\\\\\"-cp\\\\\\\",\\\\\\\"/app\\\\\\\",\\\\\\\"%s\\\\\\\"]", facts.MainClass) }
            dockerfile = fmt.Sprintf(`FROM eclipse-temurin:%[1]s-jdk AS build
WORKDIR /src
COPY . .
RUN if [ -d src/main/java ]; then find src/main/java -name "*.java" -print0 | xargs -0 javac -d /out; else echo "No src/main/java found"; fi

FROM eclipse-temurin:%[1]s-jre
WORKDIR /app
COPY --from=build /out /app
ENV PORT=8080
EXPOSE 8080
%s
`, v, entry)
        }
        return os.WriteFile(filepath.Join(srcDir, "Dockerfile"), []byte(dockerfile), 0644)
    }

	// Go
	goMod := filepath.Join(srcDir, "go.mod")
	if _, err := os.Stat(goMod); err == nil {
		content := `FROM golang:1.22-alpine AS build
WORKDIR /src
COPY go.mod .
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o /out/app ./...

FROM gcr.io/distroless/static
ENV PORT=8080
ENV PORT=8080
EXPOSE 8080
COPY --from=build /out/app /app
ENTRYPOINT ["/app"]
`
		return os.WriteFile(filepath.Join(srcDir, "Dockerfile"), []byte(content), 0644)
	}
    // Node via template (npm)
    pkgJSON := filepath.Join(srcDir, "package.json")
    if _, err := os.Stat(pkgJSON); err == nil {
        v := facts.Versions.Node
        if v == "" { v = "20" }
        type data struct { NodeVersion string }
        d := data{NodeVersion: v}
        if rendered, err := templates.Render("dockerfiles/node/npm.Dockerfile.tmpl", d); err == nil {
            return os.WriteFile(filepath.Join(srcDir, "Dockerfile"), []byte(rendered), 0644)
        }
        // Fallback inline
        content := fmt.Sprintf(`FROM node:%s-alpine
WORKDIR /app
COPY package.json .
RUN npm install --omit=dev || true
COPY . .
ENV PORT=8080
EXPOSE 8080
CMD ["node", "index.js"]
`, v)
        return os.WriteFile(filepath.Join(srcDir, "Dockerfile"), []byte(content), 0644)
    }
    // Python via template
    // Use detected Python version for base image (python:<ver>-slim). Fallback to 3.12.
    // Also support minimal apps with app.py only (no requirements.txt/pyproject.toml).
    if facts.Language == "python" || fileExists(filepath.Join(srcDir, "requirements.txt")) || fileExists(filepath.Join(srcDir, "pyproject.toml")) || fileExists(filepath.Join(srcDir, "app.py")) {
        v := facts.Versions.Python
        if v == "" { v = "3.12" }
        if parts := strings.Split(v, "."); len(parts) >= 2 { v = parts[0] + "." + parts[1] }
        hasGunicorn := pythonDepPresent(srcDir, "gunicorn")
        hasUvicorn := pythonDepPresent(srcDir, "uvicorn")
        type data struct { PythonVersion string; UseGunicorn bool; UseUvicorn bool }
        d := data{PythonVersion: v, UseGunicorn: hasGunicorn, UseUvicorn: hasUvicorn}
        if rendered, err := templates.Render("dockerfiles/python/default.Dockerfile.tmpl", d); err == nil {
            return os.WriteFile(filepath.Join(srcDir, "Dockerfile"), []byte(rendered), 0644)
        }
        // Fallback inline
        base := fmt.Sprintf("python:%s-slim", v)
        cmd := `CMD ["python", "app.py"]`
        if hasGunicorn { cmd = `CMD ["sh", "-lc", "exec gunicorn -b 0.0.0.0:$PORT app:app"]` } else if hasUvicorn { cmd = `CMD ["sh", "-lc", "exec uvicorn app:app --host 0.0.0.0 --port $PORT"]` }
        content := fmt.Sprintf(`FROM %s
WORKDIR /app
ENV PYTHONDONTWRITEBYTECODE=1
ENV PYTHONUNBUFFERED=1
ENV PYTHONPATH=/app
ENV PORT=8080
COPY . .
RUN if [ -f requirements.txt ] && [ -s requirements.txt ]; then pip install --no-cache-dir -r requirements.txt; fi || true
EXPOSE 8080
%s
`, base, cmd)
        return os.WriteFile(filepath.Join(srcDir, "Dockerfile"), []byte(content), 0644)
    }
	// .NET
	// Detect .NET projects by presence of *.csproj
	if csproj := findFirstCsproj(srcDir); csproj != "" {
		// Derive version tag
		v := facts.Versions.Dotnet
		if v == "" {
			v = "8.0"
		}
		// Normalize to major.minor (e.g., 8.0)
		if parts := strings.Split(v, "."); len(parts) >= 2 {
			v = parts[0] + "." + parts[1]
		} else if len(v) == 1 {
			v = v + ".0"
		}
		projName := strings.TrimSuffix(filepath.Base(csproj), filepath.Ext(csproj))
		content := fmt.Sprintf(`FROM mcr.microsoft.com/dotnet/sdk:%[1]s AS build
WORKDIR /src
COPY . .
RUN dotnet restore
RUN dotnet publish -c Release -o /app/out

FROM mcr.microsoft.com/dotnet/aspnet:%[1]s
WORKDIR /app
COPY --from=build /app/out .
ENV ASPNETCORE_URLS=http://0.0.0.0:8080
EXPOSE 8080
ENTRYPOINT ["dotnet", "%[2]s.dll"]
`, v, projName)
		return os.WriteFile(filepath.Join(srcDir, "Dockerfile"), []byte(content), 0644)
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
