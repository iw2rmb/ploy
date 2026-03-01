package config

// Config represents the ployd daemon configuration.
type Config struct {
	HTTP        HTTPConfig        `yaml:"http"`
	Metrics     MetricsConfig     `yaml:"metrics"`
	Auth        AuthConfig        `yaml:"auth"`
	Admin       AdminConfig       `yaml:"admin"`
	PKI         PKIConfig         `yaml:"pki"`
	Scheduler   SchedulerConfig   `yaml:"scheduler"`
	Logging     LoggingConfig     `yaml:"logging"`
	Postgres    PostgresConfig    `yaml:"postgres"`
	GitLab      GitLabConfig      `yaml:"gitlab"`
	ObjectStore ObjectStoreConfig `yaml:"object_store"`
	FilePath    string            `yaml:"-"`
}
