package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

type Result struct {
	Lane     string   `json:"lane"`
	Language string   `json:"language"`
	Reasons  []string `json:"reasons"`
}

func main() {
	var path string
	flag.StringVar(&path, "path", ".", "path to app")
	var validate bool
	flag.BoolVar(&validate, "validate", false, "validate manifests")
	flag.Parse()

	if validate {
		fmt.Println("Validation placeholder OK")
		return
	}

	res := detect(path)
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	_ = enc.Encode(res)
}

func detect(root string) Result {
	language, reasons := detectLanguage(root)
	reasons = append(reasons, "Lane D (Docker) selected: other lanes are disabled")
	return Result{
		Lane:     "D",
		Language: language,
		Reasons:  reasons,
	}
}

func detectLanguage(root string) (string, []string) {
	reasons := []string{}

	if exists(filepath.Join(root, "go.mod")) {
		return "go", append(reasons, "go.mod detected")
	}
	if exists(filepath.Join(root, "Cargo.toml")) {
		return "rust", append(reasons, "Cargo.toml detected")
	}
	if exists(filepath.Join(root, "package.json")) {
		return "node", append(reasons, "package.json detected")
	}
	if exists(filepath.Join(root, "pyproject.toml")) || exists(filepath.Join(root, "requirements.txt")) {
		return "python", append(reasons, "Python project detected")
	}
	if hasAny(root, ".csproj") {
		return ".net", append(reasons, ".csproj detected")
	}
	if exists(filepath.Join(root, "build.sbt")) {
		return "scala", append(reasons, "build.sbt detected")
	}
	if exists(filepath.Join(root, "pom.xml")) || hasAny(root, "build.gradle") || hasAny(root, "build.gradle.kts") {
		return "java", append(reasons, "Java build tool detected")
	}
	return "unknown", reasons
}

func exists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

func hasAny(root, suffix string) bool {
	found := false
	filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return nil
		}
		if strings.HasSuffix(path, suffix) {
			found = true
		}
		return nil
	})
	return found
}
