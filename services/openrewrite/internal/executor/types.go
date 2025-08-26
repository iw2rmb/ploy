package openrewrite

import (
	"context"
	"os"
	"time"
)

// RecipeConfig defines the OpenRewrite recipe configuration
type RecipeConfig struct {
	// Recipe is the fully qualified recipe name (e.g., "org.openrewrite.java.migrate.UpgradeToJava17")
	Recipe string `json:"recipe"`
	
	// Artifacts are the Maven coordinates for recipe artifacts (e.g., "org.openrewrite.recipe:rewrite-migrate-java:3.15.0")
	Artifacts string `json:"artifacts"`
	
	// Options are additional recipe-specific options
	Options map[string]string `json:"options,omitempty"`
}

// TransformRequest represents a transformation request
type TransformRequest struct {
	// JobID is a unique identifier for this transformation job
	JobID string `json:"job_id"`
	
	// TarArchive is the base64-encoded tar.gz archive of the source code
	TarArchive string `json:"tar_archive"`
	
	// RecipeConfig contains the OpenRewrite recipe configuration
	RecipeConfig RecipeConfig `json:"recipe_config"`
	
	// Timeout is the maximum duration for the transformation
	Timeout time.Duration `json:"timeout,omitempty"`
}

// TransformResult represents the result of a transformation
type TransformResult struct {
	// Success indicates whether the transformation completed successfully
	Success bool `json:"success"`
	
	// Diff is the unified diff showing the changes made
	Diff []byte `json:"diff,omitempty"`
	
	// Error contains any error message if the transformation failed
	Error string `json:"error,omitempty"`
	
	// Duration is how long the transformation took
	Duration time.Duration `json:"duration"`
	
	// BuildSystem indicates which build system was detected (maven/gradle)
	BuildSystem string `json:"build_system,omitempty"`
	
	// JavaVersion indicates which Java version was detected
	JavaVersion string `json:"java_version,omitempty"`
}

// BuildSystem represents the detected build system
type BuildSystem string

const (
	BuildSystemMaven  BuildSystem = "maven"
	BuildSystemGradle BuildSystem = "gradle"
	BuildSystemNone   BuildSystem = "none"
)

// JavaVersion represents supported Java versions
type JavaVersion string

const (
	Java11 JavaVersion = "11"
	Java17 JavaVersion = "17"
	Java21 JavaVersion = "21"
)

// Executor defines the interface for OpenRewrite execution
type Executor interface {
	// Execute runs an OpenRewrite transformation on the provided source code
	Execute(ctx context.Context, jobID string, tarData []byte, recipe RecipeConfig) (*TransformResult, error)
	
	// DetectBuildSystem identifies the build system used in the source directory
	DetectBuildSystem(srcPath string) BuildSystem
	
	// DetectJavaVersion identifies the Java version from the source directory
	DetectJavaVersion(srcPath string) (JavaVersion, error)
}

// GitManager defines the interface for Git operations
type GitManager interface {
	// InitializeRepo creates a Git repository from a tar archive
	InitializeRepo(ctx context.Context, jobID string, tarData []byte) (string, error)
	
	// GenerateDiff creates a unified diff after transformation
	GenerateDiff(ctx context.Context, repoPath string) ([]byte, error)
	
	// Cleanup removes the temporary repository directory
	Cleanup(repoPath string) error
}

// DefaultRecipes provides commonly used OpenRewrite recipes
var DefaultRecipes = map[string]RecipeConfig{
	"java11to17": {
		Recipe:    "org.openrewrite.java.migrate.UpgradeToJava17",
		Artifacts: "org.openrewrite.recipe:rewrite-migrate-java:3.15.0",
	},
	"java17to21": {
		Recipe:    "org.openrewrite.java.migrate.UpgradeToJava21",
		Artifacts: "org.openrewrite.recipe:rewrite-migrate-java:3.15.0",
	},
	"spring-boot-3": {
		Recipe:    "org.openrewrite.java.spring.boot3.UpgradeSpringBoot_3_0",
		Artifacts: "org.openrewrite.recipe:rewrite-spring:5.21.0",
	},
}

// Config holds the configuration for the OpenRewrite service
type Config struct {
	// WorkDir is the directory where temporary repositories are created
	WorkDir string
	
	// MavenPath is the path to the Maven executable
	MavenPath string
	
	// GradlePath is the path to the Gradle executable
	GradlePath string
	
	// JavaHome is the JAVA_HOME environment variable
	JavaHome string
	
	// GitPath is the path to the Git executable
	GitPath string
	
	// MaxTransformTime is the maximum time allowed for a transformation
	MaxTransformTime time.Duration
	
	// PreCachedArtifacts is a list of Maven coordinates to pre-cache
	PreCachedArtifacts []string
}

// DefaultConfig returns a default configuration
func DefaultConfig() *Config {
	return &Config{
		WorkDir:          "/tmp/openrewrite",
		MavenPath:        "mvn",
		GradlePath:       "gradle",
		JavaHome:         os.Getenv("JAVA_HOME"),
		GitPath:          "git",
		MaxTransformTime: 5 * time.Minute,
		PreCachedArtifacts: []string{
			"org.openrewrite.recipe:rewrite-migrate-java:3.15.0",
			"org.openrewrite.recipe:rewrite-spring:5.21.0",
		},
	}
}