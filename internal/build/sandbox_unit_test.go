package build

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestNewSandboxService(t *testing.T) {
	s := NewSandboxService()
	require.NotNil(t, s)
}
