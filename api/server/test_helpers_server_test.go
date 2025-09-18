package server

import (
	"github.com/gofiber/fiber/v2"
	envstore "github.com/iw2rmb/ploy/internal/envstore"
)

type stubEnvStore struct {
	data map[string]envstore.AppEnvVars
}

func newStubEnvStore() *stubEnvStore {
	return &stubEnvStore{data: make(map[string]envstore.AppEnvVars)}
}

func (s *stubEnvStore) clone(app string) envstore.AppEnvVars {
	src := s.data[app]
	if src == nil {
		return envstore.AppEnvVars{}
	}
	clone := make(envstore.AppEnvVars, len(src))
	for k, v := range src {
		clone[k] = v
	}
	return clone
}

func (s *stubEnvStore) GetAll(app string) (envstore.AppEnvVars, error) {
	return s.clone(app), nil
}

func (s *stubEnvStore) Set(app, key, value string) error {
	vars := s.clone(app)
	vars[key] = value
	s.data[app] = vars
	return nil
}

func (s *stubEnvStore) SetAll(app string, envVars envstore.AppEnvVars) error {
	vars := make(envstore.AppEnvVars, len(envVars))
	for k, v := range envVars {
		vars[k] = v
	}
	s.data[app] = vars
	return nil
}

func (s *stubEnvStore) Get(app, key string) (string, bool, error) {
	vars := s.data[app]
	if vars == nil {
		return "", false, nil
	}
	v, ok := vars[key]
	return v, ok, nil
}

func (s *stubEnvStore) Delete(app, key string) error {
	vars := s.clone(app)
	delete(vars, key)
	s.data[app] = vars
	return nil
}

func (s *stubEnvStore) ToStringArray(app string) ([]string, error) {
	vars := s.data[app]
	if vars == nil {
		return nil, nil
	}
	result := make([]string, 0, len(vars))
	for k, v := range vars {
		result = append(result, k+"="+v)
	}
	return result, nil
}

// createMockServer provides a minimal Server instance suitable for handler tests.
// It initializes a Fiber app and empty dependencies; tests can override fields.
func createMockServer() *Server {
	return &Server{
		app:          fiber.New(),
		config:       &ControllerConfig{},
		dependencies: &ServiceDependencies{EnvStore: newStubEnvStore()},
	}
}
