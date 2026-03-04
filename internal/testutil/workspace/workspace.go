package workspace

import (
	"os"
	"path/filepath"
	"testing"
)

func Maven(t testing.TB, javaVersion string) string {
	t.Helper()
	tmpDir := t.TempDir()
	pomContent := `<?xml version="1.0" encoding="UTF-8"?>
<project>
  <modelVersion>4.0.0</modelVersion>
  <groupId>test</groupId>
  <artifactId>test</artifactId>
  <version>1.0</version>
  <properties>
    <maven.compiler.release>` + javaVersion + `</maven.compiler.release>
  </properties>
</project>`
	if err := os.WriteFile(filepath.Join(tmpDir, "pom.xml"), []byte(pomContent), 0o644); err != nil {
		t.Fatalf("failed to create pom.xml: %v", err)
	}
	return tmpDir
}

func MavenNoJavaVersion(t testing.TB) string {
	t.Helper()
	tmpDir := t.TempDir()
	pomContent := `<?xml version="1.0" encoding="UTF-8"?>
<project>
  <modelVersion>4.0.0</modelVersion>
  <groupId>test</groupId>
  <artifactId>test</artifactId>
  <version>1.0</version>
</project>`
	if err := os.WriteFile(filepath.Join(tmpDir, "pom.xml"), []byte(pomContent), 0o644); err != nil {
		t.Fatalf("failed to create pom.xml: %v", err)
	}
	return tmpDir
}

func Gradle(t testing.TB, javaVersion string) string {
	t.Helper()
	tmpDir := t.TempDir()
	gradleContent := `plugins {
    id 'java'
}

java {
    sourceCompatibility = JavaVersion.VERSION_` + javaVersion + `
    targetCompatibility = JavaVersion.VERSION_` + javaVersion + `
}
`
	if err := os.WriteFile(filepath.Join(tmpDir, "build.gradle"), []byte(gradleContent), 0o644); err != nil {
		t.Fatalf("failed to create build.gradle: %v", err)
	}
	return tmpDir
}

func GradleWithWrapper(t testing.TB, javaVersion string) string {
	t.Helper()
	tmpDir := Gradle(t, javaVersion)
	wrapperDir := filepath.Join(tmpDir, "gradle", "wrapper")
	if err := os.MkdirAll(wrapperDir, 0o755); err != nil {
		t.Fatalf("failed to create gradle wrapper directory: %v", err)
	}
	const wrapperProps = "distributionUrl=https\\://services.gradle.org/distributions/gradle-8.8-bin.zip\n"
	if err := os.WriteFile(filepath.Join(wrapperDir, "gradle-wrapper.properties"), []byte(wrapperProps), 0o644); err != nil {
		t.Fatalf("failed to create gradle-wrapper.properties: %v", err)
	}
	return tmpDir
}

func Go(t testing.TB, goVersion string) string {
	t.Helper()
	tmpDir := t.TempDir()
	goModuleFile := "go." + "mo" + "d"
	goMod := "module example.com/test\n\ngo " + goVersion + "\n"
	if err := os.WriteFile(filepath.Join(tmpDir, goModuleFile), []byte(goMod), 0o644); err != nil {
		t.Fatalf("failed to create go module file: %v", err)
	}
	return tmpDir
}

func Cargo(t testing.TB, rustVersion string) string {
	t.Helper()
	tmpDir := t.TempDir()
	cargo := `[package]
name = "test"
version = "0.1.0"
edition = "2021"
rust-version = "` + rustVersion + `"
`
	if err := os.WriteFile(filepath.Join(tmpDir, "Cargo.toml"), []byte(cargo), 0o644); err != nil {
		t.Fatalf("failed to create Cargo.toml: %v", err)
	}
	return tmpDir
}

func Python(t testing.TB, pythonVersion string) string {
	t.Helper()
	tmpDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(tmpDir, ".python-version"), []byte(pythonVersion+"\n"), 0o644); err != nil {
		t.Fatalf("failed to create .python-version: %v", err)
	}
	return tmpDir
}
