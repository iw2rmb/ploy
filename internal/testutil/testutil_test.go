package testutil

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/iw2rmb/ploy/api/envstore"
)

func TestNewTestDataRepository(t *testing.T) {
	repo := NewTestDataRepository()
	
	assert.NotNil(t, repo)
	assert.Len(t, repo.Apps, 6, "Should have 6 test apps")
	assert.Len(t, repo.BuildConfigs, 5, "Should have 5 build configs")
	assert.Len(t, repo.StorageItems, 5, "Should have 5 storage items")
	assert.Len(t, repo.EnvVarSets, 5, "Should have 5 env var sets")
}

func TestGetTestApp(t *testing.T) {
	repo := NewTestDataRepository()
	
	tests := []struct {
		name        string
		appName     string
		shouldExist bool
	}{
		{"existing go app", TestAppGoAPI, true},
		{"existing node app", TestAppNodeFrontend, true},
		{"existing java app", TestAppJavaService, true},
		{"non-existent app", "non-existent", false},
	}
	
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			app := repo.GetTestApp(tt.appName)
			if tt.shouldExist {
				assert.NotNil(t, app)
				assert.Equal(t, tt.appName, app.Name)
			} else {
				assert.Nil(t, app)
			}
		})
	}
}

func TestGetTestBuildConfig(t *testing.T) {
	repo := NewTestDataRepository()
	
	tests := []struct {
		name        string
		lane        string
		shouldExist bool
	}{
		{"lane A", TestLaneA, true},
		{"lane B", TestLaneB, true},
		{"lane C", TestLaneC, true},
		{"non-existent lane", "X", false},
	}
	
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			config := repo.GetTestBuildConfig(tt.lane)
			if tt.shouldExist {
				assert.NotNil(t, config)
				assert.Equal(t, tt.lane, config.Lane)
			} else {
				assert.Nil(t, config)
			}
		})
	}
}

func TestAppTestBuilder(t *testing.T) {
	builder := NewAppTestBuilder()
	
	app := builder.
		Named("test-app").
		WithLanguage("go").
		InLane("A").
		Version("2.0.0").
		WithStatus("running").
		WithInstances(3).
		WithEnvVar("PORT", "9090").
		WithGitRepo("https://github.com/test/app.git", "develop").
		WithBuildTime(5 * time.Minute).
		Build()
	
	assert.Equal(t, "test-app", app.Name)
	assert.Equal(t, "go", app.Language)
	assert.Equal(t, "A", app.Lane)
	assert.Equal(t, "2.0.0", app.Version)
	assert.Equal(t, "running", app.Status)
	assert.Equal(t, 3, app.Instances)
	assert.Equal(t, "9090", app.EnvVars["PORT"])
	assert.Equal(t, "https://github.com/test/app.git", app.GitURL)
	assert.Equal(t, "develop", app.Branch)
	assert.Equal(t, 5*time.Minute, app.BuildTime)
}

func TestHTTPTestBuilder(t *testing.T) {
	builder := NewHTTPTestBuilder()
	
	// Test GET request
	req, err := builder.
		GET("/api/v1/apps").
		WithHeader("Authorization", "Bearer token").
		WithQuery("limit", "10").
		BuildRequest()
	
	require.NoError(t, err)
	assert.Equal(t, "GET", req.Method)
	assert.Equal(t, "/api/v1/apps", req.URL.Path)
	assert.Equal(t, "Bearer token", req.Header.Get("Authorization"))
	assert.Equal(t, "10", req.URL.Query().Get("limit"))
	
	// Test POST request with JSON
	builder = NewHTTPTestBuilder()
	body := map[string]string{"name": "test-app"}
	
	req, err = builder.
		POST("/api/v1/apps").
		WithJSON(body).
		WithAuth("secret-token").
		BuildRequest()
	
	require.NoError(t, err)
	assert.Equal(t, "POST", req.Method)
	assert.Equal(t, "application/json", req.Header.Get("Content-Type"))
	assert.Equal(t, "Bearer secret-token", req.Header.Get("Authorization"))
}

