package lane

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDetect_GoProject(t *testing.T) {
	tests := []struct {
		name     string
		files    map[string]string
		expected Result
	}{
		{
			name: "simple go project",
			files: map[string]string{
				"go.mod":  "module test\n\ngo 1.21",
				"main.go": "package main\n\nfunc main() {}",
			},
			expected: Result{
				Lane:     "A",
				Language: "go",
				Reasons:  []string{"go.mod detected"},
			},
		},
		{
			name: "go wasm project",
			files: map[string]string{
				"go.mod": "module test\n\ngo 1.21",
				"main.go": `// +build js,wasm
package main
import "syscall/js"
func main() {}`,
			},
			expected: Result{
				Lane:     "A",
				Language: "go",
				Reasons:  []string{"go.mod detected"},
			},
		},
		{
			name: "go with GOOS/GOARCH wasm",
			files: map[string]string{
				"go.mod":   "module test",
				"build.sh": "GOOS=js GOARCH=wasm go build",
			},
			expected: Result{
				Lane:     "A",
				Language: "go",
				Reasons:  []string{"go.mod detected"},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tmpDir := createTestDir(t, tt.files)
			defer func() { _ = os.RemoveAll(tmpDir) }()

			result := Detect(tmpDir)
			assert.Equal(t, tt.expected.Lane, result.Lane)
			assert.Equal(t, tt.expected.Language, result.Language)
			assert.Equal(t, tt.expected.Reasons, result.Reasons)
		})
	}
}

func TestDetect_RustProject(t *testing.T) {
	tests := []struct {
		name     string
		files    map[string]string
		expected Result
	}{
		{
			name: "simple rust project",
			files: map[string]string{
				"Cargo.toml":  "[package]\nname = \"test\"",
				"src/main.rs": "fn main() {}",
			},
			expected: Result{
				Lane:     "A",
				Language: "rust",
				Reasons:  []string{"Cargo.toml detected"},
			},
		},
		{
			name: "rust wasm project with wasm-bindgen",
			files: map[string]string{
				"Cargo.toml": `[package]
name = "test"

[dependencies]
wasm-bindgen = "0.2"`,
				"src/lib.rs": "use wasm_bindgen::prelude::*;",
			},
			expected: Result{
				Lane:     "A",
				Language: "rust",
				Reasons:  []string{"Cargo.toml detected"},
			},
		},
		{
			name: "rust wasm with cdylib",
			files: map[string]string{
				"Cargo.toml": `[package]
name = "test"

[lib]
crate-type = ["cdylib"]`,
			},
			expected: Result{
				Lane:     "A",
				Language: "rust",
				Reasons:  []string{"Cargo.toml detected"},
			},
		},
		{
			name: "rust with wasm32 target",
			files: map[string]string{
				"Cargo.toml": "[package]\nname = \"test\"",
				"build.sh":   "cargo build --target wasm32-unknown-unknown",
			},
			expected: Result{
				Lane:     "A",
				Language: "rust",
				Reasons:  []string{"Cargo.toml detected"},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tmpDir := createTestDir(t, tt.files)
			defer func() { _ = os.RemoveAll(tmpDir) }()

			result := Detect(tmpDir)
			assert.Equal(t, tt.expected.Lane, result.Lane)
			assert.Equal(t, tt.expected.Language, result.Language)
			assert.Equal(t, tt.expected.Reasons, result.Reasons)
		})
	}
}

func TestDetect_NodeProject(t *testing.T) {
	tests := []struct {
		name     string
		files    map[string]string
		expected Result
	}{
		{
			name: "simple node project",
			files: map[string]string{
				"package.json": `{"name": "test", "version": "1.0.0"}`,
				"index.js":     "console.log('hello');",
			},
			expected: Result{
				Lane:     "B",
				Language: "node",
				Reasons:  []string{"package.json detected"},
			},
		},
		{
			name: "assemblyscript wasm project",
			files: map[string]string{
				"package.json": `{
				"name": "test",
				"devDependencies": {
					"assemblyscript": "^0.27.0"
				}
			}`,
				"assembly/index.ts": "export function add(a: i32, b: i32): i32 { return a + b; }",
			},
			expected: Result{
				Lane:     "B",
				Language: "node",
				Reasons:  []string{"package.json detected"},
			},
		},
		{
			name: "node with .as files",
			files: map[string]string{
				"package.json":  `{"name": "test"}`,
				"src/module.as": "// AssemblyScript code",
			},
			expected: Result{
				Lane:     "B",
				Language: "node",
				Reasons:  []string{"package.json detected"},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tmpDir := createTestDir(t, tt.files)
			defer func() { _ = os.RemoveAll(tmpDir) }()

			result := Detect(tmpDir)
			assert.Equal(t, tt.expected.Lane, result.Lane)
			assert.Equal(t, tt.expected.Language, result.Language)
			assert.Equal(t, tt.expected.Reasons, result.Reasons)
		})
	}
}

