package lane

import (
	"os"
	"path/filepath"
	"strings"
)

type Result struct {
	Lane     string   `json:"lane"`
	Language string   `json:"language"`
	Reasons  []string `json:"reasons"`
}

func Detect(root string) Result {
	reasons := []string{}
	lang := "unknown"
	lane := "A"

	// Simple language detection
	if exists(filepath.Join(root, "go.mod")) {
		lang = "go"
		lane = "A"
		reasons = append(reasons, "go.mod detected")
	}
	if exists(filepath.Join(root, "Cargo.toml")) {
		lang = "rust"
		lane = "A"
		reasons = append(reasons, "Cargo.toml detected")
	}
	if exists(filepath.Join(root, "package.json")) {
		lang = "node"
		lane = "B"
		reasons = append(reasons, "package.json detected")
	}
	if exists(filepath.Join(root, "pyproject.toml")) || exists(filepath.Join(root, "requirements.txt")) {
		lang = "python"
		lane = "B"
		reasons = append(reasons, "python detected")
		// Enhanced C extensions detection
		if hasPythonCExtensions(root) {
			lane = "C"
			reasons = append(reasons, "Python C-extensions detected - requires full POSIX environment")
		}
	}
	if hasAny(root, ".csproj") {
		lang = ".net"
		lane = "C"
		reasons = append(reasons, ".csproj detected")
	}
	// Check Scala first (more specific than Java)
	if hasAny(root, "build.sbt") {
		lang = "scala"
		// Enhanced Jib detection for SBT projects
		if hasJibPlugin(root) {
			lane = "E"
			reasons = append(reasons, "Scala with Jib plugin detected - optimal for containerless builds")
		} else {
			lane = "C"
			reasons = append(reasons, "Scala build.sbt detected - using OSv for JVM optimization")
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
				lane = "E"
				reasons = append(reasons, "Scala with Jib plugin detected - optimal for containerless builds")
			} else {
				lane = "E"
				reasons = append(reasons, "Java with Jib plugin detected - optimal for containerless builds")
			}
		} else {
			if lang == "scala" {
				lane = "C"
				reasons = append(reasons, "Scala build tool detected - using OSv for JVM optimization")
			} else {
				lane = "C"
				reasons = append(reasons, "Java build tool detected - using OSv for JVM optimization")
			}
		}
	}

	// Heuristics for POSIX-heavy
	if grep(root, "fork(") || grep(root, "/proc") {
		lane = "C"
		reasons = append(reasons, "POSIX-heavy features detected")
	}

	return Result{Lane: lane, Language: lang, Reasons: reasons}
}

func exists(p string) bool {
	_, err := os.Stat(p)
	return err == nil
}

func hasAny(root string, name string) bool {
	found := false
	if err := filepath.WalkDir(root, func(p string, d os.DirEntry, err error) error {
		if err == nil && strings.HasSuffix(p, name) {
			found = true
		}
		return nil
	}); err != nil {
		return false
	}
	return found
}

func grep(root, needle string) bool {
	match := false
	if err := filepath.WalkDir(root, func(p string, d os.DirEntry, err error) error {
		if err == nil && !d.IsDir() {
			// Get base filename for specific file checks
			baseName := filepath.Base(p)

			// Search in source code files and build scripts
			if strings.HasSuffix(p, ".c") || strings.HasSuffix(p, ".cc") ||
				strings.HasSuffix(p, ".cpp") || strings.HasSuffix(p, ".cxx") ||
				strings.HasSuffix(p, ".go") || strings.HasSuffix(p, ".rs") ||
				strings.HasSuffix(p, ".js") || strings.HasSuffix(p, ".ts") ||
				strings.HasSuffix(p, ".py") || strings.HasSuffix(p, ".pyx") ||
				strings.HasSuffix(p, ".gradle") || strings.HasSuffix(p, ".gradle.kts") ||
				strings.HasSuffix(p, ".kts") || strings.HasSuffix(p, "build.sbt") ||
				strings.HasSuffix(p, "pom.xml") || strings.HasSuffix(p, "setup.py") ||
				strings.HasSuffix(p, "pyproject.toml") || strings.HasSuffix(p, "requirements.txt") ||
				strings.HasSuffix(p, "CMakeLists.txt") || strings.HasSuffix(p, ".toml") ||
				strings.HasSuffix(p, ".json") || strings.HasSuffix(p, ".sh") ||
				strings.HasSuffix(p, ".bash") || strings.HasSuffix(p, ".zsh") ||
				// Check for files without extensions (like monitor.py without .py)
				baseName == "Makefile" || baseName == "makefile" ||
				baseName == "Dockerfile" || baseName == "dockerfile" {
				b, _ := os.ReadFile(p)
				if strings.Contains(string(b), needle) {
					match = true
				}
			}
		}
		return nil
	}); err != nil {
		return false
	}
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

// hasPythonCExtensions detects Python C-extensions with comprehensive checks
func hasPythonCExtensions(root string) bool {
	// Check for C/C++/Cython source files
	if hasAny(root, ".c") || hasAny(root, ".cc") || hasAny(root, ".cpp") ||
		hasAny(root, ".cxx") || hasAny(root, ".pyx") || hasAny(root, ".pxd") {
		return true
	}

	// Check for setuptools/distutils configuration
	if grep(root, "ext_modules") || grep(root, "Extension(") {
		return true
	}

	// Check for Cython usage
	if grep(root, "from Cython") || grep(root, "import Cython") ||
		grep(root, "cythonize") {
		return true
	}

	// Check for popular C-extension libraries in requirements
	if grep(root, "numpy") || grep(root, "scipy") || grep(root, "pandas") ||
		grep(root, "psycopg2") || grep(root, "lxml") || grep(root, "pillow") ||
		grep(root, "cryptography") || grep(root, "cffi") || grep(root, "pycrypto") {
		return true
	}

	// Check for build configuration files with C-extension hints
	if grep(root, "build_ext") || grep(root, "include_dirs") ||
		grep(root, "library_dirs") || grep(root, "libraries =") {
		return true
	}

	// Check pyproject.toml for build system requiring C compilation
	if grep(root, "build-backend.*setuptools") &&
		(grep(root, "compiler_so") || grep(root, "extra_compile_args")) {
		return true
	}

	// Check for CMake integration (common with C extensions)
	if exists(filepath.Join(root, "CMakeLists.txt")) &&
		(grep(root, "pybind11") || grep(root, "Python_add_library")) {
		return true
	}

	return false
}
