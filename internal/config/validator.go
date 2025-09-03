package config

// Validator validates a fully merged configuration.
type Validator interface {
    Validate(*Config) error
}

// StructValidator performs basic cross-field checks.
// Kept intentionally minimal for Phase 3 slice.
type StructValidator struct{}

func NewStructValidator() *StructValidator { return &StructValidator{} }

func (sv *StructValidator) Validate(cfg *Config) error {
    // If S3 provider is selected, require Region
    if cfg != nil && cfg.Storage.Provider == "s3" && cfg.Storage.Region == "" {
        return ErrValidation("s3 region is required when provider is s3")
    }
    // If SeaweedFS provider is selected, require Endpoint (master/filer base)
    if cfg != nil && cfg.Storage.Provider == "seaweedfs" && cfg.Storage.Endpoint == "" {
        return ErrValidation("seaweedfs endpoint is required when provider is seaweedfs")
    }
    return nil
}

// ErrValidation provides a lightweight error type for validation failures.
type ErrValidation string

func (e ErrValidation) Error() string { return string(e) }
