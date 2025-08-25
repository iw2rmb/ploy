// Package builders provides builder patterns for constructing test objects
package builders

import (
	"time"

	"github.com/iw2rmb/ploy/internal/testutils/mocks"
)

// ApplicationBuilder helps build test applications
type ApplicationBuilder struct {
	id        string
	name      string
	language  string
	lane      string
	status    string
	createdAt time.Time
	updatedAt time.Time
}

// NewApplicationBuilder creates a new application builder with defaults
func NewApplicationBuilder() *ApplicationBuilder {
	return &ApplicationBuilder{
		id:        "test-app-id",
		name:      "test-app",
		language:  "go",
		lane:      "B",
		status:    "created",
		createdAt: time.Now(),
		updatedAt: time.Now(),
	}
}

// WithID sets the application ID
func (b *ApplicationBuilder) WithID(id string) *ApplicationBuilder {
	b.id = id
	return b
}

// WithName sets the application name
func (b *ApplicationBuilder) WithName(name string) *ApplicationBuilder {
	b.name = name
	return b
}

// WithLanguage sets the programming language
func (b *ApplicationBuilder) WithLanguage(language string) *ApplicationBuilder {
	b.language = language
	return b
}

// WithLane sets the deployment lane
func (b *ApplicationBuilder) WithLane(lane string) *ApplicationBuilder {
	b.lane = lane
	return b
}

// WithStatus sets the application status
func (b *ApplicationBuilder) WithStatus(status string) *ApplicationBuilder {
	b.status = status
	return b
}

// WithTimestamps sets both created and updated timestamps
func (b *ApplicationBuilder) WithTimestamps(createdAt, updatedAt time.Time) *ApplicationBuilder {
	b.createdAt = createdAt
	b.updatedAt = updatedAt
	return b
}

// Build creates the application object
func (b *ApplicationBuilder) Build() *MockApplication {
	return &MockApplication{
		ID:        b.id,
		Name:      b.name,
		Language:  b.language,
		Lane:      b.lane,
		Status:    b.status,
		CreatedAt: b.createdAt,
		UpdatedAt: b.updatedAt,
	}
}

// MockApplication represents a test application
type MockApplication struct {
	ID        string
	Name      string
	Language  string
	Lane      string
	Status    string
	CreatedAt time.Time
	UpdatedAt time.Time
}

// DeploymentBuilder helps build test deployments
type DeploymentBuilder struct {
	id          string
	appID       string
	version     string
	artifactURL string
	status      string
	deployedAt  time.Time
	healthURL   string
	environment map[string]string
}

// NewDeploymentBuilder creates a new deployment builder with defaults
func NewDeploymentBuilder() *DeploymentBuilder {
	return &DeploymentBuilder{
		id:          "test-deployment-id",
		appID:       "test-app-id",
		version:     "v1.0.0",
		artifactURL: "http://localhost:8888/artifacts/test-app-v1.0.0.tar.gz",
		status:      "pending",
		deployedAt:  time.Now(),
		healthURL:   "http://test-app.local.dev/health",
		environment: make(map[string]string),
	}
}

// WithID sets the deployment ID
func (b *DeploymentBuilder) WithID(id string) *DeploymentBuilder {
	b.id = id
	return b
}

// WithAppID sets the application ID
func (b *DeploymentBuilder) WithAppID(appID string) *DeploymentBuilder {
	b.appID = appID
	return b
}

// WithVersion sets the version
func (b *DeploymentBuilder) WithVersion(version string) *DeploymentBuilder {
	b.version = version
	return b
}

// WithArtifactURL sets the artifact URL
func (b *DeploymentBuilder) WithArtifactURL(url string) *DeploymentBuilder {
	b.artifactURL = url
	return b
}

// WithStatus sets the deployment status
func (b *DeploymentBuilder) WithStatus(status string) *DeploymentBuilder {
	b.status = status
	return b
}

// WithHealthURL sets the health check URL
func (b *DeploymentBuilder) WithHealthURL(url string) *DeploymentBuilder {
	b.healthURL = url
	return b
}

