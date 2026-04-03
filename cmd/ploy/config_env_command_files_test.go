package main

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/iw2rmb/ploy/internal/testutil/clienv"
	"github.com/iw2rmb/ploy/internal/testutil/stdcapture"
)

// TestHandleConfigEnvListSuccess verifies that the 'list' subcommand retrieves
// and displays global env variables from the server using target wire format.
func TestHandleConfigEnvListSuccess(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "GET" {
			t.Fatalf("expected GET, got %s", r.Method)
		}
		if r.URL.Path != "/v1/config/env" {
			t.Fatalf("expected /v1/config/env, got %s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode([]map[string]any{
			{"key": "PLOY_CA_CERTS", "target": "gates", "secret": true},
			{"key": "PLOY_CA_CERTS", "target": "steps", "secret": true},
			{"key": "OPENAI_API_KEY", "target": "steps", "secret": true},
			{"key": "DEBUG_MODE", "value": "true", "target": "gates", "secret": false},
		})
	}))
	defer srv.Close()

	clienv.UseServerDescriptor(t, srv.URL)

	buf := &bytes.Buffer{}
	var err error
	output := stdcapture.CaptureStdout(t, func() {
		err = handleConfigEnvList(nil, buf)
	})

	if err != nil {
		t.Fatalf("handleConfigEnvList error: %v", err)
	}

	// Verify header uses TARGET column.
	if !strings.Contains(output, "TARGET") {
		t.Fatalf("expected TARGET header in output, got: %q", output)
	}
	if !strings.Contains(output, "PLOY_CA_CERTS") {
		t.Fatalf("expected PLOY_CA_CERTS in output, got: %q", output)
	}
	if !strings.Contains(output, "OPENAI_API_KEY") {
		t.Fatalf("expected OPENAI_API_KEY in output, got: %q", output)
	}
	// Secret values should be shown as (redacted).
	if !strings.Contains(output, "(redacted)") {
		t.Fatalf("expected (redacted) for secret values, got: %q", output)
	}
	// Non-secret value should be displayed.
	if !strings.Contains(output, "true") {
		t.Fatalf("expected 'true' value for DEBUG_MODE, got: %q", output)
	}
}

// TestHandleConfigEnvListEmpty verifies that an empty list displays an appropriate message.
func TestHandleConfigEnvListEmpty(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "GET" || r.URL.Path != "/v1/config/env" {
			t.Fatalf("unexpected request %s %s", r.Method, r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode([]map[string]any{})
	}))
	defer srv.Close()

	clienv.UseServerDescriptor(t, srv.URL)

	buf := &bytes.Buffer{}
	var err error
	output := stdcapture.CaptureStdout(t, func() {
		err = handleConfigEnvList(nil, buf)
	})

	if err != nil {
		t.Fatalf("handleConfigEnvList error: %v", err)
	}

	if !strings.Contains(output, "No global environment variables configured") {
		t.Fatalf("expected empty message, got: %q", output)
	}
}

// TestHandleConfigEnvShowSuccess verifies that 'show' retrieves a single env variable
// using the target wire format.
func TestHandleConfigEnvShowSuccess(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "GET" {
			t.Fatalf("expected GET, got %s", r.Method)
		}
		if r.URL.Path != "/v1/config/env/PLOY_CA_CERTS" {
			t.Fatalf("expected /v1/config/env/PLOY_CA_CERTS, got %s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"key":    "PLOY_CA_CERTS",
			"value":  "-----BEGIN CERTIFICATE-----\nMIID...\n-----END CERTIFICATE-----",
			"target": "gates",
			"secret": true,
		})
	}))
	defer srv.Close()

	clienv.UseServerDescriptor(t, srv.URL)

	buf := &bytes.Buffer{}
	var err error
	output := stdcapture.CaptureStdout(t, func() {
		err = handleConfigEnvShow([]string{"--key", "PLOY_CA_CERTS"}, buf)
	})

	if err != nil {
		t.Fatalf("handleConfigEnvShow error: %v", err)
	}

	if !strings.Contains(output, "Key:    PLOY_CA_CERTS") {
		t.Fatalf("expected key in output, got: %q", output)
	}
	if !strings.Contains(output, "Target: gates") {
		t.Fatalf("expected target in output, got: %q", output)
	}
	if !strings.Contains(output, "Secret: true") {
		t.Fatalf("expected secret flag in output, got: %q", output)
	}
	// Value should be redacted for secrets by default.
	if strings.Contains(output, "-----BEGIN CERTIFICATE-----") {
		t.Fatalf("secret value should be redacted without --raw, got: %q", output)
	}
}

