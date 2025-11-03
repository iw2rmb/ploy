package handlers

import (
	"crypto/x509"
	"encoding/json"
	"encoding/pem"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"

	internalPKI "github.com/iw2rmb/ploy/internal/pki"
	"github.com/iw2rmb/ploy/internal/store"
)

// pkiSignHandler returns an HTTP handler that signs node CSRs with the cluster CA.
// It requires admin role authorization and returns a PEM bundle with the signed certificate.
func pkiSignHandler(st store.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// Decode request body.
		var req struct {
			NodeID string `json:"node_id"`
			CSR    string `json:"csr"`
		}

		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, fmt.Sprintf("invalid request: %v", err), http.StatusBadRequest)
			return
		}

		// Validate node_id format.
		nodeUUID, err := uuid.Parse(req.NodeID)
		if err != nil {
			http.Error(w, fmt.Sprintf("invalid node_id: %v", err), http.StatusBadRequest)
			return
		}

		// Validate CSR is not empty.
		if strings.TrimSpace(req.CSR) == "" {
			http.Error(w, "csr field is required", http.StatusBadRequest)
			return
		}

		// Load cluster CA from environment or well-known file paths
		ca, rawCACert, err := loadClusterCA()
		if err != nil {
			if errors.Is(err, errCANotConfigured) {
				http.Error(w, "PKI not configured", http.StatusServiceUnavailable)
			} else {
				http.Error(w, "failed to load CA", http.StatusInternalServerError)
			}
			slog.Error("pki sign: load CA failed", "err", err)
			return
		}

		// Parse CSR to validate subject common name matches node_id when possible.
		if block, _ := pem.Decode([]byte(req.CSR)); block != nil && block.Type == "CERTIFICATE REQUEST" {
			if parsedCSR, err := x509.ParseCertificateRequest(block.Bytes); err == nil {
				if err := parsedCSR.CheckSignature(); err == nil {
					expectedCN := "node:" + req.NodeID
					if strings.TrimSpace(parsedCSR.Subject.CommonName) != expectedCN {
						http.Error(w, "csr subject common name must match node:<node_id>", http.StatusBadRequest)
						return
					}
				}
			}
			// If parsing/signature fails, fall through to SignNodeCSR for consistent error path.
		}

		// Sign the CSR.
		cert, err := internalPKI.SignNodeCSR(ca, []byte(req.CSR), time.Now())
		if err != nil {
			http.Error(w, fmt.Sprintf("sign failed: %v", err), http.StatusBadRequest)
			slog.Warn("pki sign: sign CSR failed", "node_id", req.NodeID, "err", err)
			return
		}

		// Persist certificate metadata to the database.
		err = st.UpdateNodeCertMetadata(r.Context(), store.UpdateNodeCertMetadataParams{
			ID: pgtype.UUID{
				Bytes: nodeUUID,
				Valid: true,
			},
			CertSerial:      &cert.Serial,
			CertFingerprint: &cert.Fingerprint,
			CertNotBefore: pgtype.Timestamptz{
				Time:  cert.NotBefore,
				Valid: true,
			},
			CertNotAfter: pgtype.Timestamptz{
				Time:  cert.NotAfter,
				Valid: true,
			},
		})
		if err != nil {
			http.Error(w, "failed to persist certificate metadata", http.StatusInternalServerError)
			slog.Error("pki sign: persist metadata failed", "node_id", req.NodeID, "err", err)
			return
		}

		// Build response according to docs/api/components/schemas/pki.yaml.
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
			slog.Error("pki sign: encode response failed", "err", err)
		}

		slog.Info("pki sign: certificate issued",
			"node_id", req.NodeID,
			"serial", cert.Serial,
			"fingerprint", cert.Fingerprint,
			"not_before", cert.NotBefore.Format(time.RFC3339),
			"not_after", cert.NotAfter.Format(time.RFC3339),
		)
	}
}

// loadClusterCA loads the CA from env PEM, env file paths, or default filesystem paths.
// Returns the parsed CA bundle and the raw CA certificate PEM for inclusion in responses.
var errCANotConfigured = errors.New("pki: ca not configured")

func loadClusterCA() (*internalPKI.CABundle, string, error) {
	// 1) Direct PEMs via env
	rawCACert := strings.TrimSpace(os.Getenv("PLOY_SERVER_CA_CERT"))
	rawCAKey := strings.TrimSpace(os.Getenv("PLOY_SERVER_CA_KEY"))
	if rawCACert != "" && rawCAKey != "" {
		ca, err := internalPKI.LoadCA(rawCACert, rawCAKey)
		return ca, rawCACert, err
	}

	// 2) File paths via env
	certPath := strings.TrimSpace(os.Getenv("PLOY_SERVER_CA_CERT_PATH"))
	keyPath := strings.TrimSpace(os.Getenv("PLOY_SERVER_CA_KEY_PATH"))
	if certPath != "" && keyPath != "" {
		certPEM, err1 := os.ReadFile(certPath)
		keyPEM, err2 := os.ReadFile(keyPath)
		if err1 != nil {
			return nil, "", err1
		}
		if err2 != nil {
			return nil, "", err2
		}
		ca, err := internalPKI.LoadCA(string(certPEM), string(keyPEM))
		return ca, string(certPEM), err
	}

	// 3) Default paths
	const defaultCert = "/etc/ploy/pki/ca.crt"
	const defaultKey = "/etc/ploy/pki/ca.key"
	if data, err := os.ReadFile(defaultCert); err == nil {
		if key, err2 := os.ReadFile(defaultKey); err2 == nil {
			ca, err := internalPKI.LoadCA(string(data), string(key))
			return ca, string(data), err
		}
	}

	return nil, "", errCANotConfigured
}
