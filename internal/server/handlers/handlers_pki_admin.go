package handlers

import (
	"crypto/x509"
	"encoding/asn1"
	"encoding/json"
	"encoding/pem"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"time"

	internalPKI "github.com/iw2rmb/ploy/internal/pki"
)

// pkiSignAdminHandler returns an HTTP handler that signs admin CSRs with the cluster CA.
// It requires cli-admin role authorization (enforced by middleware) and validates that
// the CSR has the correct OU and ExtKeyUsage for client authentication.
func pkiSignAdminHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// Decode request body.
		var req struct {
			CSR string `json:"csr"`
		}

		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, fmt.Sprintf("invalid request: %v", err), http.StatusBadRequest)
			return
		}

		// Validate CSR is not empty.
		if strings.TrimSpace(req.CSR) == "" {
			http.Error(w, "csr field is required", http.StatusBadRequest)
			return
		}

		// Load cluster CA from environment or well-known file paths.
		ca, rawCACert, err := loadClusterCA()
		if err != nil {
			if errors.Is(err, errCANotConfigured) {
				http.Error(w, "PKI not configured", http.StatusServiceUnavailable)
			} else {
				http.Error(w, "failed to load CA", http.StatusInternalServerError)
			}
			slog.Error("pki sign admin: load CA failed", "err", err)
			return
		}

		// Parse and validate CSR for admin requirements.
		if err := validateAdminCSR([]byte(req.CSR)); err != nil {
			http.Error(w, fmt.Sprintf("invalid admin CSR: %v", err), http.StatusBadRequest)
			slog.Warn("pki sign admin: CSR validation failed", "err", err)
			return
		}

		// Sign the CSR using the generic node CSR signing function.
		// This reuses the existing signing logic which validates signature and issues a cert.
		cert, err := internalPKI.SignNodeCSR(ca, []byte(req.CSR), time.Now())
		if err != nil {
			http.Error(w, fmt.Sprintf("sign failed: %v", err), http.StatusBadRequest)
			slog.Warn("pki sign admin: sign CSR failed", "err", err)
			return
		}

		// Build response according to PKI schema.
		resp := struct {
			Certificate string `json:"certificate"`
			CABundle    string `json:"ca_bundle"`
			Serial      string `json:"serial"`
			Fingerprint string `json:"fingerprint"`
			NotBefore   string `json:"not_before"`
			NotAfter    string `json:"not_after"`
		}{
			Certificate: cert.CertPEM,
			CABundle:    rawCACert,
			Serial:      cert.Serial,
			Fingerprint: cert.Fingerprint,
			NotBefore:   cert.NotBefore.Format(time.RFC3339),
			NotAfter:    cert.NotAfter.Format(time.RFC3339),
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		if err := json.NewEncoder(w).Encode(resp); err != nil {
			slog.Error("pki sign admin: encode response failed", "err", err)
		}

		slog.Info("pki sign admin: certificate issued",
			"serial", cert.Serial,
			"fingerprint", cert.Fingerprint,
			"not_before", cert.NotBefore.Format(time.RFC3339),
			"not_after", cert.NotAfter.Format(time.RFC3339),
		)
	}
}

// validateAdminCSR validates that a CSR meets the requirements for an admin certificate:
// - Must have OU containing "Ploy role=cli-admin"
// - Must request ExtKeyUsage with ClientAuth
func validateAdminCSR(csrPEM []byte) error {
	block, _ := pem.Decode(csrPEM)
	if block == nil || block.Type != "CERTIFICATE REQUEST" {
		return errors.New("invalid CSR PEM: missing or wrong block type")
	}

	csr, err := x509.ParseCertificateRequest(block.Bytes)
	if err != nil {
		return fmt.Errorf("parse CSR: %w", err)
	}

	if err := csr.CheckSignature(); err != nil {
		return fmt.Errorf("verify CSR signature: %w", err)
	}

	// Validate OU contains "Ploy role=cli-admin".
	hasAdminOU := false
	for _, ou := range csr.Subject.OrganizationalUnit {
		if strings.TrimSpace(ou) == "Ploy role=cli-admin" {
			hasAdminOU = true
			break
		}
	}
	if !hasAdminOU {
		return errors.New("CSR must have OU=\"Ploy role=cli-admin\"")
	}

	// Validate ExtKeyUsage requests ClientAuth.
	// In CSRs, requested extensions are carried in the extensionRequest
	// attribute and exposed via csr.Extensions by the x509 parser. The
	// ExtKeyUsage OID is 2.5.29.37; its value is a SEQUENCE of OIDs.
	var hasClientAuthEKU bool
	for _, ext := range csr.Extensions {
		if ext.Id.Equal(asn1.ObjectIdentifier{2, 5, 29, 37}) { // extKeyUsage
			var oids []asn1.ObjectIdentifier
			if _, err := asn1.Unmarshal(ext.Value, &oids); err != nil {
				return fmt.Errorf("parse EKU extension: %w", err)
			}
			for _, oid := range oids {
				if oid.Equal(asn1.ObjectIdentifier{1, 3, 6, 1, 5, 5, 7, 3, 2}) { // clientAuth
					hasClientAuthEKU = true
					break
				}
			}
			break
		}
	}
	if !hasClientAuthEKU {
		return errors.New("CSR must request ExtKeyUsage with ClientAuth")
	}

	return nil
}
