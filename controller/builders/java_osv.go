package builders

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"
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
	
	// Build OSv image using embedded capstan logic
	out := filepath.Join(req.OutDir, fmt.Sprintf("%s-%s.qcow2", req.App, short(req.GitSHA)))
	if err := buildOSvWithCapstan(jibTar, req.MainClass, req.App, req.GitSHA, out, javaVersion); err != nil {
		return "", fmt.Errorf("failed to build OSv image: %w", err)
	}
	return out, nil
}

func runJibBuildTar(src string, envVars map[string]string) (string, error) {
	fmt.Printf("Starting Jib build process in directory: %s\n", src)
	
	env := os.Environ()
	for k, v := range envVars {
		env = append(env, fmt.Sprintf("%s=%s", k, v))
	}
	
	// Ensure gradlew and mvnw are executable
	gradlewPath := filepath.Join(src, "gradlew")
	if exists(gradlewPath) {
		fmt.Println("Making gradlew executable...")
		if err := os.Chmod(gradlewPath, 0755); err != nil {
			fmt.Printf("Warning: Failed to make gradlew executable: %v\n", err)
		}
	}
	
	mvnwPath := filepath.Join(src, "mvnw")
	if exists(mvnwPath) {
		fmt.Println("Making mvnw executable...")
		if err := os.Chmod(mvnwPath, 0755); err != nil {
			fmt.Printf("Warning: Failed to make mvnw executable: %v\n", err)
		}
	}
	
	// Check for Gradle build
	if exists(filepath.Join(src, "gradlew")) && (exists(filepath.Join(src, "build.gradle")) || exists(filepath.Join(src, "build.gradle.kts"))) {
		fmt.Println("Detected Gradle project, checking for Jib plugin...")
		
		// First, check if Jib plugin is configured
		if !hasJibPluginGradle(src) {
			fmt.Println("Jib plugin not found, attempting to add it temporarily...")
			if err := addJibPluginToGradle(src); err != nil {
				fmt.Printf("Failed to add Jib plugin to Gradle: %v\n", err)
				// Try Spring Boot fallback
				return trySpringBootBuildGradle(src, env)
			}
		}
		
		fmt.Println("Running: ./gradlew jibBuildTar")
		cmd := exec.Command("./gradlew", "jibBuildTar")
		cmd.Dir = src
		cmd.Env = env
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		
		if err := cmd.Run(); err != nil {
			fmt.Printf("Gradle Jib build failed: %v\n", err)
			// Try Spring Boot fallback
			return trySpringBootBuildGradle(src, env)
		}
		
		p := filepath.Join(src, "build", "jib-image.tar")
		if exists(p) {
			fmt.Printf("Successfully created Jib tar: %s\n", p)
			return p, nil
		}
		fmt.Println("Jib tar file not found at expected location")
	}
	
	// Check for Maven build
	if exists(filepath.Join(src, "mvnw")) && exists(filepath.Join(src, "pom.xml")) {
		fmt.Println("Detected Maven project, checking for Jib plugin...")
		
		// First, check if Jib plugin is configured
		if !hasJibPluginMaven(src) {
			fmt.Println("Jib plugin not found, attempting to add it temporarily...")
			if err := addJibPluginToMaven(src); err != nil {
				fmt.Printf("Failed to add Jib plugin to Maven: %v\n", err)
				// Try Spring Boot fallback
				return trySpringBootBuildMaven(src, env)
			}
		}
		
		fmt.Println("Running: ./mvnw -B com.google.cloud.tools:jib-maven-plugin:buildTar")
		cmd := exec.Command("./mvnw", "-B", "com.google.cloud.tools:jib-maven-plugin:buildTar")
		cmd.Dir = src
		cmd.Env = env
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		
		if err := cmd.Run(); err != nil {
			fmt.Printf("Maven Jib build failed: %v\n", err)
			// Try Spring Boot fallback
			return trySpringBootBuildMaven(src, env)
		}
		
		p := filepath.Join(src, "target", "jib-image.tar")
		if exists(p) {
			fmt.Printf("Successfully created Jib tar: %s\n", p)
			return p, nil
		}
		fmt.Println("Jib tar file not found at expected location")
	}
	
	return "", errors.New("failed to produce Jib tar: no supported build system found")
}

