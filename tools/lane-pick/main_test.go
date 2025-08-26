package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDetect(t *testing.T) {
	tests := []struct {
		name           string
		files          map[string]string // relative path -> content
		expectedLane   string
		expectedLang   string
		expectedReasons []string
		description    string
	}{
		// Go Projects - Lane A
		{
			name: "Go project with go.mod",
			files: map[string]string{
				"go.mod": "module example.com/app\n\ngo 1.21",
				"main.go": "package main\n\nfunc main() {}",
			},
			expectedLane: "A",
			expectedLang: "go",
			expectedReasons: []string{"go.mod detected"},
			description: "Standard Go project should use Lane A",
		},
		
		// Node.js Projects - Lane B
		{
			name: "Node.js project with package.json",
			files: map[string]string{
				"package.json": `{"name": "test-app", "version": "1.0.0"}`,
				"index.js": "console.log('hello world');",
			},
			expectedLane: "B",
			expectedLang: "node",
			expectedReasons: []string{"package.json detected"},
			description: "Standard Node.js project should use Lane B",
		},
		
		// Python Projects - Lane B (simple)
		{
			name: "Python project without C extensions",
			files: map[string]string{
				"requirements.txt": "flask==2.0.1\nrequests==2.25.1",
				"app.py": "from flask import Flask\napp = Flask(__name__)",
			},
			expectedLane: "B",
			expectedLang: "python",
			expectedReasons: []string{"python detected"},
			description: "Pure Python project should use Lane B",
		},
		
		// Python Projects - Lane C (with C extensions)
		{
			name: "Python project with C extensions",
			files: map[string]string{
				"requirements.txt": "numpy==1.21.0\nscipy==1.7.0",
				"setup.py": "from setuptools import setup, Extension\next_modules = [Extension('_myext', ['myext.c'])]",
				"myext.c": "#include <Python.h>\nstatic PyObject* hello() { return PyUnicode_FromString(\"hello\"); }",
			},
			expectedLane: "C",
			expectedLang: "python",
			expectedReasons: []string{"python detected", "Python C-extensions detected - requires full POSIX environment"},
			description: "Python project with C extensions should use Lane C",
		},
		
		// Java Projects - Lane C (standard)
		{
			name: "Java project with Maven",
			files: map[string]string{
				"pom.xml": `<project>
					<groupId>com.example</groupId>
					<artifactId>test-app</artifactId>
					<version>1.0.0</version>
				</project>`,
				"src/main/java/Main.java": "public class Main { public static void main(String[] args) {} }",
			},
			expectedLane: "C",
			expectedLang: "java",
			expectedReasons: []string{"Java build tool detected - using OSv for JVM optimization"},
			description: "Standard Java project should use Lane C",
		},
		
		// Java Projects - Lane E (with Jib)
		{
			name: "Java project with Jib plugin",
			files: map[string]string{
				"pom.xml": `<project>
					<plugins>
						<plugin>
							<groupId>com.google.cloud.tools</groupId>
							<artifactId>jib-maven-plugin</artifactId>
							<version>3.1.4</version>
						</plugin>
					</plugins>
				</project>`,
				"src/main/java/Main.java": "public class Main { public static void main(String[] args) {} }",
			},
			expectedLane: "E",
			expectedLang: "java",
			expectedReasons: []string{"Java with Jib plugin detected - optimal for containerless builds"},
			description: "Java project with Jib should use Lane E",
		},
		
		// Scala Projects - Lane C (standard)
		{
			name: "Scala project with SBT",
			files: map[string]string{
				"build.sbt": `name := "test-app"\nversion := "1.0.0"\nscalaVersion := "2.13.8"`,
				"src/main/scala/Main.scala": "object Main extends App { println(\"hello\") }",
			},
			expectedLane: "C",
			expectedLang: "scala",
			expectedReasons: []string{"Scala build.sbt detected - using OSv for JVM optimization"},
			description: "Standard Scala project should use Lane C",
		},
		
		// .NET Projects - Lane C
		{
			name: ".NET project with csproj",
			files: map[string]string{
				"MyApp.csproj": `<Project Sdk="Microsoft.NET.Sdk">
					<PropertyGroup>
						<TargetFramework>net6.0</TargetFramework>
					</PropertyGroup>
				</Project>`,
				"Program.cs": "using System;\nConsole.WriteLine(\"Hello World!\");",
			},
			expectedLane: "C",
			expectedLang: ".net",
			expectedReasons: []string{".csproj detected"},
			description: ".NET project should use Lane C",
		},
		
		// Rust Projects - Lane A
		{
			name: "Rust project with Cargo.toml",
			files: map[string]string{
				"Cargo.toml": `[package]
name = "test-app"
version = "0.1.0"
edition = "2021"`,
				"src/main.rs": "fn main() { println!(\"Hello, world!\"); }",
			},
			expectedLane: "A",
			expectedLang: "rust",
			expectedReasons: []string{"Cargo.toml detected"},
			description: "Standard Rust project should use Lane A",
		},
		
		// WASM Projects - Lane G
		{
			name: "Rust WASM project",
			files: map[string]string{
				"Cargo.toml": `[package]
name = "wasm-app"
version = "0.1.0"

[dependencies]
wasm-bindgen = "0.2"

[lib]
crate-type = ["cdylib"]`,
				"src/lib.rs": "use wasm_bindgen::prelude::*;\n#[wasm_bindgen]\nextern \"C\" { fn alert(s: &str); }",
			},
			expectedLane: "G",
			expectedLang: "rust",
			expectedReasons: []string{"Rust wasm32 target detected"},
			description: "Rust WASM project should use Lane G",
		},
		
		{
			name: "Go WASM project",
			files: map[string]string{
				"go.mod": "module wasm-app\n\ngo 1.21",
				"main.go": "//go:build js && wasm\npackage main\n\nimport \"syscall/js\"\n\nfunc main() {}",
			},
			expectedLane: "G",
			expectedLang: "go",
			expectedReasons: []string{"Go js/wasm target detected"},
			description: "Go WASM project should use Lane G",
		},
		
		{
			name: "AssemblyScript WASM project",
			files: map[string]string{
				"package.json": `{"name": "wasm-app", "dependencies": {"assemblyscript": "^0.20.0"}}`,
				"assembly/index.ts": "export function add(a: i32, b: i32): i32 { return a + b; }",
			},
			expectedLane: "G",
			expectedLang: "assemblyscript",
			expectedReasons: []string{"AssemblyScript configuration detected"},
			description: "AssemblyScript project should use Lane G",
		},
		
		// POSIX-heavy detection
		{
			name: "Go project with POSIX features",
			files: map[string]string{
				"go.mod": "module posix-app\n\ngo 1.21",
				"main.go": `package main
import (
	"os"
	"syscall"
)
func main() {
	syscall.Syscall(syscall.SYS_FORK, 0, 0, 0)
	os.ReadFile("/proc/cpuinfo")
}`,
			},
			expectedLane: "C",
			expectedLang: "go",
			expectedReasons: []string{"go.mod detected", "POSIX-heavy features detected"},
			description: "Project with POSIX features should be upgraded to Lane C",
		},
		
		// Unknown/default projects
		{
			name: "Unknown project type",
			files: map[string]string{
				"README.md": "# My App\nThis is a generic application",
				"config.ini": "[app]\nname=myapp",
			},
			expectedLane: "A",
			expectedLang: "unknown",
			expectedReasons: []string{},
			description: "Unknown project should default to Lane A",
		},
		
		// Edge case: Multiple language indicators (Python gets detected last and overrides)
		{
			name: "Multi-language project - Python C-extensions override",
			files: map[string]string{
				"go.mod": "module multi-app\n\ngo 1.21",
				"package.json": `{"name": "multi-app"}`,
				"requirements.txt": "numpy==1.21.0",  // Will trigger C-extensions detection
				"main.go": "package main\nfunc main() {}",
			},
			expectedLane: "C", // Python C-extensions should upgrade to Lane C
			expectedLang: "python",
			expectedReasons: []string{"go.mod detected", "package.json detected", "python detected", "Python C-extensions detected - requires full POSIX environment"},
			description: "Multi-language project with Python C-extensions should use Lane C",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create temporary directory
			tmpDir, err := os.MkdirTemp("", "lane-test-")
			require.NoError(t, err)
			defer os.RemoveAll(tmpDir)

			// Create test files
			for relativePath, content := range tt.files {
				fullPath := filepath.Join(tmpDir, relativePath)
				err := os.MkdirAll(filepath.Dir(fullPath), 0755)
				require.NoError(t, err)
				
				err = os.WriteFile(fullPath, []byte(content), 0644)
				require.NoError(t, err)
			}

			// Run detection
			result := detect(tmpDir)

			// Verify results
			assert.Equal(t, tt.expectedLane, result.Lane, "Lane mismatch: %s", tt.description)
			assert.Equal(t, tt.expectedLang, result.Language, "Language mismatch: %s", tt.description)
			
			// Check that expected reasons are present
			for _, expectedReason := range tt.expectedReasons {
				assert.Contains(t, result.Reasons, expectedReason, 
					"Missing expected reason '%s' in %v: %s", expectedReason, result.Reasons, tt.description)
			}
		})
	}
}

