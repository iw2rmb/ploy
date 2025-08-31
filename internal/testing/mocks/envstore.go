package mocks

import (
	"github.com/stretchr/testify/mock"
	"github.com/iw2rmb/ploy/api/envstore"
)

// EnvStore provides a mock implementation of envstore.EnvStoreInterface
type EnvStore struct {
	mock.Mock
	data map[string]envstore.AppEnvVars // In-memory storage for testing
}

// NewEnvStore creates a new mock environment store
func NewEnvStore() *EnvStore {
	return &EnvStore{
		data: make(map[string]envstore.AppEnvVars),
	}
}

// GetAll retrieves all environment variables for an app
func (m *EnvStore) GetAll(app string) (envstore.AppEnvVars, error) {
	args := m.Called(app)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(envstore.AppEnvVars), args.Error(1)
}

// Set sets a single environment variable
func (m *EnvStore) Set(app, key, value string) error {
	args := m.Called(app, key, value)
	return args.Error(0)
}

// SetAll sets all environment variables for an app
func (m *EnvStore) SetAll(app string, envVars envstore.AppEnvVars) error {
	args := m.Called(app, envVars)
	return args.Error(0)
}

// Get retrieves a single environment variable
func (m *EnvStore) Get(app, key string) (string, bool, error) {
	args := m.Called(app, key)
	return args.String(0), args.Bool(1), args.Error(2)
}

// Delete deletes a single environment variable
func (m *EnvStore) Delete(app, key string) error {
	args := m.Called(app, key)
	return args.Error(0)
}

// ToStringArray converts environment variables to string array
func (m *EnvStore) ToStringArray(app string) ([]string, error) {
	args := m.Called(app)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]string), args.Error(1)
}

// Helper methods for easier mock setup

// WithApp sets up mock data for a specific app
func (m *EnvStore) WithApp(app string, envVars envstore.AppEnvVars) *EnvStore {
	m.data[app] = envVars
	m.On("GetAll", app).Return(envVars, nil)
	
	// Set up individual Get calls
	for key, value := range envVars {
		m.On("Get", app, key).Return(value, true, nil)
	}
	
	return m
}

// WithError sets up mock to return error for specific app
func (m *EnvStore) WithError(app string, err error) *EnvStore {
	m.On("GetAll", app).Return(nil, err)
	return m
}

// WithSetError sets up mock to return error when setting variables
func (m *EnvStore) WithSetError(app string, err error) *EnvStore {
	m.On("SetAll", app, mock.Anything).Return(err)
	return m
}