// WithEnvironment sets environment variables
func (b *DeploymentBuilder) WithEnvironment(env map[string]string) *DeploymentBuilder {
	b.environment = env
	return b
}

// WithEnvVar adds a single environment variable
func (b *DeploymentBuilder) WithEnvVar(key, value string) *DeploymentBuilder {
	if b.environment == nil {
		b.environment = make(map[string]string)
	}
	b.environment[key] = value
	return b
}

// Build creates the deployment object
func (b *DeploymentBuilder) Build() *MockDeployment {
	return &MockDeployment{
		ID:          b.id,
		AppID:       b.appID,
		Version:     b.version,
		ArtifactURL: b.artifactURL,
		Status:      b.status,
		DeployedAt:  b.deployedAt,
		HealthURL:   b.healthURL,
		Environment: b.environment,
	}
}

// MockDeployment represents a test deployment
type MockDeployment struct {
	ID          string
	AppID       string
	Version     string
	ArtifactURL string
	Status      string
	DeployedAt  time.Time
	HealthURL   string
	Environment map[string]string
}

// ConsulServiceBuilder helps build test Consul services
type ConsulServiceBuilder struct {
	id      string
	name    string
	tags    []string
	address string
	port    int
	health  mocks.ServiceHealth
	meta    map[string]string
}

// NewConsulServiceBuilder creates a new Consul service builder with defaults
func NewConsulServiceBuilder() *ConsulServiceBuilder {
	return &ConsulServiceBuilder{
		id:      "test-service-id",
		name:    "test-service",
		tags:    []string{"test"},
		address: "127.0.0.1",
		port:    8080,
		health:  mocks.ServiceHealthPassing,
		meta:    make(map[string]string),
	}
}

// WithID sets the service ID
func (b *ConsulServiceBuilder) WithID(id string) *ConsulServiceBuilder {
	b.id = id
	return b
}

// WithName sets the service name
func (b *ConsulServiceBuilder) WithName(name string) *ConsulServiceBuilder {
	b.name = name
	return b
}

// WithTags sets the service tags
func (b *ConsulServiceBuilder) WithTags(tags []string) *ConsulServiceBuilder {
	b.tags = tags
	return b
}

// WithTag adds a single tag
func (b *ConsulServiceBuilder) WithTag(tag string) *ConsulServiceBuilder {
	b.tags = append(b.tags, tag)
	return b
}

// WithAddress sets the service address
func (b *ConsulServiceBuilder) WithAddress(address string) *ConsulServiceBuilder {
	b.address = address
	return b
}

// WithPort sets the service port
func (b *ConsulServiceBuilder) WithPort(port int) *ConsulServiceBuilder {
	b.port = port
	return b
}

// WithHealth sets the service health status
func (b *ConsulServiceBuilder) WithHealth(health mocks.ServiceHealth) *ConsulServiceBuilder {
	b.health = health
	return b
}

// WithMeta sets service metadata
func (b *ConsulServiceBuilder) WithMeta(meta map[string]string) *ConsulServiceBuilder {
	b.meta = meta
	return b
}

// WithMetaValue adds a single metadata key-value pair
func (b *ConsulServiceBuilder) WithMetaValue(key, value string) *ConsulServiceBuilder {
	if b.meta == nil {
		b.meta = make(map[string]string)
	}
	b.meta[key] = value
	return b
}

// Build creates the Consul service object
func (b *ConsulServiceBuilder) Build() *mocks.MockService {
	return &mocks.MockService{
		ID:      b.id,
		Name:    b.name,
		Tags:    b.tags,
		Address: b.address,
		Port:    b.port,
		Health:  b.health,
		Meta:    b.meta,
	}
}

// NomadJobBuilder helps build test Nomad jobs
type NomadJobBuilder struct {
	id          string
	name        string
	status      mocks.JobStatus
	allocations []mocks.MockAllocation
	createdAt   time.Time
	updatedAt   time.Time
}

// NewNomadJobBuilder creates a new Nomad job builder with defaults
func NewNomadJobBuilder() *NomadJobBuilder {
	return &NomadJobBuilder{
		id:          "test-job-id",
		name:        "test-job",
		status:      mocks.JobStatusPending,
		allocations: []mocks.MockAllocation{},
		createdAt:   time.Now(),
		updatedAt:   time.Now(),
	}
}

