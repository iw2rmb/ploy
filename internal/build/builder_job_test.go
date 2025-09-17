package build

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestGetJobLogsSnippet_EmptyJob(t *testing.T) {
	// When job is empty, function should return empty string without invoking external tools
	out := getJobLogsSnippet("", 100)
	require.Equal(t, "", out)
}

func TestGetJobLogsSnippet_NoManagerBinary(t *testing.T) {
	// When manager binary is not present, function should handle errors and return a string
	_ = getJobLogsSnippet("some-job", 20)
}