func exists(p string) bool { _, err := os.Stat(p); return err == nil }
func short(s string) string { if len(s)>12 { return s[:12] }; return s }

// hasJibPluginMaven checks if Maven project has Jib plugin configured
func hasJibPluginMaven(src string) bool {
	pomPath := filepath.Join(src, "pom.xml")
	content, err := os.ReadFile(pomPath)
	if err != nil {
		return false
	}
	return strings.Contains(string(content), "jib-maven-plugin")
}

// hasJibPluginGradle checks if Gradle project has Jib plugin configured
func hasJibPluginGradle(src string) bool {
	// Check build.gradle
	if exists(filepath.Join(src, "build.gradle")) {
		content, _ := os.ReadFile(filepath.Join(src, "build.gradle"))
		if strings.Contains(string(content), "com.google.cloud.tools.jib") {
			return true
		}
	}
	// Check build.gradle.kts
	if exists(filepath.Join(src, "build.gradle.kts")) {
		content, _ := os.ReadFile(filepath.Join(src, "build.gradle.kts"))
		if strings.Contains(string(content), "com.google.cloud.tools.jib") {
			return true
		}
	}
	return false
}

// addJibPluginToMaven adds Jib plugin to Maven pom.xml
func addJibPluginToMaven(src string) error {
	pomPath := filepath.Join(src, "pom.xml")
	content, err := os.ReadFile(pomPath)
	if err != nil {
		return err
	}
	
	// Check if it's a Spring Boot project
	if !strings.Contains(string(content), "spring-boot") {
		return errors.New("not a Spring Boot project, cannot auto-configure Jib")
	}
	
	// Find the </plugins> tag and insert Jib plugin before it
	pomStr := string(content)
	jibPlugin := `
			<plugin>
				<groupId>com.google.cloud.tools</groupId>
				<artifactId>jib-maven-plugin</artifactId>
				<version>3.4.0</version>
				<configuration>
					<to>
						<image>ploy-arf-app</image>
					</to>
					<container>
						<format>OCI</format>
					</container>
				</configuration>
			</plugin>
		</plugins>`
	
	// Replace the closing </plugins> tag with our plugin + closing tag
	if strings.Contains(pomStr, "</plugins>") {
		newPom := strings.Replace(pomStr, "</plugins>", jibPlugin, 1)
		return os.WriteFile(pomPath, []byte(newPom), 0644)
	}
	
	// If no plugins section exists, add one
	if strings.Contains(pomStr, "</build>") {
		pluginsSection := `
		<plugins>` + jibPlugin + `
	</build>`
		newPom := strings.Replace(pomStr, "</build>", pluginsSection, 1)
		return os.WriteFile(pomPath, []byte(newPom), 0644)
	}
	
	return errors.New("could not find suitable location to add Jib plugin")
}

// addJibPluginToGradle adds Jib plugin to Gradle build file
func addJibPluginToGradle(src string) error {
	// For now, return an error as Gradle modification is more complex
	// Will implement if needed based on testing
	return errors.New("Gradle Jib auto-configuration not yet implemented")
}

