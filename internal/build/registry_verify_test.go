package build

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestVerifyOCIPush_InvalidTagFormat(t *testing.T) {
	vr := verifyOCIPush("invalidtag")
	require.False(t, vr.OK)
	require.Equal(t, 0, vr.Status)
	require.Contains(t, vr.Message, "unverifiable")
}
