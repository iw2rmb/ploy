package build

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestMapNomadStatusToARF(t *testing.T) {
	tests := []struct {
		name         string
		nomadStatus  string
		expectedARF  string
	}{
		{
			name:        "pending to building",
			nomadStatus: "pending",
			expectedARF: "building",
		},
		{
			name:        "running to deploying",
			nomadStatus: "running",
			expectedARF: "deploying",
		},
		{
			name:        "dead to stopped",
			nomadStatus: "dead",
			expectedARF: "stopped",
		},
		{
			name:        "case insensitive pending",
			nomadStatus: "PENDING",
			expectedARF: "building",
		},
		{
			name:        "case insensitive running",
			nomadStatus: "RUNNING",
			expectedARF: "deploying",
		},
		{
			name:        "case insensitive dead",
			nomadStatus: "DEAD",
			expectedARF: "stopped",
		},
		{
			name:        "unknown status defaults to running",
			nomadStatus: "unknown",
			expectedARF: "running",
		},
		{
			name:        "empty status defaults to running",
			nomadStatus: "",
			expectedARF: "running",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := mapNomadStatusToARF(tt.nomadStatus)
			assert.Equal(t, tt.expectedARF, result)
		})
	}
}

func TestExtractLaneFromJobName(t *testing.T) {
	tests := []struct {
		name        string
		jobName     string
		expectedLane string
	}{
		{
			name:        "valid lane A",
			jobName:     "myapp-lane-a",
			expectedLane: "A",
		},
		{
			name:        "valid lane B",
			jobName:     "myapp-lane-b",
			expectedLane: "B",
		},
		{
			name:        "valid lane C",
			jobName:     "hello-world-lane-c",
			expectedLane: "C",
		},
		{
			name:        "valid lane with complex app name",
			jobName:     "my-complex-app-name-lane-d",
			expectedLane: "D",
		},
		{
			name:        "invalid format - no lane",
			jobName:     "myapp",
			expectedLane: "unknown",
		},
		{
			name:        "invalid format - wrong separator",
			jobName:     "myapp_lane_a",
			expectedLane: "unknown",
		},
		{
			name:        "empty job name",
			jobName:     "",
			expectedLane: "unknown",
		},
		{
			name:        "lane with special characters",
			jobName:     "app-with-dashes-lane-e",
			expectedLane: "E",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := extractLaneFromJobName(tt.jobName)
			assert.Equal(t, tt.expectedLane, result)
		})
	}
}

// Test Status function
func TestStatus(t *testing.T) {
	t.Skip("Integration test - requires Nomad API mocking")
	// This test would require mocking the Nomad monitor
	// In unit tests, we focus on the helper functions
}