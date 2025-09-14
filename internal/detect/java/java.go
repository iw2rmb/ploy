package java

import (
	"encoding/xml"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// DetectVersion attempts to detect the Java version used by a project located at srcDir.
// It scans common Gradle (KTS/Groovy), gradle.properties, Maven pom.xml, and .java-version files.
// Returns a version string like "21", "17", "11", or "8". Empty string if unknown.
func DetectVersion(srcDir string) string {
	if v := detectFromGradleKts(filepath.Join(srcDir, "build.gradle.kts")); v != "" {
		return v
	}
	if v := detectFromGradle(filepath.Join(srcDir, "build.gradle")); v != "" {
		return v
	}
	if v := detectFromGradleProperties(filepath.Join(srcDir, "gradle.properties")); v != "" {
		return v
	}
	if v := detectFromPom(filepath.Join(srcDir, "pom.xml")); v != "" {
		return v
	}
	if v := detectFromJavaVersionFile(filepath.Join(srcDir, ".java-version")); v != "" {
		return v
	}
	return ""
}

// DetectMainClass attempts to detect the JVM main class from Gradle settings (KTS/Groovy) or Jib container configuration.
func DetectMainClass(srcDir string) string {
	if v := detectMainFromGradleKts(filepath.Join(srcDir, "build.gradle.kts")); v != "" {
		return v
	}
	if v := detectMainFromGradle(filepath.Join(srcDir, "build.gradle")); v != "" {
		return v
	}
	return ""
}

// DetectJib checks whether the project config references the Jib plugin
func DetectJib(srcDir string) bool {
	// Gradle KTS/Groovy: id("com.google.cloud.tools.jib") or plugins { id 'com.google.cloud.tools.jib' }
	if txt := read(filepath.Join(srcDir, "build.gradle.kts")); txt != "" {
		if strings.Contains(txt, "com.google.cloud.tools.jib") {
			return true
		}
	}
	if txt := read(filepath.Join(srcDir, "build.gradle")); txt != "" {
		if strings.Contains(txt, "com.google.cloud.tools.jib") {
			return true
		}
	}
	// Maven: jib-maven-plugin in pom.xml
	if b, err := os.ReadFile(filepath.Join(srcDir, "pom.xml")); err == nil {
		if strings.Contains(string(b), "jib-maven-plugin") {
			return true
		}
	}
	return false
}

func read(path string) string {
	b, _ := os.ReadFile(path)
	return string(b)
}

// --- Version detectors ---

func detectFromGradleKts(path string) string {
	txt := read(path)
	if txt == "" {
		return ""
	}
	// application { } toolchain { languageVersion.set(JavaLanguageVersion.of(17)) }
	if re := regexp.MustCompile(`JavaLanguageVersion\.of\((\d+)\)`); re.MatchString(txt) {
		return re.FindStringSubmatch(txt)[1]
	}
	// sourceCompatibility = "17"
	if re := regexp.MustCompile(`sourceCompatibility\s*=\s*"(\d+)"`); re.MatchString(txt) {
		return re.FindStringSubmatch(txt)[1]
	}
	// Java toolchain DSL kotlin format: java { toolchain { languageVersion.set(JavaLanguageVersion.of(21)) } }
	if re := regexp.MustCompile(`languageVersion\.set\(JavaLanguageVersion\.of\((\d+)\)\)`); re.MatchString(txt) {
		return re.FindStringSubmatch(txt)[1]
	}
	return ""
}

func detectFromGradle(path string) string {
	txt := read(path)
	if txt == "" {
		return ""
	}
	// sourceCompatibility = JavaVersion.VERSION_17
	if re := regexp.MustCompile(`JavaVersion\.VERSION_(\d+)`); re.MatchString(txt) {
		return re.FindStringSubmatch(txt)[1]
	}
	// sourceCompatibility = '17' or "17"
	if re := regexp.MustCompile(`sourceCompatibility\s*=\s*['"](\d+)['"]`); re.MatchString(txt) {
		return re.FindStringSubmatch(txt)[1]
	}
	return ""
}

func detectFromGradleProperties(path string) string {
	txt := read(path)
	if txt == "" {
		return ""
	}
	// org.gradle.java.home or similar not used; look for javaVersion=17, jdk=17 patterns
	for _, line := range strings.Split(txt, "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "#") || line == "" {
			continue
		}
		if strings.Contains(line, "java") && strings.Contains(line, "=") {
			// naive parse: java=17 or javaVersion=21
			parts := strings.SplitN(line, "=", 2)
			if len(parts) == 2 {
				v := strings.TrimSpace(parts[1])
				if re := regexp.MustCompile(`^(\d+)$`); re.MatchString(v) {
					return v
				}
			}
		}
	}
	return ""
}

type mavenProject struct {
	XMLName    xml.Name `xml:"project"`
	Properties struct {
		JavaVersion         string `xml:"java.version"`
		MavenCompilerSource string `xml:"maven.compiler.source"`
	} `xml:"properties"`
}

func detectFromPom(path string) string {
	b, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	var p mavenProject
	if err := xml.Unmarshal(b, &p); err == nil {
		if v := strings.TrimSpace(p.Properties.JavaVersion); v != "" {
			return v
		}
		if v := strings.TrimSpace(p.Properties.MavenCompilerSource); v != "" {
			return v
		}
	}
	// fallback regex for key tags
	txt := string(b)
	if re := regexp.MustCompile(`<maven\.compiler\.source>\s*(\d+)\s*</maven\.compiler\.source>`); re.MatchString(txt) {
		return re.FindStringSubmatch(txt)[1]
	}
	if re := regexp.MustCompile(`<java\.version>\s*(\d+)\s*</java\.version>`); re.MatchString(txt) {
		return re.FindStringSubmatch(txt)[1]
	}
	return ""
}

func detectFromJavaVersionFile(path string) string {
	v := strings.TrimSpace(read(path))
	if re := regexp.MustCompile(`^(\d+)$`); re.MatchString(v) {
		return v
	}
	return ""
}

// --- Main class detectors ---

func detectMainFromGradleKts(path string) string {
	txt := read(path)
	if txt == "" {
		return ""
	}
	// application { mainClass.set("com.example.Main") }
	if re := regexp.MustCompile(`application\s*\{[^}]*mainClass\.set\(["']([^"']+)["']\)`); re.MatchString(txt) {
		return re.FindStringSubmatch(txt)[1]
	}
	// jib { container { mainClass = "com.example.Main" } }
	if re := regexp.MustCompile(`jib\s*\{[\s\S]*container\s*\{[\s\S]*mainClass\s*=\s*["']([^"']+)["']`); re.MatchString(txt) {
		return re.FindStringSubmatch(txt)[1]
	}
	return ""
}

func detectMainFromGradle(path string) string {
	txt := read(path)
	if txt == "" {
		return ""
	}
	// application { mainClassName = 'com.example.Main' } or mainClass = '...'
	if re := regexp.MustCompile(`application\s*\{[\s\S]*mainClass(Name)?\s*=\s*['"]([^'"\n]+)['"]`); re.MatchString(txt) {
		m := re.FindStringSubmatch(txt)
		return m[len(m)-1]
	}
	// jib { container { mainClass = "com.example.Main" } }
	if re := regexp.MustCompile(`jib\s*\{[\s\S]*container\s*\{[\s\S]*mainClass\s*=\s*['"]([^'"\n]+)['"]`); re.MatchString(txt) {
		return re.FindStringSubmatch(txt)[1]
	}
	return ""
}
