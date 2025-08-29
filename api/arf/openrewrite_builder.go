package arf

import (
	"context"
	"crypto/md5"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"text/template"
	"time"

	"github.com/iw2rmb/ploy/api/arf/models"
	"github.com/iw2rmb/ploy/internal/storage"
	docker "github.com/fsouza/go-dockerclient"
)

// OpenRewriteRecipe represents a known OpenRewrite recipe
type OpenRewriteRecipe struct {
	ShortName    string
	FullClass    string
	ArtifactID   string
	GroupID      string
	Version      string
	Category     string // java-migration, spring, testing, etc.
}

// OpenRewriteImageBuilder handles dynamic image building for OpenRewrite
type OpenRewriteImageBuilder struct {
	dockerClient   *docker.Client
	storageClient  *storage.StorageClient
	registryURL    string
	consulCatalog  *ConsulRecipeCatalog
	buildDir       string
}

// NewOpenRewriteImageBuilder creates a new image builder
func NewOpenRewriteImageBuilder(dockerEndpoint, registryURL, consulAddr string, storageClient *storage.StorageClient) (*OpenRewriteImageBuilder, error) {
	client, err := docker.NewClient(dockerEndpoint)
	if err != nil {
		return nil, fmt.Errorf("failed to create Docker client: %w", err)
	}

	// Create Consul catalog for recipe lookup
	consulCatalog, err := NewConsulRecipeCatalog(consulAddr, "ploy/arf")
	if err != nil {
		return nil, fmt.Errorf("failed to create Consul catalog: %w", err)
	}

	builder := &OpenRewriteImageBuilder{
		dockerClient:  client,
		storageClient: storageClient,
		registryURL:   registryURL,
		consulCatalog: consulCatalog,
		buildDir:      "/tmp/openrewrite-builds",
	}

	// Create build directory
	if err := os.MkdirAll(builder.buildDir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create build directory: %w", err)
	}

	return builder, nil
}


// BuildImageRequest represents a request to build an OpenRewrite image
type BuildImageRequest struct {
	Recipes        []string `json:"recipes"`
	PackageManager string   `json:"package_manager"` // "maven" or "gradle"
	BaseJDK        string   `json:"base_jdk"`        // "11", "17", "21"
	Force          bool     `json:"force"`           // Force rebuild even if exists
}

// BuildImageResponse represents the response from building an image
type BuildImageResponse struct {
	ImageName   string    `json:"image_name"`
	ImageTag    string    `json:"image_tag"`
	FullImage   string    `json:"full_image"`
	Recipes     []string  `json:"recipes"`
	BuildTime   time.Time `json:"build_time"`
	Cached      bool      `json:"cached"`
	Size        int64     `json:"size"`
}

// ValidateRecipes checks if all requested recipes exist by querying Consul
func (b *OpenRewriteImageBuilder) ValidateRecipes(recipes []string) ([]OpenRewriteRecipe, error) {
	validated := make([]OpenRewriteRecipe, 0, len(recipes))
	ctx := context.Background()
	
	for _, recipeName := range recipes {
		// Check if it's a full class name (custom recipe)
		if strings.Contains(recipeName, ".") {
			// Create custom recipe entry for full class names
			validated = append(validated, OpenRewriteRecipe{
				ShortName:  recipeName,
				FullClass:  recipeName,
				Category:   "custom",
				ArtifactID: "rewrite-migrate-java",
				GroupID:    "org.openrewrite.recipe",
				Version:    "2.11.0",
			})
			continue
		}
		
		// Look up recipe in Consul by ID
		recipeID := fmt.Sprintf("openrewrite-%s", recipeName)
		recipe, err := b.consulCatalog.GetRecipe(ctx, recipeID)
		if err != nil {
			// Fallback: try without prefix for backward compatibility
			recipe, err = b.consulCatalog.GetRecipe(ctx, recipeName)
			if err != nil {
				return nil, fmt.Errorf("unknown recipe: %s", recipeName)
			}
		}
		
		// Extract OpenRewrite configuration from recipe steps
		found := false
		for _, step := range recipe.Steps {
			if step.Type == models.StepTypeOpenRewrite {
				config := step.Config
				
				// Extract fields with safe type assertions
				orRecipe := OpenRewriteRecipe{
					ShortName: recipeName,
				}
				
				if fullClass, ok := config["recipe"].(string); ok {
					orRecipe.FullClass = fullClass
				}
				if artifactID, ok := config["artifactId"].(string); ok {
					orRecipe.ArtifactID = artifactID
				}
				if groupID, ok := config["groupId"].(string); ok {
					orRecipe.GroupID = groupID
				}
				if version, ok := config["version"].(string); ok {
					orRecipe.Version = version
				}
				
				// Get category from recipe metadata
				if len(recipe.Metadata.Categories) > 0 {
					orRecipe.Category = recipe.Metadata.Categories[0]
				}
				
				validated = append(validated, orRecipe)
				found = true
				break
			}
		}
		
		if !found {
			return nil, fmt.Errorf("recipe %s does not contain OpenRewrite step", recipeName)
		}
	}
	
	return validated, nil
}

