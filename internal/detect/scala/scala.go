package scala

import (
    "os"
    "path/filepath"
    "regexp"
)

// DetectVersion attempts to detect Scala version from Gradle build files.
func DetectVersion(srcDir string) string {
    if v := detectFromGradleKts(filepath.Join(srcDir, "build.gradle.kts")); v != "" { return v }
    if v := detectFromGradle(filepath.Join(srcDir, "build.gradle")); v != "" { return v }
    return ""
}

func detectFromGradleKts(path string) string {
    b, err := os.ReadFile(path)
    if err != nil { return "" }
    // implementation("org.scala-lang:scala-library:2.13.14")
    re := regexp.MustCompile(`org\.scala\-lang:scala\-library:(\d+\.\d+\.\d+)`)
    if re.Match(b) { return re.FindStringSubmatch(string(b))[1] }
    return ""
}

func detectFromGradle(path string) string {
    b, err := os.ReadFile(path)
    if err != nil { return "" }
    re := regexp.MustCompile(`org\.scala\-lang:scala\-library:(\d+\.\d+\.\d+)`)
    if re.Match(b) { return re.FindStringSubmatch(string(b))[1] }
    return ""
}

