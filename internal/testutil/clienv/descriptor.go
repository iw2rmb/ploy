package clienv

import (
	"testing"
)

func UseControlPlaneEnv(t testing.TB, baseURL string) {
	t.Helper()
	t.Setenv("PLOY_SERVER_URL", baseURL)
	t.Setenv("PLOY_AUTH_TOKEN", "")
}

func UseControlPlaneEnvWithToken(t testing.TB, baseURL, token string) {
	t.Helper()
	UseControlPlaneEnv(t, baseURL)
	t.Setenv("PLOY_AUTH_TOKEN", token)
}