func TestHelperFunctions(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "helper-test-")
	require.NoError(t, err)
	defer os.RemoveAll(tmpDir)

	t.Run("exists function", func(t *testing.T) {
		// Create a test file
		testFile := filepath.Join(tmpDir, "test.txt")
		err := os.WriteFile(testFile, []byte("test content"), 0644)
		require.NoError(t, err)

		assert.True(t, exists(testFile), "Should detect existing file")
		assert.False(t, exists(filepath.Join(tmpDir, "nonexistent.txt")), "Should not detect non-existent file")
	})

	t.Run("hasAny function", func(t *testing.T) {
		// Create nested directories with specific files
		subDir := filepath.Join(tmpDir, "src", "main", "java")
		err := os.MkdirAll(subDir, 0755)
		require.NoError(t, err)
		
		javaFile := filepath.Join(subDir, "Main.java")
		err = os.WriteFile(javaFile, []byte("public class Main {}"), 0644)
		require.NoError(t, err)

		assert.True(t, hasAny(tmpDir, ".java"), "Should find .java files recursively")
		assert.False(t, hasAny(tmpDir, ".cpp"), "Should not find non-existent .cpp files")
	})

	t.Run("grep function", func(t *testing.T) {
		// Create a Go file with specific content
		goFile := filepath.Join(tmpDir, "main.go")
		goContent := `package main
import (
	"os"
	"syscall"
)
func main() {
	syscall.Syscall(syscall.SYS_FORK, 0, 0, 0)
}`
		err := os.WriteFile(goFile, []byte(goContent), 0644)
		require.NoError(t, err)

		assert.True(t, grep(tmpDir, "SYS_FORK"), "Should find 'SYS_FORK' in Go files")
		assert.False(t, grep(tmpDir, "nonexistent_function"), "Should not find non-existent patterns")
	})
}

