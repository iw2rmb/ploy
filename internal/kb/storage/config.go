package storage

import "time"

// Config holds configuration for KB storage
type Config struct {
	StorageURL string        `json:"storage_url"`
	Timeout    time.Duration `json:"timeout"`
}