func TestDetect_WASMBinaryDefaultsToLaneA(t *testing.T) {
	files := map[string]string{
		"module.wasm": "",
	}
	tmpDir := createTestDir(t, files)
	defer func() { _ = os.RemoveAll(tmpDir) }()

	result := Detect(tmpDir)
	assert.Equal(t, "A", result.Lane)
	assert.Equal(t, "unknown", result.Language)
	assert.Empty(t, result.Reasons)
}

func TestDetect_PythonProject(t *testing.T) {
	tests := []struct {
		name     string
		files    map[string]string
		expected Result
	}{
		{
			name: "simple python project",
			files: map[string]string{
				"requirements.txt": "flask==2.0.0",
				"app.py":           "from flask import Flask",
			},
			expected: Result{
				Lane:     "B",
				Language: "python",
				Reasons:  []string{"python detected"},
			},
		},
		{
			name: "python with pyproject.toml",
			files: map[string]string{
				"pyproject.toml": "[tool.poetry]\nname = \"test\"",
				"main.py":        "print('hello')",
			},
			expected: Result{
				Lane:     "B",
				Language: "python",
				Reasons:  []string{"python detected"},
			},
		},
		{
			name: "python with C extensions",
			files: map[string]string{
				"requirements.txt": "numpy==1.21.0\npandas==1.3.0",
				"setup.py":         "from setuptools import setup, Extension",
			},
			expected: Result{
				Lane:     "C",
				Language: "python",
				Reasons:  []string{"python detected", "Python C-extensions detected - requires full POSIX environment"},
			},
		},
		{
			name: "python with Cython",
			files: map[string]string{
				"pyproject.toml": "[tool.poetry]",
				"module.pyx":     "def hello(): pass",
			},
			expected: Result{
				Lane:     "C",
				Language: "python",
				Reasons:  []string{"python detected", "Python C-extensions detected - requires full POSIX environment"},
			},
		},
		{
			name: "python with native extensions in setup.py",
			files: map[string]string{
				"requirements.txt": "requests",
				"setup.py": `from setuptools import setup
ext_modules = [Extension('module', ['module.c'])]`,
			},
			expected: Result{
				Lane:     "C",
				Language: "python",
				Reasons:  []string{"python detected", "Python C-extensions detected - requires full POSIX environment"},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tmpDir := createTestDir(t, tt.files)
			defer func() { _ = os.RemoveAll(tmpDir) }()

			result := Detect(tmpDir)
			assert.Equal(t, tt.expected.Lane, result.Lane)
			assert.Equal(t, tt.expected.Language, result.Language)
			assert.Equal(t, tt.expected.Reasons, result.Reasons)
		})
	}
}

func TestDetect_JavaProject(t *testing.T) {
	tests := []struct {
		name     string
		files    map[string]string
		expected Result
	}{
		{
			name: "maven java project",
			files: map[string]string{
				"pom.xml":       "<project><groupId>test</groupId></project>",
				"src/Main.java": "public class Main {}",
			},
			expected: Result{
				Lane:     "C",
				Language: "java",
				Reasons:  []string{"Java build tool detected - using OSv for JVM optimization"},
			},
		},
		{
			name: "gradle java project",
			files: map[string]string{
				"build.gradle":  "apply plugin: 'java'",
				"src/Main.java": "public class Main {}",
			},
			expected: Result{
				Lane:     "C",
				Language: "java",
				Reasons:  []string{"Java build tool detected - using OSv for JVM optimization"},
			},
		},
		{
			name: "java with Jib plugin",
			files: map[string]string{
				"build.gradle": `plugins {
					id 'com.google.cloud.tools.jib' version '3.0.0'
				}`,
			},
			expected: Result{
				Lane:     "E",
				Language: "java",
				Reasons:  []string{"Java with Jib plugin detected - optimal for containerless builds"},
			},
		},
		{
			name: "kotlin project",
			files: map[string]string{
				"build.gradle.kts": `plugins {
					id("org.jetbrains.kotlin.jvm") version "1.9.0"
				}`,
			},
			expected: Result{
				Lane:     "C",
				Language: "java",
				Reasons:  []string{"Java build tool detected - using OSv for JVM optimization"},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tmpDir := createTestDir(t, tt.files)
			defer func() { _ = os.RemoveAll(tmpDir) }()

			result := Detect(tmpDir)
			assert.Equal(t, tt.expected.Lane, result.Lane)
			assert.Equal(t, tt.expected.Language, result.Language)
			assert.Equal(t, tt.expected.Reasons, result.Reasons)
		})
	}
}

