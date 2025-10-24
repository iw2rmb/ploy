package bootstrap_test

import (
	"bytes"
	"context"
	"io"
	"strings"
	"testing"

	"github.com/iw2rmb/ploy/internal/ployd/bootstrap"
	"github.com/iw2rmb/ploy/internal/ployd/config"
)

func TestRunnerEmitsScript(t *testing.T) {
	t.Helper()

	var buf bytes.Buffer
	runner := bootstrap.NewRunner(bootstrap.Options{
		Shell:  "cat",
		Stdout: &buf,
		Stderr: io.Discard,
	})

	cfg := config.Config{
		HTTP: config.HTTPConfig{
			Listen: ":9443",
		},
		Metrics: config.MetricsConfig{
			Listen: ":9200",
		},
		ControlPlane: config.ControlPlaneConfig{
			Endpoint: "https://control.example.com",
		},
	}

	if err := runner.Run(context.Background(), cfg); err != nil {
		t.Fatalf("runner.Run returned error: %v", err)
	}

	output := buf.String()
	if output == "" {
		t.Fatalf("expected script output, got empty string")
	}
	if !containsAll(output, []string{
		`export PLOY_CONTROL_PLANE_ENDPOINT="https://control.example.com"`,
		`export PLOYD_HTTP_LISTEN=":9443"`,
		`export PLOYD_METRICS_LISTEN=":9200"`,
		"configure_ployd_service",
	}) {
		t.Fatalf("runner output missing expected fragments:\n%s", output)
	}
}

func containsAll(haystack string, needles []string) bool {
	for _, needle := range needles {
		if !strings.Contains(haystack, needle) {
			return false
		}
	}
	return true
}
