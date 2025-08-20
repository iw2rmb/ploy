package builders

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
)

type JavaOSVRequest struct {
	App         string
	MainClass   string
	SrcDir      string            // source directory
	JibTar      string            // optional
	GitSHA      string
	OutDir      string
	EnvVars     map[string]string // environment variables
	JavaVersion string            // detected Java version (e.g., "21", "17", "11")
}

func BuildOSVJava(req JavaOSVRequest) (string, error) {
	if req.SrcDir == "" && req.JibTar == "" {
		return "", errors.New("either SrcDir or JibTar must be provided")
	}
	
	// Detect Java version if not provided
	javaVersion := req.JavaVersion
	if javaVersion == "" && req.SrcDir != "" {
		if detected, err := detectJavaVersion(req.SrcDir); err == nil && detected != "" {
			javaVersion = detected
			fmt.Printf("Detected Java version: %s\n", javaVersion)
		} else {
			javaVersion = "21" // Default to Java 21
			fmt.Printf("Java version detection failed, using default: %s\n", javaVersion)
		}
	} else if javaVersion == "" {
		javaVersion = "21" // Default fallback
	}
	
	jibTar := req.JibTar
	if jibTar == "" {
		var err error
		jibTar, err = runJibBuildTar(req.SrcDir, req.EnvVars)
		if err != nil { return "", err }
	}
	out := filepath.Join(req.OutDir, fmt.Sprintf("%s-%s.qcow2", req.App, short(req.GitSHA)))
	args := []string{ "--tar", jibTar, "--main", req.MainClass, "--app", req.App, "--sha", req.GitSHA, "--out", out, "--java-version", javaVersion }
	cmd := exec.Command("./scripts/build/osv/java/build_osv_java_with_capstan.sh", args...)
	cmd.Stdout = os.Stdout; cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil { return "", err }
	return out, nil
}

func runJibBuildTar(src string, envVars map[string]string) (string, error) {
	env := os.Environ()
	for k, v := range envVars {
		env = append(env, fmt.Sprintf("%s=%s", k, v))
	}
	
	if exists(filepath.Join(src,"gradlew")) && (exists(filepath.Join(src,"build.gradle")) || exists(filepath.Join(src,"build.gradle.kts"))) {
		cmd := exec.Command("./gradlew","jibBuildTar"); cmd.Dir = src; cmd.Env = env; cmd.Stdout=os.Stdout; cmd.Stderr=os.Stderr; if err:=cmd.Run(); err==nil { p:=filepath.Join(src,"build","jib-image.tar"); if exists(p){return p,nil} }
	}
	if exists(filepath.Join(src,"mvnw")) && exists(filepath.Join(src,"pom.xml")) {
		cmd := exec.Command("./mvnw","-B","com.google.cloud.tools:jib-maven-plugin:buildTar"); cmd.Dir = src; cmd.Env = env; cmd.Stdout=os.Stdout; cmd.Stderr=os.Stderr; if err:=cmd.Run(); err==nil { p:=filepath.Join(src,"target","jib-image.tar"); if exists(p){return p,nil} }
	}
	return "", errors.New("failed to produce Jib tar")
}

func exists(p string) bool { _, err := os.Stat(p); return err == nil }
func short(s string) string { if len(s)>12 { return s[:12] }; return s }

// detectJavaVersion detects Java version from Gradle, Maven, or other build files
func detectJavaVersion(srcDir string) (string, error) {
	// Check Gradle files first (build.gradle, build.gradle.kts)
	if javaVersion := detectJavaVersionFromGradle(srcDir); javaVersion != "" {
		return javaVersion, nil
	}
	
	// Check Maven files (pom.xml)
	if javaVersion := detectJavaVersionFromMaven(srcDir); javaVersion != "" {
		return javaVersion, nil
	}
	
	// Check .java-version file
	if javaVersion := detectJavaVersionFromFile(srcDir); javaVersion != "" {
		return javaVersion, nil
	}
	
	return "", errors.New("Java version not detected")
}

// detectJavaVersionFromGradle detects Java version from Gradle build files
func detectJavaVersionFromGradle(srcDir string) string {
	// Check build.gradle.kts first
	if exists(filepath.Join(srcDir, "build.gradle.kts")) {
		if version := parseJavaVersionFromGradleKts(filepath.Join(srcDir, "build.gradle.kts")); version != "" {
			return version
		}
	}
	
	// Check build.gradle
	if exists(filepath.Join(srcDir, "build.gradle")) {
		if version := parseJavaVersionFromGradle(filepath.Join(srcDir, "build.gradle")); version != "" {
			return version
		}
	}
	
	// Check gradle.properties
	if exists(filepath.Join(srcDir, "gradle.properties")) {
		if version := parseJavaVersionFromGradleProperties(filepath.Join(srcDir, "gradle.properties")); version != "" {
			return version
		}
	}
	
	return ""
}

