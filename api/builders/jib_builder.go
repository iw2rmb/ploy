package builders

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

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
	return errors.New("gradle jib auto-configuration not yet implemented")
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
		return "", fmt.Errorf("maven compile failed: %w", err)
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

	return "", errors.New("jib tar not found after direct execution")
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
		return "", fmt.Errorf("gradle compile failed: %w", err)
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
	defer func() { _ = os.Remove(initScript) }()

	// Run Jib with init script
	fmt.Println("Running: ./gradlew jibBuildTar with init script")
	jibCmd := exec.Command("./gradlew", "--init-script", "jib-init.gradle", "jibBuildTar", "-x", "test")
	jibCmd.Dir = src
	jibCmd.Env = env
	jibCmd.Stdout = os.Stdout
	jibCmd.Stderr = os.Stderr

	if err := jibCmd.Run(); err != nil {
		fmt.Printf("Gradle Jib build failed: %v\n", err)
		return "", fmt.Errorf("gradle jib build failed: %w", err)
	}

	tarPath := filepath.Join(src, "build", "jib-image.tar")
	if exists(tarPath) {
		fmt.Printf("Successfully created Jib tar for Gradle: %s\n", tarPath)
		return tarPath, nil
	}

	return "", errors.New("jib tar not found after Gradle build")
}