// GenerateImageName creates a deterministic image name based on recipes
func (b *OpenRewriteImageBuilder) GenerateImageName(recipes []string, packageManager string) string {
	// Sort recipes for deterministic naming
	sorted := make([]string, len(recipes))
	copy(sorted, recipes)
	sort.Strings(sorted)
	
	// Create hash of recipe combination
	h := md5.New()
	io.WriteString(h, strings.Join(sorted, ","))
	io.WriteString(h, packageManager)
	hash := fmt.Sprintf("%x", h.Sum(nil))[:8]
	
	// Create readable name with hash
	prefix := "openrewrite"
	if len(recipes) == 1 {
		// Single recipe: use its name
		return fmt.Sprintf("%s-%s-%s", prefix, recipes[0], packageManager)
	} else if len(recipes) <= 3 {
		// Few recipes: concatenate names
		names := make([]string, len(recipes))
		for i, r := range recipes {
			// Shorten long names
			if len(r) > 10 {
				names[i] = r[:10]
			} else {
				names[i] = r
			}
		}
		return fmt.Sprintf("%s-%s-%s-%s", prefix, strings.Join(names, "-"), packageManager, hash)
	} else {
		// Many recipes: just use multi with hash
		// (We don't want to query Consul just for naming)
		return fmt.Sprintf("%s-multi-%s-%s", prefix, packageManager, hash)
	}
}

// CheckImageExists verifies if an image already exists in the registry
func (b *OpenRewriteImageBuilder) CheckImageExists(imageName, tag string) (bool, error) {
	fullImage := fmt.Sprintf("%s/%s:%s", b.registryURL, imageName, tag)
	
	// Try to inspect the image
	_, err := b.dockerClient.InspectImage(fullImage)
	if err != nil {
		// Docker client returns ErrNoSuchImage when image doesn't exist
		if err == docker.ErrNoSuchImage {
			return false, nil
		}
		// Also check for error string in case of registry check
		if strings.Contains(err.Error(), "No such image") {
			return false, nil
		}
		return false, err
	}
	
	return true, nil
}

