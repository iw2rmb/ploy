package builders_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/iw2rmb/ploy/internal/testing/builders"
)

func TestAppBuilder(t *testing.T) {
	t.Run("default values", func(t *testing.T) {
		app := builders.NewApp().Build()
		
		// Should have sensible defaults
		assert.NotEmpty(t, app.ID)
		assert.NotEmpty(t, app.Name)
		assert.NotEmpty(t, app.Language)
		assert.NotEmpty(t, app.Lane)
		assert.NotEmpty(t, app.Status)
		assert.NotZero(t, app.CreatedAt)
		assert.NotZero(t, app.UpdatedAt)
		assert.NotNil(t, app.EnvVars)
	})

	t.Run("with custom name", func(t *testing.T) {
		app := builders.NewApp().
			WithName("custom-app").
			Build()
		
		assert.Equal(t, "custom-app", app.Name)
	})

	t.Run("with language and version", func(t *testing.T) {
		app := builders.NewApp().
			WithLanguage("python").
			WithVersion("3.9.0").
			Build()
		
		assert.Equal(t, "python", app.Language)
		assert.Equal(t, "3.9.0", app.Version)
	})

	t.Run("with deployment lane", func(t *testing.T) {
		app := builders.NewApp().
			InLane("C").
			Build()
		
		assert.Equal(t, "C", app.Lane)
	})

	t.Run("with status and instances", func(t *testing.T) {
		app := builders.NewApp().
			WithStatus("running").
			WithInstances(3).
			Build()
		
		assert.Equal(t, "running", app.Status)
		assert.Equal(t, 3, app.Instances)
	})

	t.Run("with environment variables", func(t *testing.T) {
		app := builders.NewApp().
			WithEnvVar("PORT", "8080").
			WithEnvVar("LOG_LEVEL", "debug").
			Build()
		
		assert.Equal(t, "8080", app.EnvVars["PORT"])
		assert.Equal(t, "debug", app.EnvVars["LOG_LEVEL"])
	})

	t.Run("with bulk environment variables", func(t *testing.T) {
		envVars := map[string]string{
			"DATABASE_URL": "postgres://localhost/test",
			"REDIS_URL":    "redis://localhost:6379",
		}
		
		app := builders.NewApp().
			WithEnvVars(envVars).
			Build()
		
		assert.Equal(t, envVars["DATABASE_URL"], app.EnvVars["DATABASE_URL"])
		assert.Equal(t, envVars["REDIS_URL"], app.EnvVars["REDIS_URL"])
	})

	t.Run("with Git configuration", func(t *testing.T) {
		app := builders.NewApp().
			WithGitRepo("https://github.com/test/repo.git", "main").
			Build()
		
		assert.Equal(t, "https://github.com/test/repo.git", app.GitURL)
		assert.Equal(t, "main", app.Branch)
	})

	t.Run("with custom ID", func(t *testing.T) {
		app := builders.NewApp().
			WithID("custom-id-123").
			Build()
		
		assert.Equal(t, "custom-id-123", app.ID)
	})

	t.Run("with timestamps", func(t *testing.T) {
		now := time.Now()
		yesterday := now.Add(-24 * time.Hour)
		
		app := builders.NewApp().
			WithCreatedAt(yesterday).
			WithUpdatedAt(now).
			Build()
		
		assert.Equal(t, yesterday, app.CreatedAt)
		assert.Equal(t, now, app.UpdatedAt)
	})

	t.Run("with build configuration", func(t *testing.T) {
		buildTime := 5 * time.Minute
		
		app := builders.NewApp().
			WithBuildTime(buildTime).
			WithBuildCommand("go build -o app").
			Build()
		
		assert.Equal(t, buildTime, app.BuildTime)
		assert.Equal(t, "go build -o app", app.BuildCommand)
	})

	t.Run("fluent interface chaining", func(t *testing.T) {
		app := builders.NewApp().
			WithName("chained-app").
			WithLanguage("go").
			WithVersion("1.19").
			InLane("B").
			WithStatus("deployed").
			WithInstances(2).
			WithEnvVar("ENV", "test").
			WithGitRepo("https://github.com/test/chained.git", "develop").
			Build()
		
		assert.Equal(t, "chained-app", app.Name)
		assert.Equal(t, "go", app.Language)
		assert.Equal(t, "1.19", app.Version)
		assert.Equal(t, "B", app.Lane)
		assert.Equal(t, "deployed", app.Status)
		assert.Equal(t, 2, app.Instances)
		assert.Equal(t, "test", app.EnvVars["ENV"])
		assert.Equal(t, "https://github.com/test/chained.git", app.GitURL)
		assert.Equal(t, "develop", app.Branch)
	})

	t.Run("multiple builds from same builder", func(t *testing.T) {
		builder := builders.NewApp().WithLanguage("java")
		
		app1 := builder.WithName("app1").Build()
		app2 := builder.WithName("app2").Build()
		
		// Each build should be independent
		assert.Equal(t, "app1", app1.Name)
		assert.Equal(t, "app2", app2.Name)
		assert.Equal(t, "java", app1.Language)
		assert.Equal(t, "java", app2.Language)
		// Note: IDs will be the same since we're building from the same builder
		// Use separate builders or WithID() for different IDs
	})
}

func TestAppBuilder_EdgeCases(t *testing.T) {
	t.Run("empty string values", func(t *testing.T) {
		app := builders.NewApp().
			WithName("").
			WithLanguage("").
			Build()
		
		// Should allow empty strings if explicitly set
		assert.Equal(t, "", app.Name)
		assert.Equal(t, "", app.Language)
	})

	t.Run("nil environment variables", func(t *testing.T) {
		app := builders.NewApp().
			WithEnvVars(nil).
			Build()
		
		// Should handle nil gracefully
		assert.NotNil(t, app.EnvVars)
		assert.Empty(t, app.EnvVars)
	})

	t.Run("zero instances", func(t *testing.T) {
		app := builders.NewApp().
			WithInstances(0).
			Build()
		
		assert.Equal(t, 0, app.Instances)
	})
}