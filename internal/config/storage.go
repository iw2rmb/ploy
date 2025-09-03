package config

// StorageConfig is a minimal schema to support provider selection.
// For this slice we only require Provider and accept everything else as zero values.
type StorageConfig struct {
    Provider string `yaml:"provider" json:"provider"`
    Endpoint string `yaml:"endpoint" json:"endpoint"`
    Bucket   string `yaml:"bucket" json:"bucket"`
    Region   string `yaml:"region" json:"region"`
    Retry    RetrySettings `yaml:"retry" json:"retry"`
    Cache    CacheSettings `yaml:"cache" json:"cache"`
    // Provider-specific fields may be added incrementally in future slices.
}

// RetrySettings enables retry middleware configuration via config service
type RetrySettings struct {
    Enabled           bool    `yaml:"enabled" json:"enabled"`
    MaxAttempts       int     `yaml:"max_attempts" json:"max_attempts"`
    InitialDelay      string  `yaml:"initial_delay" json:"initial_delay"`
    MaxDelay          string  `yaml:"max_delay" json:"max_delay"`
    BackoffMultiplier float64 `yaml:"backoff_multiplier" json:"backoff_multiplier"`
}

// CacheSettings enables cache middleware configuration via config service
type CacheSettings struct {
    Enabled bool   `yaml:"enabled" json:"enabled"`
    MaxSize int    `yaml:"max_size" json:"max_size"`
    TTL     string `yaml:"ttl" json:"ttl"`
}