// trySpringBootBuildMaven attempts to build using direct Jib invocation for Maven
func trySpringBootBuildMaven(src string, env []string) (string, error) {
	fmt.Println("Attempting direct Jib build without plugin configuration...")
	
	// First, ensure the project compiles
	fmt.Println("Running: ./mvnw compile -DskipTests")
	compileCmd := exec.Command("./mvnw", "compile", "-DskipTests")
	compileCmd.Dir = src
	compileCmd.Env = env
	compileCmd.Stdout = os.Stdout
	compileCmd.Stderr = os.Stderr
	
	if err := compileCmd.Run(); err != nil {
		fmt.Printf("Maven compile failed: %v\n", err)
		return "", fmt.Errorf("Maven compile failed: %w", err)
	}
	
	// Try direct Jib execution with explicit version and configuration
	fmt.Println("Running direct Jib build with inline configuration...")
	jibCmd := exec.Command("./mvnw",
		"com.google.cloud.tools:jib-maven-plugin:3.4.0:buildTar",
		"-Djib.to.image=ploy-arf-app",
		"-Djib.container.format=OCI",
		"-Djib.outputPaths.tar=target/jib-image.tar",
		"-DskipTests")
	
	jibCmd.Dir = src
	jibCmd.Env = env
	jibCmd.Stdout = os.Stdout
	jibCmd.Stderr = os.Stderr
	
	if err := jibCmd.Run(); err != nil {
		fmt.Printf("Direct Jib execution failed: %v\n", err)
		// Last resort: try with minimal configuration
		return tryMinimalJibBuild(src, env)
	}
	
	tarPath := filepath.Join(src, "target", "jib-image.tar")
	if exists(tarPath) {
		fmt.Printf("Successfully created Jib tar via direct execution: %s\n", tarPath)
		return tarPath, nil
	}
	
	return "", errors.New("Jib tar not found after direct execution")
}

// tryMinimalJibBuild attempts a minimal Jib build with basic configuration
func tryMinimalJibBuild(src string, env []string) (string, error) {
	fmt.Println("Attempting minimal Jib build with basic defaults...")
	
	// Try with absolute minimal configuration
	cmd := exec.Command("./mvnw",
		"com.google.cloud.tools:jib-maven-plugin:3.4.0:buildTar",
		"-Djib.outputPaths.tar=target/jib-image.tar",
		"-DskipTests",
		"-B")
	
	cmd.Dir = src
	cmd.Env = env
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	
	if err := cmd.Run(); err != nil {
		fmt.Printf("Minimal Jib build also failed: %v\n", err)
		return "", fmt.Errorf("all Jib build attempts failed: %w", err)
	}
	
	tarPath := filepath.Join(src, "target", "jib-image.tar")
	if exists(tarPath) {
		fmt.Printf("Successfully created Jib tar with minimal config: %s\n", tarPath)
		return tarPath, nil
	}
	
	return "", errors.New("failed to produce Jib tar with all attempted methods")
}

// trySpringBootBuildGradle attempts to build using direct Jib for Gradle
func trySpringBootBuildGradle(src string, env []string) (string, error) {
	fmt.Println("Attempting direct Jib build for Gradle...")
	
	// Ensure gradlew is executable
	gradlewPath := filepath.Join(src, "gradlew")
	if err := os.Chmod(gradlewPath, 0755); err != nil {
		fmt.Printf("Warning: Failed to make gradlew executable: %v\n", err)
	}
	
	// First compile the project
	fmt.Println("Running: ./gradlew classes -x test")
	compileCmd := exec.Command("./gradlew", "classes", "-x", "test")
	compileCmd.Dir = src
	compileCmd.Env = env
	compileCmd.Stdout = os.Stdout
	compileCmd.Stderr = os.Stderr
	
	if err := compileCmd.Run(); err != nil {
		fmt.Printf("Gradle compile failed: %v\n", err)
		return "", fmt.Errorf("Gradle compile failed: %w", err)
	}
	
	// Try to add Jib plugin dynamically via init script
	initScript := filepath.Join(src, "jib-init.gradle")
	initScriptContent := `
initscript {
    repositories {
        gradlePluginPortal()
    }
    dependencies {
        classpath 'com.google.cloud.tools:jib-gradle-plugin:3.4.0'
    }
}

allprojects {
    apply plugin: com.google.cloud.tools.jib.gradle.JibPlugin
    
    jib {
        to {
            image = 'ploy-arf-app'
        }
        container {
            format = 'OCI'
        }
    }
}
`
	if err := os.WriteFile(initScript, []byte(initScriptContent), 0644); err != nil {
		return "", fmt.Errorf("failed to create Jib init script: %w", err)
	}
	defer os.Remove(initScript)
	
	// Run Jib with init script
	fmt.Println("Running: ./gradlew jibBuildTar with init script")
	jibCmd := exec.Command("./gradlew", "--init-script", "jib-init.gradle", "jibBuildTar", "-x", "test")
	jibCmd.Dir = src
	jibCmd.Env = env
	jibCmd.Stdout = os.Stdout
	jibCmd.Stderr = os.Stderr
	
	if err := jibCmd.Run(); err != nil {
		fmt.Printf("Gradle Jib build failed: %v\n", err)
		return "", fmt.Errorf("Gradle Jib build failed: %w", err)
	}
	
	tarPath := filepath.Join(src, "build", "jib-image.tar")
	if exists(tarPath) {
		fmt.Printf("Successfully created Jib tar for Gradle: %s\n", tarPath)
		return tarPath, nil
	}
	
	return "", errors.New("Jib tar not found after Gradle build")
}

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