func TestWASMDetection(t *testing.T) {
	tests := []struct {
		name          string
		files         map[string]string
		expectWASM    bool
		expectReasons []string
		description   string
	}{
		{
			name: "Direct WASM files",
			files: map[string]string{
				"app.wasm": "binary wasm content",
				"utils.wat": "(module (func (export \"test\")))",
			},
			expectWASM: true,
			expectReasons: []string{"Direct WASM files (.wasm/.wat) detected"},
			description: "Should detect direct .wasm/.wat files",
		},
		{
			name: "Rust WASM project",
			files: map[string]string{
				"Cargo.toml": `[dependencies]
wasm-bindgen = "0.2"
web-sys = "0.3"`,
				"src/lib.rs": "use wasm_bindgen::prelude::*;",
			},
			expectWASM: true,
			expectReasons: []string{"Rust wasm32 target detected"},
			description: "Should detect Rust WASM dependencies",
		},
		{
			name: "Emscripten C++ project",
			files: map[string]string{
				"CMakeLists.txt": "set_target_properties(app PROPERTIES COMPILE_FLAGS \"-s WASM=1\")",
				"main.cpp": "#include <emscripten.h>\nEMSCRIPTEN_KEEPALIVE int add(int a, int b) { return a + b; }",
			},
			expectWASM: true,
			expectReasons: []string{"C++ Emscripten configuration detected"},
			description: "Should detect Emscripten C++ projects",
		},
		{
			name: "Non-WASM Go project",
			files: map[string]string{
				"go.mod": "module regular-app\ngo 1.21",
				"main.go": "package main\nfunc main() { println(\"hello\") }",
			},
			expectWASM: false,
			expectReasons: []string{},
			description: "Should not detect WASM in regular Go projects",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tmpDir, err := os.MkdirTemp("", "wasm-test-")
			require.NoError(t, err)
			defer os.RemoveAll(tmpDir)

			// Create test files
			for relativePath, content := range tt.files {
				fullPath := filepath.Join(tmpDir, relativePath)
				err := os.MkdirAll(filepath.Dir(fullPath), 0755)
				require.NoError(t, err)
				
				err = os.WriteFile(fullPath, []byte(content), 0644)
				require.NoError(t, err)
			}

			detected, reasons := detectWASM(tmpDir)
			
			assert.Equal(t, tt.expectWASM, detected, "WASM detection mismatch: %s", tt.description)
			
			if tt.expectWASM {
				for _, expectedReason := range tt.expectReasons {
					assert.Contains(t, reasons, expectedReason, 
						"Missing expected WASM reason '%s' in %v: %s", expectedReason, reasons, tt.description)
				}
			}
		})
	}
}