func TestDetect_ScalaProject(t *testing.T) {
	tests := []struct {
		name     string
		files    map[string]string
		expected Result
	}{
		{
			name: "sbt scala project",
			files: map[string]string{
				"build.sbt":      "name := \"test\"\nscalaVersion := \"2.13.8\"",
				"src/Main.scala": "object Main extends App {}",
			},
			expected: Result{
				Lane:     "C",
				Language: "scala",
				Reasons:  []string{"Scala build.sbt detected - using OSv for JVM optimization"},
			},
		},
		{
			name: "scala with Jib",
			files: map[string]string{
				"build.sbt": `name := "test"
addSbtPlugin("de.gccc.sbt" % "sbt-jib" % "1.0.0")`,
			},
			expected: Result{
				Lane:     "E",
				Language: "scala",
				Reasons:  []string{"Scala with Jib plugin detected - optimal for containerless builds"},
			},
		},
		{
			name: "scala with gradle",
			files: map[string]string{
				"build.gradle":              "apply plugin: 'scala'\ndependencies { implementation 'org.scala-lang:scala-library:2.13.8' }",
				"src/main/scala/Main.scala": "object Main",
			},
			expected: Result{
				Lane:     "C",
				Language: "scala",
				Reasons:  []string{"Scala build tool detected - using OSv for JVM optimization"},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tmpDir := createTestDir(t, tt.files)
			defer func() { _ = os.RemoveAll(tmpDir) }()

			result := Detect(tmpDir)
			assert.Equal(t, tt.expected.Lane, result.Lane)
			assert.Equal(t, tt.expected.Language, result.Language)
			assert.Equal(t, tt.expected.Reasons, result.Reasons)
		})
	}
}

func TestDetect_DotNetProject(t *testing.T) {
	tests := []struct {
		name     string
		files    map[string]string
		expected Result
	}{
		{
			name: "dotnet project",
			files: map[string]string{
				"MyApp.csproj": "<Project Sdk=\"Microsoft.NET.Sdk\"></Project>",
				"Program.cs":   "Console.WriteLine(\"Hello\");",
			},
			expected: Result{
				Lane:     "C",
				Language: ".net",
				Reasons:  []string{".csproj detected"},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tmpDir := createTestDir(t, tt.files)
			defer func() { _ = os.RemoveAll(tmpDir) }()

			result := Detect(tmpDir)
			assert.Equal(t, tt.expected.Lane, result.Lane)
			assert.Equal(t, tt.expected.Language, result.Language)
			assert.Equal(t, tt.expected.Reasons, result.Reasons)
		})
	}
}

func TestDetect_POSIXHeavy(t *testing.T) {
	tests := []struct {
		name     string
		files    map[string]string
		expected Result
	}{
		{
			name: "code using fork",
			files: map[string]string{
				"main.c": `#include <unistd.h>
int main() { 
	pid_t pid = fork();
	return 0;
}`,
			},
			expected: Result{
				Lane:     "C",
				Language: "unknown",
				Reasons:  []string{"POSIX-heavy features detected"},
			},
		},
		{
			name: "code accessing /proc",
			files: map[string]string{
				"requirements.txt": "psutil",
				"monitor.py": `import os
with open('/proc/meminfo') as f:
	print(f.read())`,
			},
			expected: Result{
				Lane:     "C",
				Language: "python",
				Reasons:  []string{"python detected", "POSIX-heavy features detected"},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tmpDir := createTestDir(t, tt.files)
			defer func() { _ = os.RemoveAll(tmpDir) }()

			result := Detect(tmpDir)
			assert.Equal(t, tt.expected.Lane, result.Lane)
			assert.Equal(t, tt.expected.Language, result.Language)
			// For multiple reasons, just check that expected reasons are present
			for _, expectedReason := range tt.expected.Reasons {
				assert.Contains(t, result.Reasons, expectedReason)
			}
		})
	}
}