// TestHandleConfigEnvShowWithFrom verifies that --from sends the target query parameter.
func TestHandleConfigEnvShowWithFrom(t *testing.T) {
	var gotQuery string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotQuery = r.URL.Query().Get("target")
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"key":    "PLOY_CA_CERTS",
			"value":  "pem-content",
			"target": "gates",
			"secret": false,
		})
	}))
	defer srv.Close()

	clienv.UseServerDescriptor(t, srv.URL)

	buf := &bytes.Buffer{}
	var err error
	stdcapture.CaptureStdout(t, func() {
		err = handleConfigEnvShow([]string{"--key", "PLOY_CA_CERTS", "--from", "gates"}, buf)
	})

	if err != nil {
		t.Fatalf("handleConfigEnvShow error: %v", err)
	}
	if gotQuery != "gates" {
		t.Fatalf("expected target query param 'gates', got %q", gotQuery)
	}
}

// TestHandleConfigEnvShowAmbiguity verifies that 409 responses produce a --from hint.
func TestHandleConfigEnvShowAmbiguity(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "ambiguous key: exists for targets gates, steps", http.StatusConflict)
	}))
	defer srv.Close()

	clienv.UseServerDescriptor(t, srv.URL)

	buf := &bytes.Buffer{}
	err := handleConfigEnvShow([]string{"--key", "PLOY_CA_CERTS"}, buf)
	if err == nil {
		t.Fatalf("expected error for ambiguous key")
	}
	if !strings.Contains(err.Error(), "use --from") {
		t.Fatalf("expected --from hint in error, got: %v", err)
	}
}

// TestHandleConfigEnvShowRaw verifies that --raw displays the full secret value.
func TestHandleConfigEnvShowRaw(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/config/env/SECRET_KEY" {
			t.Fatalf("expected /v1/config/env/SECRET_KEY, got %s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"key":    "SECRET_KEY",
			"value":  "super-secret-value-12345",
			"target": "steps",
			"secret": true,
		})
	}))
	defer srv.Close()

	clienv.UseServerDescriptor(t, srv.URL)

	buf := &bytes.Buffer{}
	var err error
	output := stdcapture.CaptureStdout(t, func() {
		err = handleConfigEnvShow([]string{"--key", "SECRET_KEY", "--raw"}, buf)
	})

	if err != nil {
		t.Fatalf("handleConfigEnvShow error: %v", err)
	}

	// With --raw, the full value should be shown.
	if !strings.Contains(output, "super-secret-value-12345") {
		t.Fatalf("expected full value with --raw, got: %q", output)
	}
}

// TestHandleConfigEnvShowNotFound verifies that missing keys return a clear error.
func TestHandleConfigEnvShowNotFound(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "global env key not found: MISSING_KEY", http.StatusNotFound)
	}))
	defer srv.Close()

	clienv.UseServerDescriptor(t, srv.URL)

	buf := &bytes.Buffer{}
	err := handleConfigEnvShow([]string{"--key", "MISSING_KEY"}, buf)
	if err == nil {
		t.Fatalf("expected error for missing key")
	}
	if !strings.Contains(err.Error(), "not found") {
		t.Fatalf("unexpected error: %v", err)
	}
}

