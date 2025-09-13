package envstore

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
)

type EnvStore struct {
	basePath string
	mu       sync.RWMutex
}

type AppEnvVars map[string]string

func New(basePath string) *EnvStore {
	_ = os.MkdirAll(basePath, 0755)
	return &EnvStore{basePath: basePath}
}

func (s *EnvStore) envFilePath(app string) string {
	return filepath.Join(s.basePath, fmt.Sprintf("%s.env.json", app))
}

func (s *EnvStore) GetAll(app string) (AppEnvVars, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	filePath := s.envFilePath(app)
	if _, err := os.Stat(filePath); os.IsNotExist(err) {
		return AppEnvVars{}, nil
	}

	data, err := os.ReadFile(filePath)
	if err != nil {
		return nil, err
	}

	var envVars AppEnvVars
	if err := json.Unmarshal(data, &envVars); err != nil {
		return nil, err
	}

	return envVars, nil
}

func (s *EnvStore) Set(app, key, value string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	envVars, err := s.getUnsafe(app)
	if err != nil {
		return err
	}

	if envVars == nil {
		envVars = make(AppEnvVars)
	}

	envVars[key] = value
	return s.saveUnsafe(app, envVars)
}

func (s *EnvStore) SetAll(app string, envVars AppEnvVars) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	return s.saveUnsafe(app, envVars)
}

func (s *EnvStore) Get(app, key string) (string, bool, error) {
	envVars, err := s.GetAll(app)
	if err != nil {
		return "", false, err
	}

	value, exists := envVars[key]
	return value, exists, nil
}

func (s *EnvStore) Delete(app, key string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	envVars, err := s.getUnsafe(app)
	if err != nil {
		return err
	}

	if envVars == nil {
		return nil
	}

	delete(envVars, key)
	return s.saveUnsafe(app, envVars)
}

func (s *EnvStore) getUnsafe(app string) (AppEnvVars, error) {
	filePath := s.envFilePath(app)
	if _, err := os.Stat(filePath); os.IsNotExist(err) {
		return nil, nil
	}

	data, err := os.ReadFile(filePath)
	if err != nil {
		return nil, err
	}

	var envVars AppEnvVars
	if err := json.Unmarshal(data, &envVars); err != nil {
		return nil, err
	}

	return envVars, nil
}

func (s *EnvStore) saveUnsafe(app string, envVars AppEnvVars) error {
	data, err := json.MarshalIndent(envVars, "", "  ")
	if err != nil {
		return err
	}

	filePath := s.envFilePath(app)
	return os.WriteFile(filePath, data, 0644)
}

func (s *EnvStore) ToStringArray(app string) ([]string, error) {
	envVars, err := s.GetAll(app)
	if err != nil {
		return nil, err
	}

	var result []string
	for key, value := range envVars {
		result = append(result, fmt.Sprintf("%s=%s", key, value))
	}

	return result, nil
}