func TestMockEnvStore(t *testing.T) {
	store := NewMockEnvStore()
	
	// Test WithApp helper
	appEnvVars := envstore.AppEnvVars{
		"KEY1": "value1",
		"KEY2": "value2",
	}
	
	store.WithApp("test-app", appEnvVars)
	
	// Verify GetAll works
	vars, err := store.GetAll("test-app")
	assert.NoError(t, err)
	assert.Equal(t, appEnvVars, vars)
	
	// Verify Get works for individual keys
	value, exists, err := store.Get("test-app", "KEY1")
	assert.NoError(t, err)
	assert.True(t, exists)
	assert.Equal(t, "value1", value)
}

func TestMockFactory(t *testing.T) {
	factory := NewMockFactory()
	
	t.Run("successful env store", func(t *testing.T) {
		apps := map[string]envstore.AppEnvVars{
			"app1": {"KEY": "value1"},
			"app2": {"KEY": "value2"},
		}
		
		store := factory.CreateSuccessfulEnvStore(apps)
		assert.NotNil(t, store)
		
		// Verify it works
		vars, err := store.GetAll("app1")
		assert.NoError(t, err)
		assert.Equal(t, "value1", vars["KEY"])
	})
	
	t.Run("failing env store", func(t *testing.T) {
		store := factory.CreateFailingEnvStore("connection error")
		assert.NotNil(t, store)
		
		// Verify it fails
		_, err := store.GetAll("any-app")
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "connection error")
	})
	
	t.Run("successful storage client", func(t *testing.T) {
		files := map[string][]byte{
			"file1.txt": []byte("content1"),
			"file2.txt": []byte("content2"),
		}
		
		client := factory.CreateSuccessfulStorageClient(files)
		assert.NotNil(t, client)
		
		// Verify it works
		data, err := client.Download(nil, "file1.txt")
		assert.NoError(t, err)
		assert.Equal(t, []byte("content1"), data)
	})
	
	t.Run("canceled context", func(t *testing.T) {
		ctx := factory.CreateCanceledContext()
		assert.NotNil(t, ctx)
		assert.Error(t, ctx.Err())
	})
}

func TestRealisticEnvVarsBuilder(t *testing.T) {
	builder := NewRealisticEnvVarsBuilder()
	
	t.Run("Go app env vars", func(t *testing.T) {
		vars := builder.ForGoApp("test-go-app")
		assert.Equal(t, "production", vars["GO_ENV"])
		assert.Equal(t, "8080", vars["PORT"])
		assert.Equal(t, "test-go-app", vars["APP_NAME"])
	})
	
	t.Run("Node app env vars", func(t *testing.T) {
		vars := builder.ForNodeApp("test-node-app")
		assert.Equal(t, "production", vars["NODE_ENV"])
		assert.Equal(t, "3000", vars["PORT"])
		assert.Equal(t, "test-node-app", vars["APP_NAME"])
	})
	
	t.Run("Java app env vars", func(t *testing.T) {
		vars := builder.ForJavaApp("test-java-app")
		assert.Contains(t, vars["JAVA_OPTS"], "-Xmx1g")
		assert.Equal(t, "prod", vars["SPRING_PROFILES"])
		assert.Equal(t, "test-java-app", vars["APP_NAME"])
	})
	
	t.Run("Python app env vars", func(t *testing.T) {
		vars := builder.ForPythonApp("test-python-app")
		assert.Equal(t, "production", vars["PYTHON_ENV"])
		assert.Equal(t, "8000", vars["PORT"])
		assert.Equal(t, "test-python-app", vars["APP_NAME"])
	})
}

func TestRealisticStorageBuilder(t *testing.T) {
	builder := NewRealisticStorageBuilder()
	
	items := builder.ForApp("test-app", "v1.0.0")
	
	assert.Len(t, items, 3)
	
	// Check source tarball
	assert.Contains(t, items[0].Key, "apps/test-app/v1.0.0/source.tar.gz")
	assert.Equal(t, int64(2*1024*1024), items[0].Size)
	assert.Equal(t, "application/gzip", items[0].ContentType)
	
	// Check build artifact
	assert.Contains(t, items[1].Key, "builds/test-app/v1.0.0/artifact.tar")
	assert.Equal(t, int64(10*1024*1024), items[1].Size)
	assert.Equal(t, "application/x-tar", items[1].ContentType)
	
	// Check logs
	assert.Contains(t, items[2].Key, "logs/test-app/")
	assert.Equal(t, int64(50*1024), items[2].Size)
	assert.Equal(t, "text/plain", items[2].ContentType)
}