// TestHandleConfigEnvSetSuccessInline verifies setting a value with --value.
// Default --on is jobs, which expands to gates+steps (two PUTs).
func TestHandleConfigEnvSetSuccessInline(t *testing.T) {
	var mu sync.Mutex
	var gotRequests []struct {
		Method      string
		Path        string
		ContentType string
		Body        map[string]any
	}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body map[string]any
		_ = json.NewDecoder(r.Body).Decode(&body)
		mu.Lock()
		gotRequests = append(gotRequests, struct {
			Method      string
			Path        string
			ContentType string
			Body        map[string]any
		}{r.Method, r.URL.Path, r.Header.Get("Content-Type"), body})
		mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"key":    "OPENAI_API_KEY",
			"value":  "sk-test-12345",
			"target": body["target"],
			"secret": true,
		})
	}))
	defer srv.Close()

	clienv.UseServerDescriptor(t, srv.URL)

	buf := &bytes.Buffer{}
	var err error
	output := stdcapture.CaptureStdout(t, func() {
		err = handleConfigEnvSet([]string{"--key", "OPENAI_API_KEY", "--value", "sk-test-12345"}, buf)
	})

	if err != nil {
		t.Fatalf("handleConfigEnvSet error: %v", err)
	}

	// Default --on=jobs → gates, steps (2 PUTs).
	if len(gotRequests) != 2 {
		t.Fatalf("expected 2 PUT requests (jobs=gates+steps), got %d", len(gotRequests))
	}

	// Verify targets sent.
	targets := map[string]bool{}
	for _, req := range gotRequests {
		if req.Method != "PUT" {
			t.Fatalf("expected PUT, got %s", req.Method)
		}
		if req.Path != "/v1/config/env/OPENAI_API_KEY" {
			t.Fatalf("expected /v1/config/env/OPENAI_API_KEY, got %s", req.Path)
		}
		if req.ContentType != "application/json" {
			t.Fatalf("expected application/json, got %s", req.ContentType)
		}
		if req.Body["value"] != "sk-test-12345" {
			t.Fatalf("expected value 'sk-test-12345', got: %v", req.Body["value"])
		}
		if req.Body["secret"] != true {
			t.Fatalf("expected secret true (default), got: %v", req.Body["secret"])
		}
		targets[req.Body["target"].(string)] = true
	}
	if !targets["gates"] || !targets["steps"] {
		t.Fatalf("expected targets gates and steps, got: %v", targets)
	}

	if !strings.Contains(output, "set for targets") {
		t.Fatalf("expected success message, got: %q", output)
	}
}

// TestHandleConfigEnvSetSingleTarget verifies setting with --on gates (single target, one PUT).
func TestHandleConfigEnvSetSingleTarget(t *testing.T) {
	var gotBody map[string]any

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewDecoder(r.Body).Decode(&gotBody)
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"key":    "FOO",
			"value":  "bar",
			"target": "gates",
			"secret": true,
		})
	}))
	defer srv.Close()

	clienv.UseServerDescriptor(t, srv.URL)

	buf := &bytes.Buffer{}
	var err error
	output := stdcapture.CaptureStdout(t, func() {
		err = handleConfigEnvSet([]string{"--key", "FOO", "--value", "bar", "--on", "gates"}, buf)
	})

	if err != nil {
		t.Fatalf("handleConfigEnvSet error: %v", err)
	}
	if gotBody["target"] != "gates" {
		t.Fatalf("expected target 'gates', got: %v", gotBody["target"])
	}
	if !strings.Contains(output, "set for target gates") {
		t.Fatalf("expected single-target success message, got: %q", output)
	}
}

// TestHandleConfigEnvSetSuccessFromFile verifies setting a value from a file.
func TestHandleConfigEnvSetSuccessFromFile(t *testing.T) {
	tmpDir := t.TempDir()
	filePath := filepath.Join(tmpDir, "ca-bundle.pem")
	fileContent := "-----BEGIN CERTIFICATE-----\nMIID...\n-----END CERTIFICATE-----"
	if err := os.WriteFile(filePath, []byte(fileContent), 0o600); err != nil {
		t.Fatalf("write test file: %v", err)
	}

	var mu sync.Mutex
	var gotBodies []map[string]any

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "PUT" {
			t.Fatalf("expected PUT, got %s", r.Method)
		}
		var body map[string]any
		_ = json.NewDecoder(r.Body).Decode(&body)
		mu.Lock()
		gotBodies = append(gotBodies, body)
		mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"key":    "MY_CUSTOM_CERT",
			"value":  fileContent,
			"target": body["target"],
			"secret": true,
		})
	}))
	defer srv.Close()

	clienv.UseServerDescriptor(t, srv.URL)

	buf := &bytes.Buffer{}
	var err error
	stdcapture.CaptureStdout(t, func() {
		err = handleConfigEnvSet([]string{"--key", "MY_CUSTOM_CERT", "--file", filePath, "--on", "all"}, buf)
	})

	if err != nil {
		t.Fatalf("handleConfigEnvSet error: %v", err)
	}

	// --on all → 4 PUTs (gates, nodes, server, steps).
	if len(gotBodies) != 4 {
		t.Fatalf("expected 4 PUT requests (all targets), got %d", len(gotBodies))
	}

	// Verify the file content was sent in each request.
	for _, body := range gotBodies {
		if body["value"] != fileContent {
			t.Fatalf("expected file content, got: %v", body["value"])
		}
	}
}

