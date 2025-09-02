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

	// WASM Detection Logic - Lane G (priority check)
	if wasmDetected, wasmReasons := detectWASM(root); wasmDetected {
		lane = "G"
		reasons = append(reasons, wasmReasons...)
		
		// Determine language from WASM context
		if contains(wasmReasons, "Rust") {
			lang = "rust"
		} else if contains(wasmReasons, "Go") {
			lang = "go" 
		} else if contains(wasmReasons, "AssemblyScript") {
			lang = "assemblyscript"
		} else if contains(wasmReasons, "C++") || contains(wasmReasons, "Emscripten") {
			lang = "cpp"
		} else {
			lang = "wasm"
		}
		return Result{ Lane: lane, Language: lang, Reasons: reasons }
	}

	// Simple language detection
	if exists(filepath.Join(root, "go.mod")) { lang = "go"; lane = "A"; reasons = append(reasons, "go.mod detected") }
	if exists(filepath.Join(root, "Cargo.toml")) { lang = "rust"; lane = "A"; reasons = append(reasons, "Cargo.toml detected") }
	if exists(filepath.Join(root, "package.json")) { lang = "node"; lane = "B"; reasons = append(reasons, "package.json detected") }
	if exists(filepath.Join(root, "pyproject.toml")) || exists(filepath.Join(root, "requirements.txt")) { 
		lang = "python"; lane = "B"; reasons = append(reasons, "python detected")
		// Enhanced C extensions detection
		if hasPythonCExtensions(root) {
			lane = "C"; reasons = append(reasons, "Python C-extensions detected - requires full POSIX environment")
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
			   strings.HasSuffix(p, ".cpp") || strings.HasSuffix(p, ".cxx") ||
			   strings.HasSuffix(p, ".go") || strings.HasSuffix(p, ".rs") || 
			   strings.HasSuffix(p, ".js") || strings.HasSuffix(p, ".ts") || 
			   strings.HasSuffix(p, ".py") || strings.HasSuffix(p, ".pyx") ||
			   strings.HasSuffix(p, ".gradle") || strings.HasSuffix(p, ".gradle.kts") || 
			   strings.HasSuffix(p, ".kts") || strings.HasSuffix(p, "build.sbt") || 
			   strings.HasSuffix(p, "pom.xml") || strings.HasSuffix(p, "setup.py") ||
			   strings.HasSuffix(p, "pyproject.toml") || strings.HasSuffix(p, "requirements.txt") ||
			   strings.HasSuffix(p, "CMakeLists.txt") || strings.HasSuffix(p, ".toml") ||
			   strings.HasSuffix(p, ".json") {
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

// detectWASM implements comprehensive WASM detection logic
func detectWASM(root string) (bool, []string) {
	reasons := []string{}
	
	// Direct WASM files detection
	if hasAny(root, ".wasm") || hasAny(root, ".wat") {
		reasons = append(reasons, "Direct WASM files (.wasm/.wat) detected")
		return true, reasons
	}
	
	// Rust WASM target detection
	if exists(filepath.Join(root, "Cargo.toml")) {
		if hasRustWASMTarget(root) {
			reasons = append(reasons, "Rust wasm32 target detected")
			return true, reasons
		}
	}
	
	// Go WASM target detection
	if exists(filepath.Join(root, "go.mod")) {
		if hasGoWASMTarget(root) {
			reasons = append(reasons, "Go js/wasm target detected")
			return true, reasons
		}
	}
	
	// AssemblyScript detection
	if exists(filepath.Join(root, "package.json")) {
		if hasAssemblyScriptConfig(root) {
			reasons = append(reasons, "AssemblyScript configuration detected")
			return true, reasons
		}
	}
	
	// C/C++ Emscripten detection
	if hasEmscriptenConfig(root) {
		reasons = append(reasons, "C++ Emscripten configuration detected")
		return true, reasons
	}
	
	return false, reasons
}

// hasRustWASMTarget detects Rust projects targeting WASM
func hasRustWASMTarget(root string) bool {
	// Check for WASM-specific dependencies
	if grep(root, "wasm-bindgen") || grep(root, "js-sys") || grep(root, "web-sys") || grep(root, "wasi") {
		return true
	}
	
	// Check for cdylib crate type (common for WASM)
	if grep(root, "crate-type.*cdylib") {
		return true
	}
	
	// Check for wasm32 target in build scripts
	if grep(root, "wasm32-unknown-unknown") || grep(root, "wasm32-wasi") {
		return true
	}
	
	// Check for wasm-pack configuration
	if grep(root, "wasm-pack") {
		return true
	}
	
	return false
}

// hasGoWASMTarget detects Go projects targeting WASM
func hasGoWASMTarget(root string) bool {
	// Check for js,wasm build tags
	if grep(root, "js,wasm") || grep(root, "js && wasm") {
		return true
	}
	
	// Check for syscall/js imports (WASM-specific)
	if grep(root, "syscall/js") {
		return true
	}
	
	// Check for GOOS/GOARCH environment in build scripts
	if grep(root, "GOOS=js") && grep(root, "GOARCH=wasm") {
		return true
	}
	
	return false
}

// hasAssemblyScriptConfig detects AssemblyScript projects
func hasAssemblyScriptConfig(root string) bool {
	// Check package.json for AssemblyScript dependencies
	if grep(root, "assemblyscript") {
		return true
	}
	
	// Check for .asc or .as files
	if hasAny(root, ".asc") || hasAny(root, ".as") {
		return true
	}
	
	// Check for AssemblyScript build scripts
	if grep(root, "asc ") || grep(root, "asbuild") {
		return true
	}
	
	return false
}

// hasEmscriptenConfig detects C/C++ projects using Emscripten
func hasEmscriptenConfig(root string) bool {
	// Check for .emscripten config file
	if exists(filepath.Join(root, ".emscripten")) {
		return true
	}
	
	// Check for Emscripten in CMakeLists.txt
	if grep(root, "emscripten") || grep(root, "Emscripten") {
		return true
	}
	
	// Check for emcc compiler usage
	if grep(root, "emcc") || grep(root, "em++") {
		return true
	}
	
	// Check for Emscripten-specific headers
	if grep(root, "emscripten.h") || grep(root, "emscripten/bind.h") {
		return true
	}
	
	// Check for WASM export attributes
	if grep(root, "EMSCRIPTEN_KEEPALIVE") || grep(root, "EXPORTED_FUNCTIONS") {
		return true
	}
	
	// Check for typical Emscripten build flags
	if grep(root, "-s WASM=1") || grep(root, "--target=wasm32") {
		return true
	}
	
	return false
}

// contains checks if a slice contains a substring
func contains(slice []string, substr string) bool {
	for _, item := range slice {
		if strings.Contains(item, substr) {
			return true
		}
	}
	return false
}