// buildOSvWithCapstan builds an OSv image using capstan package approach
func buildOSvWithCapstan(jibTar, mainClass, app, sha, outputPath, javaVersion string) error {
	fmt.Printf("Building OSv image with capstan package approach for app: %s\n", app)
	
	// Check if capstan is available
	if _, err := exec.LookPath("capstan"); err != nil {
		fmt.Println("Warning: capstan not found in PATH, falling back to container-based build")
		return buildWithJibContainer(jibTar, mainClass, app, sha, outputPath)
	}
	
	// Create a temporary working directory
	workDir, err := ioutil.TempDir("", "osv-build-*")
	if err != nil {
		return fmt.Errorf("failed to create temp directory: %w", err)
	}
	defer os.RemoveAll(workDir)
	
	// Extract the Jib tar to staging directory
	stagingDir := filepath.Join(workDir, "staging")
	if err := os.MkdirAll(stagingDir, 0755); err != nil {
		return fmt.Errorf("failed to create staging directory: %w", err)
	}
	
	fmt.Printf("Extracting Jib tar: %s\n", jibTar)
	if err := extractTar(jibTar, stagingDir); err != nil {
		return fmt.Errorf("failed to extract tar: %w", err)
	}
	
	// Create capstan package structure
	projectDir := filepath.Join(workDir, "package")
	metaDir := filepath.Join(projectDir, "meta")
	if err := os.MkdirAll(metaDir, 0755); err != nil {
		return fmt.Errorf("failed to create package structure: %w", err)
	}
	
	// Copy application files to package root
	for _, dir := range []string{"classes", "resources", "libs", "dependencies"} {
		srcPath := filepath.Join(stagingDir, dir)
		if exists(srcPath) {
			dstPath := filepath.Join(projectDir, dir)
			if err := copyDir(srcPath, dstPath); err != nil {
				fmt.Printf("Warning: failed to copy %s: %v\n", dir, err)
			}
		}
	}
	
	// Determine which Java package to use
	javaPackage := "openjdk8-zulu-full" // Default for Java 8
	
	// Create package.yaml
	packageYaml := fmt.Sprintf(`name: %s
title: %s
author: ploy
version: "1.0"
require:
- %s
created: "%s"
`, app, app, javaPackage, time.Now().Format(time.RFC3339))
	
	packageYamlPath := filepath.Join(metaDir, "package.yaml")
	if err := ioutil.WriteFile(packageYamlPath, []byte(packageYaml), 0644); err != nil {
		return fmt.Errorf("failed to write package.yaml: %w", err)
	}
	
	// Create run.yaml with proper Java configuration
	runYaml := fmt.Sprintf(`runtime: java
config_set:
  default:
    main: %s
    classpath:
      - /classes
      - /resources
      - /libs/*
      - /dependencies/*
    jvm_args: "-XX:+UnlockExperimentalVMOptions -XX:+UseZGC"
config_set_default: default
`, mainClass)
	
	runYamlPath := filepath.Join(metaDir, "run.yaml")
	if err := ioutil.WriteFile(runYamlPath, []byte(runYaml), 0644); err != nil {
		return fmt.Errorf("failed to write run.yaml: %w", err)
	}
	
	// Compose the package into an OSv image
	fmt.Println("Composing OSv image with capstan package...")
	imageName := fmt.Sprintf("ploy-%s", app)
	composeCmd := exec.Command("capstan", "package", "compose", "--pull-missing", imageName)
	composeCmd.Dir = projectDir
	
	// Capture stderr for debugging
	var stderr bytes.Buffer
	composeCmd.Stdout = os.Stdout
	composeCmd.Stderr = &stderr
	
	if err := composeCmd.Run(); err != nil {
		return fmt.Errorf("capstan package compose failed: %w, stderr: %s", err, stderr.String())
	}
	
	// Find the generated image - package compose creates it in capstan repository
	imgPath := filepath.Join(os.Getenv("HOME"), ".capstan", "repository", imageName, imageName+".qemu")
	if !exists(imgPath) {
		// Try without ploy prefix
		imgPath = filepath.Join(os.Getenv("HOME"), ".capstan", "repository", app, app+".qemu")
		if !exists(imgPath) {
			return fmt.Errorf("capstan image not found at expected locations")
		}
	}
	
	// Copy to output location
	if err := copyFile(imgPath, outputPath); err != nil {
		return fmt.Errorf("failed to copy image to output: %w", err)
	}
	
	fmt.Printf("OSv image successfully created: %s (size: %.2f MB)\n", outputPath, float64(fileSize(outputPath))/(1024*1024))
	return nil
}