// WithID sets the job ID
func (b *NomadJobBuilder) WithID(id string) *NomadJobBuilder {
	b.id = id
	return b
}

// WithName sets the job name
func (b *NomadJobBuilder) WithName(name string) *NomadJobBuilder {
	b.name = name
	return b
}

// WithStatus sets the job status
func (b *NomadJobBuilder) WithStatus(status mocks.JobStatus) *NomadJobBuilder {
	b.status = status
	return b
}

// WithAllocations sets the job allocations
func (b *NomadJobBuilder) WithAllocations(allocations []mocks.MockAllocation) *NomadJobBuilder {
	b.allocations = allocations
	return b
}

// WithAllocation adds a single allocation
func (b *NomadJobBuilder) WithAllocation(alloc mocks.MockAllocation) *NomadJobBuilder {
	b.allocations = append(b.allocations, alloc)
	return b
}

// WithRunningAllocation adds a running allocation
func (b *NomadJobBuilder) WithRunningAllocation(allocID, nodeID string) *NomadJobBuilder {
	alloc := mocks.MockAllocation{
		ID:            allocID,
		JobID:         b.id,
		NodeID:        nodeID,
		TaskGroup:     "app",
		Status:        "running",
		DesiredStatus: "run",
		ClientStatus:  "running",
		CreatedAt:     time.Now(),
	}
	b.allocations = append(b.allocations, alloc)
	return b
}

// Build creates the Nomad job object
func (b *NomadJobBuilder) Build() *mocks.MockJob {
	return &mocks.MockJob{
		ID:          b.id,
		Name:        b.name,
		Status:      b.status,
		Allocations: b.allocations,
		CreatedAt:   b.createdAt,
		UpdatedAt:   b.updatedAt,
	}
}

// BuildLogBuilder helps build test build logs
type BuildLogBuilder struct {
	id           string
	appID        string
	deploymentID *string
	phase        string
	message      string
	level        string
	timestamp    time.Time
}

// NewBuildLogBuilder creates a new build log builder with defaults
func NewBuildLogBuilder() *BuildLogBuilder {
	return &BuildLogBuilder{
		id:        "test-log-id",
		appID:     "test-app-id",
		phase:     "build",
		message:   "Test log message",
		level:     "INFO",
		timestamp: time.Now(),
	}
}

// WithID sets the log ID
func (b *BuildLogBuilder) WithID(id string) *BuildLogBuilder {
	b.id = id
	return b
}

// WithAppID sets the application ID
func (b *BuildLogBuilder) WithAppID(appID string) *BuildLogBuilder {
	b.appID = appID
	return b
}

// WithDeploymentID sets the deployment ID
func (b *BuildLogBuilder) WithDeploymentID(deploymentID string) *BuildLogBuilder {
	b.deploymentID = &deploymentID
	return b
}

// WithPhase sets the build phase
func (b *BuildLogBuilder) WithPhase(phase string) *BuildLogBuilder {
	b.phase = phase
	return b
}

// WithMessage sets the log message
func (b *BuildLogBuilder) WithMessage(message string) *BuildLogBuilder {
	b.message = message
	return b
}

// WithLevel sets the log level
func (b *BuildLogBuilder) WithLevel(level string) *BuildLogBuilder {
	b.level = level
	return b
}

// WithTimestamp sets the log timestamp
func (b *BuildLogBuilder) WithTimestamp(timestamp time.Time) *BuildLogBuilder {
	b.timestamp = timestamp
	return b
}

// Build creates the build log object
func (b *BuildLogBuilder) Build() *MockBuildLog {
	return &MockBuildLog{
		ID:           b.id,
		AppID:        b.appID,
		DeploymentID: b.deploymentID,
		Phase:        b.phase,
		Message:      b.message,
		Level:        b.level,
		Timestamp:    b.timestamp,
	}
}

// MockBuildLog represents a test build log
type MockBuildLog struct {
	ID           string
	AppID        string
	DeploymentID *string
	Phase        string
	Message      string
	Level        string
	Timestamp    time.Time
}