func TestJibDetection(t *testing.T) {
	tests := []struct {
		name        string
		files       map[string]string
		expectJib   bool
		description string
	}{
		{
			name: "Maven Jib plugin",
			files: map[string]string{
				"pom.xml": `<plugin>
					<groupId>com.google.cloud.tools</groupId>
					<artifactId>jib-maven-plugin</artifactId>
				</plugin>`,
			},
			expectJib: true,
			description: "Should detect Maven Jib plugin",
		},
		{
			name: "Gradle Jib plugin",
			files: map[string]string{
				"build.gradle": `plugins {
					id 'com.google.cloud.tools.jib' version '3.1.4'
				}`,
			},
			expectJib: true,
			description: "Should detect Gradle Jib plugin",
		},
		{
			name: "SBT Jib plugin",
			files: map[string]string{
				"build.sbt": "enablePlugins(JibPlugin)\naddSbtPlugin(\"com.github.sbt\" % \"sbt-jib\" % \"0.1.0\")",
			},
			expectJib: true,
			description: "Should detect SBT Jib plugin",
		},
		{
			name: "No Jib plugin",
			files: map[string]string{
				"pom.xml": "<project><artifactId>regular-app</artifactId></project>",
			},
			expectJib: false,
			description: "Should not detect Jib in regular projects",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tmpDir, err := os.MkdirTemp("", "jib-test-")
			require.NoError(t, err)
			defer os.RemoveAll(tmpDir)

			// Create test files
			for relativePath, content := range tt.files {
				fullPath := filepath.Join(tmpDir, relativePath)
				err := os.MkdirAll(filepath.Dir(fullPath), 0755)
				require.NoError(t, err)
				
				err = os.WriteFile(fullPath, []byte(content), 0644)
				require.NoError(t, err)
			}

			detected := hasJibPlugin(tmpDir)
			assert.Equal(t, tt.expectJib, detected, "Jib detection mismatch: %s", tt.description)
		})
	}
}

func TestPythonCExtensionsDetection(t *testing.T) {
	tests := []struct {
		name        string
		files       map[string]string
		expectCExt  bool
		description string
	}{
		{
			name: "Python with C source files",
			files: map[string]string{
				"setup.py": "from setuptools import setup, Extension",
				"myext.c": "#include <Python.h>\nstatic PyObject* hello() { return Py_None; }",
			},
			expectCExt: true,
			description: "Should detect C source files",
		},
		{
			name: "Python with Cython files",
			files: map[string]string{
				"myext.pyx": "def hello(): return 'hello'",
				"setup.py": "from Cython.Build import cythonize",
			},
			expectCExt: true,
			description: "Should detect Cython files",
		},
		{
			name: "Python with C extension libraries",
			files: map[string]string{
				"requirements.txt": "numpy==1.21.0\nscipy==1.7.0\npandas==1.3.0",
			},
			expectCExt: true,
			description: "Should detect C-extension dependencies",
		},
		{
			name: "Pure Python project",
			files: map[string]string{
				"requirements.txt": "flask==2.0.1\nrequests==2.25.1",
				"app.py": "from flask import Flask\napp = Flask(__name__)",
			},
			expectCExt: false,
			description: "Should not detect C extensions in pure Python",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tmpDir, err := os.MkdirTemp("", "python-test-")
			require.NoError(t, err)
			defer os.RemoveAll(tmpDir)

			// Create test files
			for relativePath, content := range tt.files {
				fullPath := filepath.Join(tmpDir, relativePath)
				err := os.MkdirAll(filepath.Dir(fullPath), 0755)
				require.NoError(t, err)
				
				err = os.WriteFile(fullPath, []byte(content), 0644)
				require.NoError(t, err)
			}

			detected := hasPythonCExtensions(tmpDir)
			assert.Equal(t, tt.expectCExt, detected, "Python C extensions detection mismatch: %s", tt.description)
		})
	}
}

func TestResultJSONSerialization(t *testing.T) {
	result := Result{
		Lane:     "C",
		Language: "java",
		Reasons:  []string{"Java build tool detected", "POSIX features detected"},
	}

	jsonData, err := json.Marshal(result)
	require.NoError(t, err)

	var unmarshaled Result
	err = json.Unmarshal(jsonData, &unmarshaled)
	require.NoError(t, err)

	assert.Equal(t, result.Lane, unmarshaled.Lane)
	assert.Equal(t, result.Language, unmarshaled.Language)
	assert.Equal(t, result.Reasons, unmarshaled.Reasons)
}

func TestContains(t *testing.T) {
	testSlice := []string{"Go WASM target detected", "Rust compilation found", "Java build detected"}
	
	assert.True(t, contains(testSlice, "WASM"), "Should find partial match for WASM")
	assert.True(t, contains(testSlice, "Rust"), "Should find partial match for Rust") 
	assert.False(t, contains(testSlice, "Python"), "Should not find non-existent match")
	assert.False(t, contains([]string{}, "anything"), "Should handle empty slice")
}