// TestHandleConfigEnvSetSecretFalse verifies setting --secret=false.
func TestHandleConfigEnvSetSecretFalse(t *testing.T) {
	var gotBody map[string]any

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewDecoder(r.Body).Decode(&gotBody)
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"key":    "DEBUG_MODE",
			"value":  "true",
			"target": "gates",
			"secret": false,
		})
	}))
	defer srv.Close()

	clienv.UseServerDescriptor(t, srv.URL)

	buf := &bytes.Buffer{}
	var err error
	stdcapture.CaptureStdout(t, func() {
		err = handleConfigEnvSet([]string{
			"--key", "DEBUG_MODE",
			"--value", "true",
			"--on", "gates",
			"--secret=false",
		}, buf)
	})

	if err != nil {
		t.Fatalf("handleConfigEnvSet error: %v", err)
	}

	if gotBody["secret"] != false {
		t.Fatalf("expected secret false, got: %v", gotBody["secret"])
	}
}

// TestHandleConfigEnvSetServerError verifies that server errors are propagated.
func TestHandleConfigEnvSetServerError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "internal error", http.StatusInternalServerError)
	}))
	defer srv.Close()

	clienv.UseServerDescriptor(t, srv.URL)

	buf := &bytes.Buffer{}
	err := handleConfigEnvSet([]string{"--key", "FOO", "--value", "bar", "--on", "gates"}, buf)
	if err == nil {
		t.Fatalf("expected error from server")
	}
	if !strings.Contains(err.Error(), "server returned 500") {
		t.Fatalf("unexpected error: %v", err)
	}
}

// TestHandleConfigEnvUnsetSuccess verifies that 'unset' deletes an env variable.
func TestHandleConfigEnvUnsetSuccess(t *testing.T) {
	var gotMethod, gotPath string

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotMethod = r.Method
		gotPath = r.URL.Path
		w.WriteHeader(http.StatusNoContent)
	}))
	defer srv.Close()

	clienv.UseServerDescriptor(t, srv.URL)

	buf := &bytes.Buffer{}
	var err error
	output := stdcapture.CaptureStdout(t, func() {
		err = handleConfigEnvUnset([]string{"--key", "PLOY_CA_CERTS"}, buf)
	})

	if err != nil {
		t.Fatalf("handleConfigEnvUnset error: %v", err)
	}

	if gotMethod != "DELETE" {
		t.Fatalf("expected DELETE, got %s", gotMethod)
	}
	if gotPath != "/v1/config/env/PLOY_CA_CERTS" {
		t.Fatalf("expected /v1/config/env/PLOY_CA_CERTS, got %s", gotPath)
	}

	if !strings.Contains(output, "deleted successfully") {
		t.Fatalf("expected success message, got: %q", output)
	}
}

// TestHandleConfigEnvUnsetWithFrom verifies that --from sends the target query parameter.
func TestHandleConfigEnvUnsetWithFrom(t *testing.T) {
	var gotQuery string

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotQuery = r.URL.Query().Get("target")
		w.WriteHeader(http.StatusNoContent)
	}))
	defer srv.Close()

	clienv.UseServerDescriptor(t, srv.URL)

	buf := &bytes.Buffer{}
	var err error
	stdcapture.CaptureStdout(t, func() {
		err = handleConfigEnvUnset([]string{"--key", "PLOY_CA_CERTS", "--from", "gates"}, buf)
	})

	if err != nil {
		t.Fatalf("handleConfigEnvUnset error: %v", err)
	}
	if gotQuery != "gates" {
		t.Fatalf("expected target query param 'gates', got %q", gotQuery)
	}
}

