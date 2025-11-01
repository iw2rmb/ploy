package main

import (
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/iw2rmb/ploy/internal/api/config"
)

func TestResolvePgDSN(t *testing.T) {
	tests := []struct {
		name        string
		envVars     map[string]string
		configDSN   string
		expected    string
		description string
	}{
		{
			name:        "PLOY_SERVER_PG_DSN takes precedence",
			envVars:     map[string]string{"PLOY_SERVER_PG_DSN": "postgres://server/db"},
			configDSN:   "postgres://config/db",
			expected:    "postgres://server/db",
			description: "PLOY_SERVER_PG_DSN should take precedence over config",
		},
		{
			name:        "PLOY_POSTGRES_DSN fallback",
			envVars:     map[string]string{"PLOY_POSTGRES_DSN": "postgres://fallback/db"},
			configDSN:   "postgres://config/db",
			expected:    "postgres://fallback/db",
			description: "PLOY_POSTGRES_DSN should be used when PLOY_SERVER_PG_DSN is not set",
		},
		{
			name:        "Config DSN used when no env vars",
			envVars:     map[string]string{},
			configDSN:   "postgres://config/db",
			expected:    "postgres://config/db",
			description: "Config DSN should be used when no env vars are set",
		},
		{
			name:        "PLOY_SERVER_PG_DSN overrides PLOY_POSTGRES_DSN",
			envVars:     map[string]string{"PLOY_SERVER_PG_DSN": "postgres://server/db", "PLOY_POSTGRES_DSN": "postgres://fallback/db"},
			configDSN:   "postgres://config/db",
			expected:    "postgres://server/db",
			description: "PLOY_SERVER_PG_DSN should take precedence over PLOY_POSTGRES_DSN",
		},
		{
			name:        "Empty env vars ignored",
			envVars:     map[string]string{"PLOY_SERVER_PG_DSN": "  ", "PLOY_POSTGRES_DSN": ""},
			configDSN:   "postgres://config/db",
			expected:    "postgres://config/db",
			description: "Whitespace-only env vars should be ignored",
		},
		{
			name:        "Empty when nothing configured",
			envVars:     map[string]string{},
			configDSN:   "",
			expected:    "",
			description: "Should return empty string when nothing is configured",
		},
		{
			name:        "Whitespace trimmed from config",
			envVars:     map[string]string{},
			configDSN:   "  postgres://config/db  ",
			expected:    "postgres://config/db",
			description: "Whitespace should be trimmed from config DSN",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Save and restore original env vars.
			origServerDSN := os.Getenv("PLOY_SERVER_PG_DSN")
			origPostgresDSN := os.Getenv("PLOY_POSTGRES_DSN")
			defer func() {
				os.Setenv("PLOY_SERVER_PG_DSN", origServerDSN)
				os.Setenv("PLOY_POSTGRES_DSN", origPostgresDSN)
			}()

			// Clear env vars.
			os.Unsetenv("PLOY_SERVER_PG_DSN")
			os.Unsetenv("PLOY_POSTGRES_DSN")

			// Set test env vars.
			for k, v := range tt.envVars {
				os.Setenv(k, v)
			}

			// Create test config.
			cfg := config.Config{
				Postgres: config.PostgresConfig{
					DSN: tt.configDSN,
				},
			}

			got := resolvePgDSN(cfg)
			if got != tt.expected {
				t.Errorf("resolvePgDSN() = %q, want %q\n%s", got, tt.expected, tt.description)
			}
		})
	}
}

func TestParseLogLevel(t *testing.T) {
	tests := []struct {
		input    string
		expected slog.Level
	}{
		{"debug", slog.LevelDebug},
		{"DEBUG", slog.LevelDebug},
		{"info", slog.LevelInfo},
		{"INFO", slog.LevelInfo},
		{"", slog.LevelInfo}, // Default to info.
		{"warn", slog.LevelWarn},
		{"warning", slog.LevelWarn},
		{"WARN", slog.LevelWarn},
		{"error", slog.LevelError},
		{"ERROR", slog.LevelError},
		{"invalid", slog.LevelInfo},  // Unknown levels default to info.
		{"  info  ", slog.LevelInfo}, // Whitespace trimmed.
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := parseLogLevel(tt.input)
			if got != tt.expected {
				t.Errorf("parseLogLevel(%q) = %v, want %v", tt.input, got, tt.expected)
			}
		})
	}
}

func TestInitLogging(t *testing.T) {
	tests := []struct {
		name      string
		cfg       config.LoggingConfig
		wantErr   bool
		checkFunc func(t *testing.T)
	}{
		{
			name: "text handler with info level",
			cfg: config.LoggingConfig{
				Level: "info",
				JSON:  false,
			},
			wantErr: false,
		},
		{
			name: "json handler with debug level",
			cfg: config.LoggingConfig{
				Level: "debug",
				JSON:  true,
			},
			wantErr: false,
		},
		{
			name: "with static fields",
			cfg: config.LoggingConfig{
				Level: "info",
				JSON:  false,
				StaticFields: map[string]string{
					"service": "ployd",
					"version": "test",
				},
			},
			wantErr: false,
		},
		{
			name: "with log file",
			cfg: config.LoggingConfig{
				Level: "info",
				// Note: actual file path is set in test below using t.TempDir()
				File: "", // Will be set in test run
				JSON: false,
			},
			wantErr:   false,
			checkFunc: nil, // File creation is tested separately
		},
		{
			name: "invalid log file path",
			cfg: config.LoggingConfig{
				Level: "info",
				File:  "/nonexistent/directory/test.log",
				JSON:  false,
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := initLogging(tt.cfg)
			if (err != nil) != tt.wantErr {
				t.Errorf("initLogging() error = %v, wantErr %v", err, tt.wantErr)
			}
			if tt.checkFunc != nil && err == nil {
				tt.checkFunc(t)
			}
		})
	}
}

func TestInitLogging_FileCreation(t *testing.T) {
	tmpDir := t.TempDir()
	logPath := filepath.Join(tmpDir, "ployd.log")

	cfg := config.LoggingConfig{
		Level: "debug",
		File:  logPath,
		JSON:  true,
	}

	if err := initLogging(cfg); err != nil {
		t.Fatalf("initLogging() failed: %v", err)
	}

	// Verify file exists.
	if _, err := os.Stat(logPath); err != nil {
		t.Errorf("log file not created at %s: %v", logPath, err)
	}

	// Write a test log message.
	slog.Info("test message", "key", "value")

	// Read the file and verify JSON format.
	data, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatalf("failed to read log file: %v", err)
	}

	content := string(data)
	if !strings.Contains(content, "test message") {
		t.Errorf("log file does not contain expected message, got: %s", content)
	}

	// Verify JSON format (should contain quotes and braces).
	if !strings.Contains(content, "{") || !strings.Contains(content, "}") {
		t.Errorf("log file does not appear to be JSON formatted, got: %s", content)
	}
}
