package config

// Config represents the ployd daemon configuration.
type Config struct {
	HTTP                  HTTPConfig
	Metrics               MetricsConfig
	Auth                  AuthConfig
	Admin                 AdminConfig
	PKI                   PKIConfig
	Scheduler             SchedulerConfig
	Logging               LoggingConfig
	Postgres              PostgresConfig
	GitLab                GitLabConfig
	SpecRepos             SpecRepos
	NamedSpecEnvAllowlist NamedSpecEnvAllowlist
	ObjectStore           ObjectStoreConfig
}