// TestHandleConfigEnvUnsetAmbiguity verifies that 409 responses produce a --from hint.
func TestHandleConfigEnvUnsetAmbiguity(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "ambiguous key: exists for targets gates, steps", http.StatusConflict)
	}))
	defer srv.Close()

	clienv.UseServerDescriptor(t, srv.URL)

	buf := &bytes.Buffer{}
	err := handleConfigEnvUnset([]string{"--key", "PLOY_CA_CERTS"}, buf)
	if err == nil {
		t.Fatalf("expected error for ambiguous key")
	}
	if !strings.Contains(err.Error(), "use --from") {
		t.Fatalf("expected --from hint in error, got: %v", err)
	}
}

// TestHandleConfigEnvUnsetNotFound verifies that unset handles 404 gracefully.
func TestHandleConfigEnvUnsetNotFound(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "not found", http.StatusNotFound)
	}))
	defer srv.Close()

	clienv.UseServerDescriptor(t, srv.URL)

	buf := &bytes.Buffer{}
	var err error
	output := stdcapture.CaptureStdout(t, func() {
		err = handleConfigEnvUnset([]string{"--key", "MISSING_KEY"}, buf)
	})

	// Should not error; 404 is acceptable for delete (idempotent).
	if err != nil {
		t.Fatalf("handleConfigEnvUnset error: %v", err)
	}

	if !strings.Contains(output, "not found") {
		t.Fatalf("expected 'not found' message, got: %q", output)
	}
}

// TestHandleConfigEnvUnsetServerError verifies that server errors are propagated.
func TestHandleConfigEnvUnsetServerError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "internal error", http.StatusInternalServerError)
	}))
	defer srv.Close()

	clienv.UseServerDescriptor(t, srv.URL)

	buf := &bytes.Buffer{}
	err := handleConfigEnvUnset([]string{"--key", "FOO"}, buf)
	if err == nil {
		t.Fatalf("expected error from server")
	}
	if !strings.Contains(err.Error(), "server returned 500") {
		t.Fatalf("unexpected error: %v", err)
	}
}

// TestHandleConfigEnvListServerError verifies that server errors in list are propagated.
func TestHandleConfigEnvListServerError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
	}))
	defer srv.Close()

	clienv.UseServerDescriptor(t, srv.URL)

	buf := &bytes.Buffer{}
	err := handleConfigEnvList(nil, buf)
	if err == nil {
		t.Fatalf("expected error from server")
	}
	if !strings.Contains(err.Error(), "server returned 401") {
		t.Fatalf("unexpected error: %v", err)
	}
}

// TestHandleConfigEnvShowServerError verifies that server errors in show are propagated.
func TestHandleConfigEnvShowServerError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "forbidden", http.StatusForbidden)
	}))
	defer srv.Close()

	clienv.UseServerDescriptor(t, srv.URL)

	buf := &bytes.Buffer{}
	err := handleConfigEnvShow([]string{"--key", "FOO"}, buf)
	if err == nil {
		t.Fatalf("expected error from server")
	}
	if !strings.Contains(err.Error(), "server returned 403") {
		t.Fatalf("unexpected error: %v", err)
	}
}

// TestHandleConfigEnvShowRedactsShortSecrets ensures short secrets are fully redacted.
func TestHandleConfigEnvShowRedactsShortSecrets(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"key":    "SHORT",
			"value":  "abc",
			"target": "steps",
			"secret": true,
		})
	}))
	defer srv.Close()

	clienv.UseServerDescriptor(t, srv.URL)

	buf := &bytes.Buffer{}
	var err error
	output := stdcapture.CaptureStdout(t, func() {
		err = handleConfigEnvShow([]string{"--key", "SHORT"}, buf)
	})

	if err != nil {
		t.Fatalf("handleConfigEnvShow error: %v", err)
	}

	// Short secrets should show *** instead of partial value.
	if strings.Contains(output, "abc") {
		t.Fatalf("short secret should be fully redacted, got: %q", output)
	}
	if !strings.Contains(output, "***") {
		t.Fatalf("expected *** for short secret, got: %q", output)
	}
}
