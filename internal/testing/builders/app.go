package builders

import (
	"fmt"
	"time"

	"github.com/google/uuid"
)

// App represents a test application with all common fields
// This consolidates TestApp from testutil and MockApplication from testutils
type App struct {
	ID           string
	Name         string
	Language     string
	Lane         string
	Version      string
	Status       string
	Instances    int
	EnvVars      map[string]string
	GitURL       string
	Branch       string
	BuildTime    time.Duration
	BuildCommand string
	CreatedAt    time.Time
	UpdatedAt    time.Time
}

// AppBuilder provides a fluent interface for creating test applications
// This consolidates AppTestBuilder and ApplicationBuilder from duplicate packages
type AppBuilder struct {
	app App
}

// NewApp creates a new app builder with sensible defaults
func NewApp() *AppBuilder {
	now := time.Now()
	return &AppBuilder{
		app: App{
			ID:        uuid.New().String(),
			Name:      "test-app",
			Language:  "go",
			Lane:      "A",
			Version:   "1.0.0",
			Status:    "created",
			Instances: 1,
			EnvVars:   make(map[string]string),
			GitURL:    "https://github.com/test/app.git",
			Branch:    "main",
			BuildTime: 2 * time.Minute,
			CreatedAt: now,
			UpdatedAt: now,
		},
	}
}

// WithID sets the application ID
func (b *AppBuilder) WithID(id string) *AppBuilder {
	b.app.ID = id
	return b
}

// WithName sets the application name
func (b *AppBuilder) WithName(name string) *AppBuilder {
	b.app.Name = name
	return b
}

// WithLanguage sets the programming language
func (b *AppBuilder) WithLanguage(language string) *AppBuilder {
	b.app.Language = language
	return b
}

// InLane sets the deployment lane (A-F)
func (b *AppBuilder) InLane(lane string) *AppBuilder {
	b.app.Lane = lane
	return b
}

// WithVersion sets the application version
func (b *AppBuilder) WithVersion(version string) *AppBuilder {
	b.app.Version = version
	return b
}

// WithStatus sets the application status
func (b *AppBuilder) WithStatus(status string) *AppBuilder {
	b.app.Status = status
	return b
}

// WithInstances sets the number of instances
func (b *AppBuilder) WithInstances(instances int) *AppBuilder {
	b.app.Instances = instances
	return b
}

// WithEnvVar adds a single environment variable
func (b *AppBuilder) WithEnvVar(key, value string) *AppBuilder {
	if b.app.EnvVars == nil {
		b.app.EnvVars = make(map[string]string)
	}
	b.app.EnvVars[key] = value
	return b
}

// WithEnvVars sets multiple environment variables at once
func (b *AppBuilder) WithEnvVars(envVars map[string]string) *AppBuilder {
	if envVars == nil {
		b.app.EnvVars = make(map[string]string)
		return b
	}

	if b.app.EnvVars == nil {
		b.app.EnvVars = make(map[string]string)
	}

	for k, v := range envVars {
		b.app.EnvVars[k] = v
	}
	return b
}

// WithGitRepo sets the Git repository URL and branch
func (b *AppBuilder) WithGitRepo(url, branch string) *AppBuilder {
	b.app.GitURL = url
	b.app.Branch = branch
	return b
}

// WithBuildTime sets the build duration
func (b *AppBuilder) WithBuildTime(duration time.Duration) *AppBuilder {
	b.app.BuildTime = duration
	return b
}

// WithBuildCommand sets the build command
func (b *AppBuilder) WithBuildCommand(command string) *AppBuilder {
	b.app.BuildCommand = command
	return b
}

// WithCreatedAt sets the creation timestamp
func (b *AppBuilder) WithCreatedAt(createdAt time.Time) *AppBuilder {
	b.app.CreatedAt = createdAt
	return b
}

// WithUpdatedAt sets the update timestamp
func (b *AppBuilder) WithUpdatedAt(updatedAt time.Time) *AppBuilder {
	b.app.UpdatedAt = updatedAt
	return b
}

// WithTimestamps sets both created and updated timestamps
func (b *AppBuilder) WithTimestamps(createdAt, updatedAt time.Time) *AppBuilder {
	b.app.CreatedAt = createdAt
	b.app.UpdatedAt = updatedAt
	return b
}

// Build creates the final App instance
func (b *AppBuilder) Build() *App {
	// Create a copy to avoid mutation issues
	app := b.app

	// Ensure EnvVars is not nil
	if app.EnvVars == nil {
		app.EnvVars = make(map[string]string)
	}

	return &app
}

// Common app presets for different deployment lanes

// GoServiceApp creates a typical Go service application
func GoServiceApp(name string) *AppBuilder {
	return NewApp().
		WithName(name).
		WithLanguage("go").
		InLane("A").
		WithEnvVar("GO_ENV", "production").
		WithEnvVar("PORT", "8080").
		WithBuildCommand("go build -o app")
}

// NodeWebApp creates a typical Node.js web application
func NodeWebApp(name string) *AppBuilder {
	return NewApp().
		WithName(name).
		WithLanguage("node").
		InLane("B").
		WithEnvVar("NODE_ENV", "production").
		WithEnvVar("PORT", "3000").
		WithBuildCommand("npm run build")
}

// JavaAPIApp creates a typical Java API application
func JavaAPIApp(name string) *AppBuilder {
	return NewApp().
		WithName(name).
		WithLanguage("java").
		InLane("C").
		WithEnvVar("JAVA_OPTS", "-Xmx1g -Xms512m").
		WithEnvVar("SERVER_PORT", "8080").
		WithBuildCommand("mvn clean package")
}

// PythonMLApp creates a typical Python ML application
func PythonMLApp(name string) *AppBuilder {
	return NewApp().
		WithName(name).
		WithLanguage("python").
		InLane("D").
		WithEnvVar("PYTHON_ENV", "production").
		WithEnvVar("WORKERS", "4").
		WithBuildCommand("pip install -r requirements.txt")
}

// StaticSiteApp creates a typical static site application
func StaticSiteApp(name string) *AppBuilder {
	return NewApp().
		WithName(name).
		WithLanguage("static").
		InLane("E").
		WithBuildCommand("npm run build:static")
}

// DockerApp creates a typical Docker-based application
func DockerApp(name string) *AppBuilder {
	return NewApp().
		WithName(name).
		WithLanguage("docker").
		InLane("F").
		WithBuildCommand(fmt.Sprintf("docker build -t %s .", name))
}
