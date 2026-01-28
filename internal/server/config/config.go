package config

// Config represents the ployd daemon configuration.
type Config struct {
	HTTP         HTTPConfig          `yaml:"http"`
	Metrics      MetricsConfig       `yaml:"metrics"`
	Auth         AuthConfig          `yaml:"auth"`
	Admin        AdminConfig         `yaml:"admin"`
	ControlPlane ControlPlaneConfig  `yaml:"control_plane"`
	PKI          PKIConfig           `yaml:"pki"`
	Bootstrap    BootstrapConfig     `yaml:"bootstrap"`
	Worker       WorkerConfig        `yaml:"worker"`
	Beacon       BeaconConfig        `yaml:"beacon"`
	Runtime      RuntimeConfig       `yaml:"runtime"`
	Scheduler    SchedulerConfig     `yaml:"scheduler"`
	Logging      LoggingConfig       `yaml:"logging"`
	Transfers    TransfersConfig     `yaml:"transfers"`
	Postgres     PostgresConfig      `yaml:"postgres"`
	GitLab       GitLabConfig        `yaml:"gitlab"`
	ObjectStore  ObjectStoreConfig   `yaml:"object_store"`
	FilePath     string              `yaml:"-"`
	Environment  map[string]string   `yaml:"environment"`
	Features     map[string]bool     `yaml:"features"`
	Tags         map[string]string   `yaml:"tags"`
	Metadata     map[string]any      `yaml:"metadata"`
	Extra        map[string]any      `yaml:"extra"`
	rawPlugins   map[string]struct{} `yaml:"-"`
}
