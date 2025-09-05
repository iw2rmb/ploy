package distribution

import "errors"

// Distribution errors
var (
	ErrInvalidVersion      = errors.New("invalid version")
	ErrInvalidPlatform     = errors.New("invalid platform")
	ErrInvalidArchitecture = errors.New("invalid architecture")
	ErrMissingHash         = errors.New("missing SHA256 hash")
	ErrInvalidSize         = errors.New("invalid size")
	ErrBinaryNotFound      = errors.New("binary not found")
	ErrIntegrityFailure    = errors.New("integrity verification failed")
	ErrCacheWriteFailure   = errors.New("failed to write to cache")
	ErrStorageFailure      = errors.New("storage operation failed")
)
