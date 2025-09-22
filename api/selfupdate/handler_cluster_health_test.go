package selfupdate

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestValidateClusterHealthNomadFailure(t *testing.T) {
	originalNomad := checkNomadCluster
	originalTraefik := checkTraefikCluster
	t.Cleanup(func() {
		checkNomadCluster = originalNomad
		checkTraefikCluster = originalTraefik
	})

	checkNomadCluster = func() error { return errors.New("nomad unreachable") }
	checkTraefikCluster = func() error { return nil }

	h := &Handler{statusCache: make(map[string]*UpdateStatus)}
	err := h.validateClusterHealth()
	require.Error(t, err)
	require.Contains(t, err.Error(), "nomad unreachable")
}

func TestValidateClusterHealthTraefikFailure(t *testing.T) {
	originalNomad := checkNomadCluster
	originalTraefik := checkTraefikCluster
	t.Cleanup(func() {
		checkNomadCluster = originalNomad
		checkTraefikCluster = originalTraefik
	})

	checkNomadCluster = func() error { return nil }
	checkTraefikCluster = func() error { return errors.New("traefik unhealthy") }

	h := &Handler{statusCache: make(map[string]*UpdateStatus)}
	err := h.validateClusterHealth()
	require.Error(t, err)
	require.Contains(t, err.Error(), "traefik unhealthy")
}

func TestValidateClusterHealthSuccess(t *testing.T) {
	originalNomad := checkNomadCluster
	originalTraefik := checkTraefikCluster
	t.Cleanup(func() {
		checkNomadCluster = originalNomad
		checkTraefikCluster = originalTraefik
	})

	checkNomadCluster = func() error { return nil }
	checkTraefikCluster = func() error { return nil }

	h := &Handler{statusCache: make(map[string]*UpdateStatus)}
	require.NoError(t, h.validateClusterHealth())
}
