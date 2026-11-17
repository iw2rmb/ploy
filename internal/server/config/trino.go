package config

import "os"

type TrinoConfig struct {
	User     string
	Password string
}

// LoadTrinoFromEnv must run after server-target environment projection.
// Credentials remain exact because whitespace can be part of a password.
func LoadTrinoFromEnv() TrinoConfig {
	return TrinoConfig{
		User:     os.Getenv("PLOY_TRINO_USER"),
		Password: os.Getenv("PLOY_TRINO_PASSWORD"),
	}
}