// BuildImage builds a custom OpenRewrite image with specified recipes
func (b *OpenRewriteImageBuilder) BuildImage(req BuildImageRequest) (*BuildImageResponse, error) {
	// Validate recipes
	validatedRecipes, err := b.ValidateRecipes(req.Recipes)
	if err != nil {
		return nil, fmt.Errorf("recipe validation failed: %w", err)
	}
	
	// Set defaults
	if req.PackageManager == "" {
		req.PackageManager = "maven"
	}
	if req.BaseJDK == "" {
		req.BaseJDK = "17"
	}
	
	// Generate image name
	imageName := b.GenerateImageName(req.Recipes, req.PackageManager)
	imageTag := "latest"
	
	// Check if image already exists (unless force rebuild)
	if !req.Force {
		exists, err := b.CheckImageExists(imageName, imageTag)
		if err != nil {
			return nil, fmt.Errorf("failed to check image existence: %w", err)
		}
		if exists {
			return &BuildImageResponse{
				ImageName: imageName,
				ImageTag:  imageTag,
				FullImage: fmt.Sprintf("%s/%s:%s", b.registryURL, imageName, imageTag),
				Recipes:   req.Recipes,
				BuildTime: time.Now(),
				Cached:    true,
			}, nil
		}
	}
	
	// Generate Dockerfile
	dockerfile, err := b.generateDockerfile(validatedRecipes, req.PackageManager, req.BaseJDK)
	if err != nil {
		return nil, fmt.Errorf("failed to generate Dockerfile: %w", err)
	}
	
	// Create build context
	buildPath := filepath.Join(b.buildDir, imageName)
	if err := os.MkdirAll(buildPath, 0755); err != nil {
		return nil, fmt.Errorf("failed to create build directory: %w", err)
	}
	
	dockerfilePath := filepath.Join(buildPath, "Dockerfile")
	if err := os.WriteFile(dockerfilePath, []byte(dockerfile), 0644); err != nil {
		return nil, fmt.Errorf("failed to write Dockerfile: %w", err)
	}
	
	// Build the image
	buildOptions := docker.BuildImageOptions{
		Name:         fmt.Sprintf("%s/%s:%s", b.registryURL, imageName, imageTag),
		Dockerfile:   "Dockerfile",
		ContextDir:   buildPath,
		OutputStream: os.Stdout,
		RmTmpContainer: true,
		ForceRmTmpContainer: true,
	}
	
	if err := b.dockerClient.BuildImage(buildOptions); err != nil {
		return nil, fmt.Errorf("failed to build image: %w", err)
	}
	
	// Push to registry
	pushOptions := docker.PushImageOptions{
		Name:         fmt.Sprintf("%s/%s", b.registryURL, imageName),
		Tag:          imageTag,
		OutputStream: os.Stdout,
	}
	
	if err := b.dockerClient.PushImage(pushOptions, docker.AuthConfiguration{}); err != nil {
		return nil, fmt.Errorf("failed to push image: %w", err)
	}
	
	// Get image size
	imageInfo, err := b.dockerClient.InspectImage(fmt.Sprintf("%s/%s:%s", b.registryURL, imageName, imageTag))
	var imageSize int64
	if err == nil {
		imageSize = imageInfo.Size
	}
	
	return &BuildImageResponse{
		ImageName: imageName,
		ImageTag:  imageTag,
		FullImage: fmt.Sprintf("%s/%s:%s", b.registryURL, imageName, imageTag),
		Recipes:   req.Recipes,
		BuildTime: time.Now(),
		Cached:    false,
		Size:      imageSize,
	}, nil
}