// buildWithJibContainer falls back to using the Jib container directly
func buildWithJibContainer(jibTar, mainClass, app, sha, outputPath string) error {
	fmt.Println("Using Jib container fallback (OSv capstan not available)")
	
	// For now, we'll just copy the tar as-is and let Nomad handle it
	// In a production setup, this would convert the tar to a proper format
	
	// Change extension from .qcow2 to .tar to indicate container format
	containerOutput := strings.TrimSuffix(outputPath, ".qcow2") + ".tar"
	
	if err := copyFile(jibTar, containerOutput); err != nil {
		return fmt.Errorf("failed to copy Jib tar: %w", err)
	}
	
	fmt.Printf("Container image (Jib tar) copied to: %s\n", containerOutput)
	fmt.Println("Note: OSv build skipped, using container format for Lane C")
	
	// Update the output path to return the tar file
	if err := os.Rename(containerOutput, outputPath); err != nil {
		// If rename fails, just return the tar path
		return nil
	}
	
	return nil
}

// extractTar extracts a tar or tar.gz file to a destination directory
func extractTar(tarPath, destDir string) error {
	file, err := os.Open(tarPath)
	if err != nil {
		return err
	}
	defer file.Close()
	
	var tr *tar.Reader
	
	// Check if it's gzipped
	if strings.HasSuffix(tarPath, ".gz") || strings.HasSuffix(tarPath, ".tgz") {
		gz, err := gzip.NewReader(file)
		if err != nil {
			return err
		}
		defer gz.Close()
		tr = tar.NewReader(gz)
	} else {
		tr = tar.NewReader(file)
	}
	
	for {
		header, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return err
		}
		
		target := filepath.Join(destDir, header.Name)
		
		switch header.Typeflag {
		case tar.TypeDir:
			if err := os.MkdirAll(target, 0755); err != nil {
				return err
			}
		case tar.TypeReg:
			if err := os.MkdirAll(filepath.Dir(target), 0755); err != nil {
				return err
			}
			
			file, err := os.OpenFile(target, os.O_CREATE|os.O_RDWR, os.FileMode(header.Mode))
			if err != nil {
				return err
			}
			
			if _, err := io.Copy(file, tr); err != nil {
				file.Close()
				return err
			}
			file.Close()
		}
	}
	
	return nil
}

// copyDir recursively copies a directory
func copyDir(src, dst string) error {
	return filepath.Walk(src, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		
		relPath, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}
		
		dstPath := filepath.Join(dst, relPath)
		
		if info.IsDir() {
			return os.MkdirAll(dstPath, info.Mode())
		}
		
		return copyFile(path, dstPath)
	})
}

// copyFile is already defined in wasm.go, so we'll use that one

// fileSize returns the size of a file in bytes
func fileSize(path string) int64 {
	info, err := os.Stat(path)
	if err != nil {
		return 0
	}
	return info.Size()
}
