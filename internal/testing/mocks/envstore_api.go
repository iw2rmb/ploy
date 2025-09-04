package mocks

import (
    apienv "github.com/iw2rmb/ploy/api/envstore"
    "github.com/stretchr/testify/mock"
)

// EnvStoreAPI provides a mock implementation for api/envstore.EnvStoreInterface
type EnvStoreAPI struct {
    mock.Mock
    data map[string]apienv.AppEnvVars
}

func NewAPIEnvStore() *EnvStoreAPI {
    return &EnvStoreAPI{data: make(map[string]apienv.AppEnvVars)}
}

func (m *EnvStoreAPI) GetAll(app string) (apienv.AppEnvVars, error) {
    args := m.Called(app)
    if args.Get(0) == nil {
        return nil, args.Error(1)
    }
    return args.Get(0).(apienv.AppEnvVars), args.Error(1)
}

func (m *EnvStoreAPI) Set(app, key, value string) error {
    args := m.Called(app, key, value)
    return args.Error(0)
}

func (m *EnvStoreAPI) SetAll(app string, envVars apienv.AppEnvVars) error {
    args := m.Called(app, envVars)
    return args.Error(0)
}

func (m *EnvStoreAPI) Get(app, key string) (string, bool, error) {
    args := m.Called(app, key)
    return args.String(0), args.Bool(1), args.Error(2)
}

func (m *EnvStoreAPI) Delete(app, key string) error {
    args := m.Called(app, key)
    return args.Error(0)
}

func (m *EnvStoreAPI) ToStringArray(app string) ([]string, error) {
    args := m.Called(app)
    if args.Get(0) == nil {
        return nil, args.Error(1)
    }
    return args.Get(0).([]string), args.Error(1)
}