func TestDetect_PriorityHandling(t *testing.T) {
	tests := []struct {
		name     string
		files    map[string]string
		expected Result
	}{
		{
			name: "Python C-extensions override basic Python",
			files: map[string]string{
				"requirements.txt": "numpy\nscipy\npandas",
				"app.py":           "import numpy as np",
			},
			expected: Result{
				Lane:     "C",
				Language: "python",
				Reasons:  []string{"python detected", "Python C-extensions detected - requires full POSIX environment"},
			},
		},
		{
			name: "Jib plugin overrides basic Java",
			files: map[string]string{
				"pom.xml": `<project>
					<build>
						<plugins>
							<plugin>
								<groupId>com.google.cloud.tools</groupId>
								<artifactId>jib-maven-plugin</artifactId>
							</plugin>
						</plugins>
					</build>
				</project>`,
			},
			expected: Result{
				Lane:     "E",
				Language: "java",
				Reasons:  []string{"Java with Jib plugin detected - optimal for containerless builds"},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tmpDir := createTestDir(t, tt.files)
			defer func() { _ = os.RemoveAll(tmpDir) }()

			result := Detect(tmpDir)
			assert.Equal(t, tt.expected.Lane, result.Lane)
			assert.Equal(t, tt.expected.Language, result.Language)
			assert.Equal(t, tt.expected.Reasons, result.Reasons)
		})
	}
}

func TestDetect_UnknownProject(t *testing.T) {
	tmpDir := createTestDir(t, map[string]string{
		"README.md": "# Test Project",
		"data.txt":  "some data",
	})
	defer func() { _ = os.RemoveAll(tmpDir) }()

	result := Detect(tmpDir)
	assert.Equal(t, "A", result.Lane)
	assert.Equal(t, "unknown", result.Language)
	assert.Empty(t, result.Reasons)
}

func TestDetect_EmptyDirectory(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "test-detect-")
	require.NoError(t, err)
	defer func() { _ = os.RemoveAll(tmpDir) }()

	result := Detect(tmpDir)
	assert.Equal(t, "A", result.Lane)
	assert.Equal(t, "unknown", result.Language)
	assert.Empty(t, result.Reasons)
}

func TestDetect_NonExistentDirectory(t *testing.T) {
	result := Detect("/non/existent/path")
	assert.Equal(t, "A", result.Lane)
	assert.Equal(t, "unknown", result.Language)
	assert.Empty(t, result.Reasons)
}

// Benchmark tests
func BenchmarkDetect_SimpleProject(b *testing.B) {
	tmpDir := createTestDir(b, map[string]string{
		"go.mod":  "module test",
		"main.go": "package main\nfunc main() {}",
	})
	defer func() { _ = os.RemoveAll(tmpDir) }()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = Detect(tmpDir)
	}
}

func BenchmarkDetect_ComplexProject(b *testing.B) {
	files := map[string]string{
		"go.mod":     "module test",
		"main.go":    "package main",
		"Dockerfile": "FROM golang:1.21",
		"Makefile":   "build:\n\tgo build",
		"README.md":  "# Project",
		".gitignore": "*.exe\n*.dll",
	}
	// Add more files to simulate a complex project
	for i := 0; i < 10; i++ {
		files[filepath.Join("pkg", fmt.Sprintf("module%d", i), "file.go")] = "package module"
		files[filepath.Join("internal", fmt.Sprintf("service%d", i), "service.go")] = "package service"
	}

	tmpDir := createTestDir(b, files)
	defer func() { _ = os.RemoveAll(tmpDir) }()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = Detect(tmpDir)
	}
}

// Helper function to create test directory with files
func createTestDir(t testing.TB, files map[string]string) string {
	t.Helper()

	tmpDir, err := os.MkdirTemp("", "test-detect-")
	require.NoError(t, err)

	for path, content := range files {
		fullPath := filepath.Join(tmpDir, path)
		dir := filepath.Dir(fullPath)

		// Create directory if needed
		if dir != tmpDir {
			err := os.MkdirAll(dir, 0755)
			require.NoError(t, err)
		}

		// Write file
		err := os.WriteFile(fullPath, []byte(content), 0644)
		require.NoError(t, err)
	}

	return tmpDir
}
