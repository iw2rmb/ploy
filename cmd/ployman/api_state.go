package main

import (
	"encoding/json"
	"os"
	"time"
)

// DeploymentState tracks active deployment information
type DeploymentState struct {
	StartTime      time.Time `json:"start_time"`
	PID            int       `json:"pid"`
	TargetBranch   string    `json:"target_branch"`
	TargetHost     string    `json:"target_host"`
	Timeout        int       `json:"timeout_minutes"`
	ExpectedCommit string    `json:"expected_commit"`
	LogFile        string    `json:"log_file"`
}

func getDeploymentStateFile() string {
	return "/tmp/ploy-deployment-state.json"
}

func saveDeploymentState(state *DeploymentState) error {
	data, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(getDeploymentStateFile(), data, 0644)
}

func loadDeploymentState() (*DeploymentState, error) {
	data, err := os.ReadFile(getDeploymentStateFile())
	if err != nil {
		return nil, err
	}

	var state DeploymentState
	err = json.Unmarshal(data, &state)
	return &state, err
}

func clearDeploymentState() error {
	return os.Remove(getDeploymentStateFile())
}
