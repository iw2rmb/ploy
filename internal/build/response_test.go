package build

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/iw2rmb/ploy/internal/config"
	"github.com/iw2rmb/ploy/internal/security"
	"github.com/iw2rmb/ploy/internal/utils"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMakeBuildResponse_BuilderFieldsLaneD(t *testing.T) {
	start := time.Now().Add(-2 * time.Second)
	resp := makeBuildResponse(
		"D",
		"/opt/ploy/artifacts/app.img",
		"registry.dev/app:sha",
		"apps",
		config.UserApp,
		start,
		nil,
		42.5,
		"app-e-build-123",
		"app",
		"123",
		"registry.dev",
		"project",
		nil,
		nil,
	)
	require.Contains(t, resp, "builder")
	b := resp["builder"].(fiber.Map)
	require.Equal(t, "app-e-build-123", b["job"])
	// imageSize always present
	require.Contains(t, resp, "imageSize")
}

func TestMakeBuildResponse_PushVerificationAndSecurity(t *testing.T) {
	// Set up TLS server to act as registry endpoint for verifyOCIPush
	ts := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Accept HEAD/GET for manifest endpoint and return a digest
		w.Header().Set("Docker-Content-Digest", "sha256:abcd")
		w.WriteHeader(200)
	}))
	defer ts.Close()
	// Make the verify function use this TLS client
	origClient := registryHTTPClient
	registryHTTPClient = ts.Client()
	t.Cleanup(func() { registryHTTPClient = origClient })

	host := strings.TrimPrefix(ts.URL, "https://")
	dockerTag := host + "/repo:tag"

	start := time.Now().Add(-100 * time.Millisecond)
	scan := &security.ScanResult{Passed: true, VulnCount: 1, CriticalCount: 0, HighCount: 0}
	scanner := security.NewVulnerabilityScanner()
	resp := makeBuildResponse(
		"D",
		"",
		dockerTag,
		"apps",
		config.UserApp,
		start,
		&utils.ImageSizeInfo{SizeBytes: 1234, SizeMB: 1.2},
		1.2,
		"builder-job",
		"app",
		"sha",
		host,
		"project",
		scan,
		scanner,
	)

	// pushVerification populated
	pv, ok := resp["pushVerification"].(fiber.Map)
	require.True(t, ok)
	assert.Equal(t, true, pv["ok"])
	assert.Equal(t, "sha256:abcd", pv["digest"])

	// registry info present
	reg, ok := resp["registry"].(fiber.Map)
	require.True(t, ok)
	assert.Equal(t, host, reg["endpoint"])
	assert.Equal(t, dockerTag, reg["imageTag"])

	// builder metadata preserved for lane D
	builder, ok := resp["builder"].(fiber.Map)
	require.True(t, ok)
	assert.Equal(t, "builder-job", builder["job"])

	// security summary present
	sec, ok := resp["security"].(fiber.Map)
	require.True(t, ok)
	assert.Equal(t, true, sec["vulnScanPassed"])
}

func TestMakeBuildResponse_NoSizeInfo_BytesZero(t *testing.T) {
	start := time.Now().Add(-50 * time.Millisecond)
	resp := makeBuildResponse(
		"D",
		"/opt/ploy/artifacts/app.img",
		"", // no docker image -> no verify call
		"apps",
		config.UserApp,
		start,
		nil, // sizeInfo nil
		0,
		"d-job-1",
		"app",
		"sha",
		"",
		"",
		nil,
		nil,
	)

	sz, ok := resp["imageSize"].(fiber.Map)
	require.True(t, ok)
	// When sizeInfo is nil, bytes should be 0
	require.Equal(t, int64(0), sz["bytes"])

	// Builder info present for lane D when builderJobName set
	b, ok := resp["builder"].(fiber.Map)
	require.True(t, ok)
	require.Equal(t, "d-job-1", b["job"])
	require.Equal(t, "build-logs/d-job-1.log", b["logs_key"])
	require.Equal(t, "http://seaweedfs-filer.storage.ploy.local:8888/artifacts/build-logs/d-job-1.log", b["logs_url"])
	require.Equal(t, "/opt/ploy/build-logs/d-job-1.log", b["log_path"])

	// No pushVerification since dockerImage is empty
	_, hasPV := resp["pushVerification"]
	assert.False(t, hasPV)
}

func TestMakeBuildResponse_LaneD_NoBuilderJob_OmitsBuilder(t *testing.T) {
	start := time.Now().Add(-10 * time.Millisecond)
	resp := makeBuildResponse(
		"D",
		"/opt/ploy/artifacts/app.img",
		"", // no docker image
		"apps",
		config.UserApp,
		start,
		&utils.ImageSizeInfo{SizeBytes: 42, SizeMB: 0.04},
		0.04,
		"", // empty builderJobName
		"app",
		"sha",
		"",
		"",
		nil,
		nil,
	)

	// For lane E with empty builder job, the builder field should be absent
	_, exists := resp["builder"]
	assert.False(t, exists)
}
