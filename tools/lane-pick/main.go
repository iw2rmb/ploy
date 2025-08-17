package main
import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

type Result struct{
	Lane string `json:"lane"`
	Language string `json:"language"`
	Reasons []string `json:"reasons"`
}

func main(){
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
	enc := json.NewEncoder(os.Stdout); enc.SetIndent("", "  "); enc.Encode(res)
}

func detect(root string) Result {
	reasons := []string{}
	lang := "unknown"
	lane := "A"

	// Simple language detection
	if exists(filepath.Join(root, "go.mod")) { lang = "go"; lane = "A"; reasons = append(reasons, "go.mod detected") }
	if exists(filepath.Join(root, "Cargo.toml")) { lang = "rust"; lane = "A"; reasons = append(reasons, "Cargo.toml detected") }
	if exists(filepath.Join(root, "package.json")) { lang = "node"; lane = "B"; reasons = append(reasons, "package.json detected") }
	if exists(filepath.Join(root, "pyproject.toml")) || exists(filepath.Join(root, "requirements.txt")) { lang = "python"; lane = "B"; reasons = append(reasons, "python detected") }
	if hasAny(root, ".csproj") { lang = ".net"; lane = "C"; reasons = append(reasons, ".csproj detected") }
	if exists(filepath.Join(root, "pom.xml")) || hasAny(root, "build.gradle") || hasAny(root, "build.gradle.kts") {
		if lang == "unknown" { lang = "java" }
		lane = "C"; reasons = append(reasons, "Java/Scala build tool detected")
	}
	if hasAny(root, "build.sbt") { lang = "scala"; lane = "C"; reasons = append(reasons, "build.sbt detected") }

	// Heuristics for POSIX-heavy
	if grep(root, "fork(") || grep(root, "/proc") { lane = "C"; reasons = append(reasons, "POSIX-heavy features detected") }

	return Result{ Lane: lane, Language: lang, Reasons: reasons }
}

func exists(p string) bool { _, err := os.Stat(p); return err == nil }

func hasAny(root string, name string) bool {
	found := false
	filepath.WalkDir(root, func(p string, d os.DirEntry, err error) error {
		if err==nil && strings.HasSuffix(p, name) { found = true }
		return nil
	})
	return found
}

func grep(root, needle string) bool {
	match := false
	filepath.WalkDir(root, func(p string, d os.DirEntry, err error) error {
		if err==nil && !d.IsDir() {
			if strings.HasSuffix(p, ".c") || strings.HasSuffix(p, ".cc") || strings.HasSuffix(p, ".go") || strings.HasSuffix(p, ".rs") || strings.HasSuffix(p, ".js") || strings.HasSuffix(p, ".ts") || strings.HasSuffix(p, ".py") {
				b, _ := os.ReadFile(p)
				if strings.Contains(string(b), needle) { match = true }
			}
		}
		return nil
	})
	return match
}
