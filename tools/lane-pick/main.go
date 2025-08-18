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
	if exists(filepath.Join(root, "pyproject.toml")) || exists(filepath.Join(root, "requirements.txt")) { 
		lang = "python"; lane = "B"; reasons = append(reasons, "python detected")
		// Check for C extensions
		if hasAny(root, ".c") || hasAny(root, ".cc") || grep(root, "ext_modules") {
			lane = "C"; reasons = append(reasons, "Python C-extensions detected")
		}
	}
	if hasAny(root, ".csproj") { lang = ".net"; lane = "C"; reasons = append(reasons, ".csproj detected") }
	// Check Scala first (more specific than Java)
	if hasAny(root, "build.sbt") { 
		lang = "scala"
		// Enhanced Jib detection for SBT projects
		if hasJibPlugin(root) {
			lane = "E"; reasons = append(reasons, "Scala with Jib plugin detected - optimal for containerless builds")
		} else {
			lane = "C"; reasons = append(reasons, "Scala build.sbt detected - using OSv for JVM optimization")
		}
	} else if exists(filepath.Join(root, "pom.xml")) || hasAny(root, "build.gradle") || hasAny(root, "build.gradle.kts") {
		// Check if it's a Scala project with Gradle/Maven
		if grep(root, "scala-library") || grep(root, "org.jetbrains.kotlin.jvm") || hasAny(root, ".scala") {
			if grep(root, "scala-library") || hasAny(root, ".scala") {
				lang = "scala"
			} else {
				lang = "java" // Kotlin projects treated as Java
			}
		} else {
			lang = "java"
		}
		// Enhanced Jib detection for multiple configurations
		if hasJibPlugin(root) {
			if lang == "scala" {
				lane = "E"; reasons = append(reasons, "Scala with Jib plugin detected - optimal for containerless builds")
			} else {
				lane = "E"; reasons = append(reasons, "Java with Jib plugin detected - optimal for containerless builds")
			}
		} else {
			if lang == "scala" {
				lane = "C"; reasons = append(reasons, "Scala build tool detected - using OSv for JVM optimization")
			} else {
				lane = "C"; reasons = append(reasons, "Java build tool detected - using OSv for JVM optimization")
			}
		}
	}

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
			// Search in source code files and build scripts
			if strings.HasSuffix(p, ".c") || strings.HasSuffix(p, ".cc") || 
			   strings.HasSuffix(p, ".go") || strings.HasSuffix(p, ".rs") || 
			   strings.HasSuffix(p, ".js") || strings.HasSuffix(p, ".ts") || 
			   strings.HasSuffix(p, ".py") || strings.HasSuffix(p, ".gradle") || 
			   strings.HasSuffix(p, ".gradle.kts") || strings.HasSuffix(p, ".kts") || 
			   strings.HasSuffix(p, "build.sbt") || strings.HasSuffix(p, "pom.xml") {
				b, _ := os.ReadFile(p)
				if strings.Contains(string(b), needle) { match = true }
			}
		}
		return nil
	})
	return match
}

// hasJibPlugin detects Jib plugin in various build systems
func hasJibPlugin(root string) bool {
	// Check for Gradle Jib plugin
	if grep(root, "com.google.cloud.tools.jib") {
		return true
	}
	// Check for Jib configuration block
	if grep(root, "jib {") {
		return true
	}
	// Check for Jib tasks (Gradle)
	if grep(root, "jibBuildTar") || grep(root, "jibDockerBuild") {
		return true
	}
	// Check for SBT Jib plugin
	if grep(root, "sbt-jib") {
		return true
	}
	// Check for Maven Jib plugin
	if grep(root, "<groupId>com.google.cloud.tools</groupId>") && grep(root, "<artifactId>jib-maven-plugin</artifactId>") {
		return true
	}
	// Check for Maven Jib plugin (abbreviated)
	if grep(root, "jib-maven-plugin") {
		return true
	}
	return false
}
