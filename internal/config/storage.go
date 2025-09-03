package config

// StorageConfig is a minimal schema to support provider selection.
// For this slice we only require Provider and accept everything else as zero values.
type StorageConfig struct {
    Provider string `yaml:"provider" json:"provider"`
    Endpoint string `yaml:"endpoint" json:"endpoint"`
    Bucket   string `yaml:"bucket" json:"bucket"`
    Region   string `yaml:"region" json:"region"`
    // Provider-specific fields may be added incrementally in future slices.
}