// generateDockerfile creates a Dockerfile for the specified recipes
func (b *OpenRewriteImageBuilder) generateDockerfile(recipes []OpenRewriteRecipe, packageManager, baseJDK string) (string, error) {
	tmplStr := `# Auto-generated OpenRewrite image with custom recipes
# Generated: {{ .Timestamp }}
# Recipes: {{ .RecipeList }}

FROM eclipse-temurin:{{ .BaseJDK }}-jdk-alpine

{{ if eq .PackageManager "maven" }}
# Install Maven
RUN apk add --no-cache bash tar curl && \
    mkdir -p /usr/share/maven && \
    curl -fsSL https://archive.apache.org/dist/maven/maven-3/3.9.6/binaries/apache-maven-3.9.6-bin.tar.gz | \
    tar -xzC /usr/share/maven --strip-components=1

ENV PATH="/usr/share/maven/bin:${PATH}"
ENV MAVEN_OPTS="-XX:+TieredCompilation -XX:TieredStopAtLevel=1"
{{ else }}
# Install Gradle
ENV GRADLE_VERSION=8.5
RUN apk add --no-cache bash tar curl unzip && \
    mkdir -p /opt/gradle && \
    curl -fsSL https://services.gradle.org/distributions/gradle-${GRADLE_VERSION}-bin.zip -o gradle.zip && \
    unzip -d /opt/gradle gradle.zip && \
    rm gradle.zip && \
    ln -s /opt/gradle/gradle-${GRADLE_VERSION}/bin/gradle /usr/bin/gradle
{{ end }}

WORKDIR /workspace

# Pre-download recipe dependencies
{{ range .Recipes }}
RUN {{ if eq $.PackageManager "maven" }}mvn dependency:get \
    -DgroupId={{ .GroupID }} \
    -DartifactId={{ .ArtifactID }} \
    -Dversion={{ .Version }} \
    -Dtransitive=true{{ else }}echo "Gradle recipe: {{ .FullClass }}"{{ end }}
{{ end }}

# Create execution script
RUN cat > /usr/local/bin/run-openrewrite << 'EOF'
#!/bin/bash
set -e

INPUT_TAR=$1
OUTPUT_TAR=$2
RECIPE=$3

echo "OpenRewrite Custom Image - Recipe: $RECIPE"

# Extract input
rm -rf /workspace/input
mkdir -p /workspace/input
tar -xf "$INPUT_TAR" -C /workspace/input

cd /workspace/input

{{ if eq .PackageManager "maven" }}
# Maven execution
if [ ! -f pom.xml ]; then
    cat > pom.xml << POM
<?xml version="1.0" encoding="UTF-8"?>
<project xmlns="http://maven.apache.org/POM/4.0.0">
    <modelVersion>4.0.0</modelVersion>
    <groupId>transform</groupId>
    <artifactId>project</artifactId>
    <version>1.0</version>
    <properties>
        <maven.compiler.source>{{ .BaseJDK }}</maven.compiler.source>
        <maven.compiler.target>{{ .BaseJDK }}</maven.compiler.target>
    </properties>
</project>
POM
fi

# Recipe mapping
case "$RECIPE" in
{{ range .Recipes }}
    "{{ .ShortName }}")
        RECIPE_FULL="{{ .FullClass }}"
        RECIPE_ARTIFACT="{{ .GroupID }}:{{ .ArtifactID }}:{{ .Version }}"
        ;;
{{ end }}
    *)
        RECIPE_FULL="$RECIPE"
        RECIPE_ARTIFACT="org.openrewrite.recipe:rewrite-migrate-java:2.11.0"
        ;;
esac

mvn -B org.openrewrite.maven:rewrite-maven-plugin:5.34.0:run \
    -Drewrite.recipeArtifactCoordinates="$RECIPE_ARTIFACT" \
    -Drewrite.activeRecipes="$RECIPE_FULL" \
    -Drewrite.exportDatatables=false \
    2>&1 | tee /tmp/openrewrite.log

{{ else }}
# Gradle execution
if [ ! -f build.gradle ] && [ ! -f build.gradle.kts ]; then
    cat > build.gradle << GRADLE
plugins {
    id 'java'
    id 'org.openrewrite.rewrite' version '6.8.0'
}

repositories {
    mavenCentral()
}

sourceCompatibility = '{{ .BaseJDK }}'

dependencies {
{{ range .Recipes }}
    rewrite("{{ .GroupID }}:{{ .ArtifactID }}:{{ .Version }}")
{{ end }}
}
GRADLE
fi

# Add rewrite configuration
cat >> build.gradle << GRADLE

rewrite {
    activeRecipe("$RECIPE")
}
GRADLE

gradle --no-daemon rewriteRun 2>&1 | tee /tmp/openrewrite.log
{{ end }}

# Check success
if grep -q "BUILD SUCCE" /tmp/openrewrite.log; then
    SUCCESS=true
else
    SUCCESS=false
fi

# Create output tar
tar -cf "$OUTPUT_TAR" -C /workspace/input .

# Create result JSON
echo "{\"success\":$SUCCESS,\"recipe\":\"$RECIPE\",\"manager\":\"{{ .PackageManager }}\"}" > "${OUTPUT_TAR%.tar}.json"

echo "Transformation complete"
EOF

RUN chmod +x /usr/local/bin/run-openrewrite

ENTRYPOINT ["/usr/local/bin/run-openrewrite"]
`

	// Create template data
	data := struct {
		Timestamp      string
		RecipeList     string
		BaseJDK        string
		PackageManager string
		Recipes        []OpenRewriteRecipe
	}{
		Timestamp:      time.Now().Format(time.RFC3339),
		RecipeList:     strings.Join(getRecipeNames(recipes), ", "),
		BaseJDK:        baseJDK,
		PackageManager: packageManager,
		Recipes:        recipes,
	}
	
	// Execute template
	tmpl, err := template.New("dockerfile").Parse(tmplStr)
	if err != nil {
		return "", fmt.Errorf("failed to parse template: %w", err)
	}
	
	var buf strings.Builder
	if err := tmpl.Execute(&buf, data); err != nil {
		return "", fmt.Errorf("failed to execute template: %w", err)
	}
	
	return buf.String(), nil
}

// getRecipeNames extracts recipe names from recipe list
func getRecipeNames(recipes []OpenRewriteRecipe) []string {
	names := make([]string, len(recipes))
	for i, r := range recipes {
		names[i] = r.ShortName
	}
	return names
}