package server

import (
	"bytes"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gofiber/fiber/v2"
)

func TestCertificateHandlers_Unavailable(t *testing.T) {
	s := createMockServer()
	app := fiber.New()
	// Register routes directly to test handler behavior when CertificateManager is nil
	app.Get("/v1/apps/:app/certificates", s.handleListAppCertificates)
	app.Get("/v1/apps/:app/certificates/:domain", s.handleGetDomainCertificate)
	app.Post("/v1/apps/:app/certificates/:domain/provision", s.handleProvisionCertificate)
	app.Post("/v1/apps/:app/certificates/:domain/upload", s.handleUploadCertificate)
	app.Delete("/v1/apps/:app/certificates/:domain", s.handleRemoveCertificate)

	// List certificates should return 503 when manager is nil
	resp1, err := app.Test(httptest.NewRequest(http.MethodGet, "/v1/apps/app/certificates", nil))
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer func() { _ = resp1.Body.Close() }()
	if resp1.StatusCode != http.StatusServiceUnavailable {
		t.Fatalf("expected 503, got %d", resp1.StatusCode)
	}

	// Get domain certificate
	resp2, err := app.Test(httptest.NewRequest(http.MethodGet, "/v1/apps/app/certificates/demo.example.com", nil))
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer func() { _ = resp2.Body.Close() }()
	if resp2.StatusCode != http.StatusServiceUnavailable {
		t.Fatalf("expected 503, got %d", resp2.StatusCode)
	}

	// Provision certificate
	resp3, err := app.Test(httptest.NewRequest(http.MethodPost, "/v1/apps/app/certificates/demo.example.com/provision", nil))
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer func() { _ = resp3.Body.Close() }()
	if resp3.StatusCode != http.StatusServiceUnavailable {
		t.Fatalf("expected 503, got %d", resp3.StatusCode)
	}

	// Upload certificate (multipart)
	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	_ = mw.WriteField("certificate", "cert-data")
	_ = mw.WriteField("private_key", "key-data")
	_ = mw.Close()
	req := httptest.NewRequest(http.MethodPost, "/v1/apps/app/certificates/demo.example.com/upload", &buf)
	req.Header.Set("Content-Type", mw.FormDataContentType())
	resp4, err := app.Test(req)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer func() { _ = resp4.Body.Close() }()
	if resp4.StatusCode != http.StatusServiceUnavailable {
		t.Fatalf("expected 503, got %d", resp4.StatusCode)
	}

	// Remove certificate
	resp5, err := app.Test(httptest.NewRequest(http.MethodDelete, "/v1/apps/app/certificates/demo.example.com", nil))
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer func() { _ = resp5.Body.Close() }()
	if resp5.StatusCode != http.StatusServiceUnavailable {
		t.Fatalf("expected 503, got %d", resp5.StatusCode)
	}
}