// parseJavaVersionFromGradleKts parses Java version from build.gradle.kts
func parseJavaVersionFromGradleKts(filePath string) string {
	content, err := os.ReadFile(filePath)
	if err != nil {
		return ""
	}
	
	text := string(content)
	
	// Pattern 1: java { toolchain { languageVersion.set(JavaLanguageVersion.of(21)) } }
	if re := regexp.MustCompile(`JavaLanguageVersion\.of\((\d+)\)`); re.MatchString(text) {
		matches := re.FindStringSubmatch(text)
		if len(matches) > 1 {
			return matches[1]
		}
	}
	
	// Pattern 2: java { toolchain { languageVersion = JavaLanguageVersion.of(17) } }
	if re := regexp.MustCompile(`languageVersion\s*=\s*JavaLanguageVersion\.of\((\d+)\)`); re.MatchString(text) {
		matches := re.FindStringSubmatch(text)
		if len(matches) > 1 {
			return matches[1]
		}
	}
	
	// Pattern 3: sourceCompatibility = "11" or sourceCompatibility = JavaVersion.VERSION_11
	if re := regexp.MustCompile(`sourceCompatibility\s*=\s*"(\d+)"`); re.MatchString(text) {
		matches := re.FindStringSubmatch(text)
		if len(matches) > 1 {
			return matches[1]
		}
	}
	
	if re := regexp.MustCompile(`sourceCompatibility\s*=\s*JavaVersion\.VERSION_(\d+)`); re.MatchString(text) {
		matches := re.FindStringSubmatch(text)
		if len(matches) > 1 {
			return matches[1]
		}
	}
	
	// Pattern 4: targetCompatibility = "17"
	if re := regexp.MustCompile(`targetCompatibility\s*=\s*"(\d+)"`); re.MatchString(text) {
		matches := re.FindStringSubmatch(text)
		if len(matches) > 1 {
			return matches[1]
		}
	}
	
	return ""
}

// parseJavaVersionFromGradle parses Java version from build.gradle
func parseJavaVersionFromGradle(filePath string) string {
	content, err := os.ReadFile(filePath)
	if err != nil {
		return ""
	}
	
	text := string(content)
	
	// Pattern 1: sourceCompatibility = '11' or sourceCompatibility = "17"
	if re := regexp.MustCompile(`sourceCompatibility\s*=\s*['"](\d+)['"]`); re.MatchString(text) {
		matches := re.FindStringSubmatch(text)
		if len(matches) > 1 {
			return matches[1]
		}
	}
	
	// Pattern 2: targetCompatibility = 21
	if re := regexp.MustCompile(`targetCompatibility\s*=\s*(\d+)`); re.MatchString(text) {
		matches := re.FindStringSubmatch(text)
		if len(matches) > 1 {
			return matches[1]
		}
	}
	
	// Pattern 3: JavaVersion.VERSION_17
	if re := regexp.MustCompile(`JavaVersion\.VERSION_(\d+)`); re.MatchString(text) {
		matches := re.FindStringSubmatch(text)
		if len(matches) > 1 {
			return matches[1]
		}
	}
	
	return ""
}

// parseJavaVersionFromGradleProperties parses Java version from gradle.properties
func parseJavaVersionFromGradleProperties(filePath string) string {
	content, err := os.ReadFile(filePath)
	if err != nil {
		return ""
	}
	
	lines := strings.Split(string(content), "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "java.version") || strings.HasPrefix(line, "javaVersion") {
			if parts := strings.Split(line, "="); len(parts) == 2 {
				version := strings.TrimSpace(parts[1])
				// Extract major version number
				if re := regexp.MustCompile(`(\d+)`); re.MatchString(version) {
					matches := re.FindStringSubmatch(version)
					if len(matches) > 1 {
						return matches[1]
					}
				}
			}
		}
	}
	
	return ""
}

// detectJavaVersionFromMaven detects Java version from Maven pom.xml
func detectJavaVersionFromMaven(srcDir string) string {
	pomPath := filepath.Join(srcDir, "pom.xml")
	if !exists(pomPath) {
		return ""
	}
	
	content, err := os.ReadFile(pomPath)
	if err != nil {
		return ""
	}
	
	text := string(content)
	
	// Pattern 1: <maven.compiler.source>17</maven.compiler.source>
	if re := regexp.MustCompile(`<maven\.compiler\.source>(\d+)</maven\.compiler\.source>`); re.MatchString(text) {
		matches := re.FindStringSubmatch(text)
		if len(matches) > 1 {
			return matches[1]
		}
	}
	
	// Pattern 2: <maven.compiler.target>21</maven.compiler.target>
	if re := regexp.MustCompile(`<maven\.compiler\.target>(\d+)</maven\.compiler\.target>`); re.MatchString(text) {
		matches := re.FindStringSubmatch(text)
		if len(matches) > 1 {
			return matches[1]
		}
	}
	
	// Pattern 3: <java.version>11</java.version>
	if re := regexp.MustCompile(`<java\.version>(\d+)</java\.version>`); re.MatchString(text) {
		matches := re.FindStringSubmatch(text)
		if len(matches) > 1 {
			return matches[1]
		}
	}
	
	// Pattern 4: <source>17</source> in compiler plugin configuration
	if re := regexp.MustCompile(`<source>(\d+)</source>`); re.MatchString(text) {
		matches := re.FindStringSubmatch(text)
		if len(matches) > 1 {
			return matches[1]
		}
	}
	
	// Pattern 5: <target>21</target> in compiler plugin configuration
	if re := regexp.MustCompile(`<target>(\d+)</target>`); re.MatchString(text) {
		matches := re.FindStringSubmatch(text)
		if len(matches) > 1 {
			return matches[1]
		}
	}
	
	return ""
}

// detectJavaVersionFromFile detects Java version from .java-version file
func detectJavaVersionFromFile(srcDir string) string {
	javaVersionFile := filepath.Join(srcDir, ".java-version")
	if !exists(javaVersionFile) {
		return ""
	}
	
	content, err := os.ReadFile(javaVersionFile)
	if err != nil {
		return ""
	}
	
	version := strings.TrimSpace(string(content))
	// Extract major version number
	if re := regexp.MustCompile(`(\d+)`); re.MatchString(version) {
		matches := re.FindStringSubmatch(version)
		if len(matches) > 1 {
			// Validate it's a reasonable Java version
			if majorVersion, err := strconv.Atoi(matches[1]); err == nil {
				if majorVersion >= 8 && majorVersion <= 25 { // reasonable range
					return matches[1]
				}
			}
		}
	}
	
	return ""
}
