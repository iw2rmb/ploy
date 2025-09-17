package build

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestVerifyOCIPush_InvalidTagFormat(t *testing.T) {
	vr := verifyOCIPush("invalidtag")
	require.False(t, vr.OK)
	require.Equal(t, 0, vr.Status)
	require.Contains(t, vr.Message, "unverifiable")
}

func TestVerifyOCIPush_SuccessWithDigest(t *testing.T) {
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, http.MethodHead, r.Method)
		w.Header().Set("Docker-Content-Digest", "sha256:abc123")
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	oldClient := registryHTTPClient
	registryHTTPClient = server.Client()
	defer func() { registryHTTPClient = oldClient }()

	host := strings.TrimPrefix(server.URL, "https://")
	vr := verifyOCIPush(host + "/repo:latest")
	require.True(t, vr.OK)
	require.Equal(t, http.StatusOK, vr.Status)
	require.Equal(t, "sha256:abc123", vr.Digest)
	require.Equal(t, "manifest present", vr.Message)
}

func TestVerifyOCIPush_MethodFallback(t *testing.T) {
	callCount := 0
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callCount++
		if r.Method == http.MethodHead {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		require.Equal(t, http.MethodGet, r.Method)
		w.Header().Set("Docker-Content-Digest", "sha256:fallback")
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	oldClient := registryHTTPClient
	registryHTTPClient = server.Client()
	defer func() { registryHTTPClient = oldClient }()

	host := strings.TrimPrefix(server.URL, "https://")
	vr := verifyOCIPush(host + "/repo@sha256:deadbeef")
	require.True(t, vr.OK)
	require.Equal(t, http.StatusOK, vr.Status)
	require.Equal(t, "sha256:fallback", vr.Digest)
	require.Equal(t, 2, callCount, "expected HEAD then GET fallback")
}

func TestVerifyOCIPush_NotFound(t *testing.T) {
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer server.Close()

	oldClient := registryHTTPClient
	registryHTTPClient = server.Client()
	defer func() { registryHTTPClient = oldClient }()

	host := strings.TrimPrefix(server.URL, "https://")
	vr := verifyOCIPush(host + "/repo:dev")
	require.False(t, vr.OK)
	require.Equal(t, http.StatusNotFound, vr.Status)
	require.Contains(t, vr.Message, "manifest unknown")
}

func TestVerifyOCIPush_Unauthorized(t *testing.T) {
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
	}))
	defer server.Close()

	oldClient := registryHTTPClient
	registryHTTPClient = server.Client()
	defer func() { registryHTTPClient = oldClient }()

	host := strings.TrimPrefix(server.URL, "https://")
	vr := verifyOCIPush(host + "/repo:prod")
	require.False(t, vr.OK)
	require.Equal(t, http.StatusForbidden, vr.Status)
	require.Contains(t, vr.Message, "unauthorized")
}
