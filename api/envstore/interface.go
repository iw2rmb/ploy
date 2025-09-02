package envstore

// EnvStoreInterface defines the common interface for environment variable storage
type EnvStoreInterface interface {
	GetAll(app string) (AppEnvVars, error)
	Set(app, key, value string) error
	SetAll(app string, envVars AppEnvVars) error
	Get(app, key string) (string, bool, error)
	Delete(app, key string) error
	ToStringArray(app string) ([]string, error)
}

// Ensure our file-based EnvStore implements the interface
var _ EnvStoreInterface = (*EnvStore)(nil)