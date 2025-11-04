package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

type rolloutState struct {
	Version      int                          `json:"version"`
	RetryPolicy  rolloutRetryPolicy           `json:"retry_policy"`
	Nodes        map[string]nodeRolloutStatus `json:"nodes"`
	CreatedAt    string                       `json:"created_at"`
	LastModified string                       `json:"last_modified"`
}

type rolloutRetryPolicy struct {
	MaxAttempts int `json:"max_attempts"`
}

type nodeRolloutStatus struct {
	NodeID      string `json:"node_id"`
	NodeName    string `json:"node_name"`
	InProgress  bool   `json:"in_progress"`
	Completed   bool   `json:"completed"`
	Error       string `json:"error,omitempty"`
	Attempts    int    `json:"attempts"`
	LastAttempt string `json:"last_attempt,omitempty"`
}

func rolloutStateDir() (string, error) {
	base := strings.TrimSpace(os.Getenv("PLOY_CONFIG_HOME"))
	if base == "" {
		xdg := strings.TrimSpace(os.Getenv("XDG_CONFIG_HOME"))
		if xdg != "" {
			base = filepath.Join(xdg, "ploy")
		} else {
			home, err := os.UserHomeDir()
			if err != nil {
				return "", fmt.Errorf("find home: %w", err)
			}
			base = filepath.Join(home, ".config", "ploy")
		}
	}
	return filepath.Join(base, "rollout"), nil
}

func loadRolloutState(path string) (*rolloutState, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, err
		}
		return nil, fmt.Errorf("read state: %w", err)
	}

	var state rolloutState
	if err := json.Unmarshal(data, &state); err != nil {
		return nil, fmt.Errorf("parse state: %w", err)
	}

	if state.Nodes == nil {
		state.Nodes = make(map[string]nodeRolloutStatus)
	}

	// Validate state version for integrity.
	if state.Version != 1 {
		return nil, fmt.Errorf("unsupported state version: %d", state.Version)
	}

	return &state, nil
}

func saveRolloutState(path string, state *rolloutState) error {
	state.LastModified = time.Now().UTC().Format(time.RFC3339)
	data, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal state: %w", err)
	}

	return os.WriteFile(path, data, 0o644)
}
