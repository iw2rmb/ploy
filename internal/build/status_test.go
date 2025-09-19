package build

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestMapNomadStatusToAppState(t *testing.T) {
	tests := []struct {
		name        string
		nomadStatus string
		expected    string
	}{
		{
			name:        "pending to building",
			nomadStatus: "pending",
			expected:    "building",
		},
		{
			name:        "running to running",
			nomadStatus: "running",
			expected:    "running",
		},
		{
			name:        "dead to failed",
			nomadStatus: "dead",
			expected:    "failed",
		},
		{
			name:        "complete treated as running",
			nomadStatus: "complete",
			expected:    "running",
		},
		{
			name:        "case insensitive handling",
			nomadStatus: "PeNdInG",
			expected:    "building",
		},
		{
			name:        "trim whitespace",
			nomadStatus: "  running  ",
			expected:    "running",
		},
		{
			name:        "unknown maps to unknown",
			nomadStatus: "mystery",
			expected:    "unknown",
		},
		{
			name:        "empty maps to unknown",
			nomadStatus: "",
			expected:    "unknown",
		},
		{
			name:        "whitespace maps to unknown",
			nomadStatus: "   ",
			expected:    "unknown",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.expected, mapNomadStatusToMod(tt.nomadStatus))
		})
	}
}

func TestMapNomadStatusToAppStateDeterministic(t *testing.T) {
	result1 := mapNomadStatusToMod("pending")
	result2 := mapNomadStatusToMod("pending")
	assert.Equal(t, result1, result2)
}

func TestMapNomadStatusToAppStateUnknowns(t *testing.T) {
	inputs := []string{"unknown", "invalid", "corrupted", "starting"}
	for _, status := range inputs {
		assert.Equal(t, "unknown", mapNomadStatusToMod(status))
	}
}

func TestExtractLaneFromJobName(t *testing.T) {
	tests := []struct {
		name         string
		jobName      string
		expectedLane string
	}{
		{
			name:         "valid lane lower case",
			jobName:      "myapp-lane-c",
			expectedLane: "C",
		},
		{
			name:         "valid lane already upper",
			jobName:      "myapp-lane-A",
			expectedLane: "A",
		},
		{
			name:         "missing lane information",
			jobName:      "myapp",
			expectedLane: "unknown",
		},
		{
			name:         "separator but empty lane",
			jobName:      "myapp-lane-",
			expectedLane: "",
		},
		{
			name:         "multiple separators",
			jobName:      "app-lane-c-lane-d",
			expectedLane: "unknown",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.expectedLane, extractLaneFromJobName(tt.jobName))
		})
	}
}
