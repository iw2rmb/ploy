package httpserver

import (
	"encoding/json"
	"log"
	"net/http"
	"os"
	"time"

	"github.com/google/uuid"
	"github.com/iw2rmb/ploy/internal/pki"
	"github.com/iw2rmb/ploy/internal/store"
	"github.com/jackc/pgx/v5/pgtype"
)

// PKISignRequest represents a request to sign a node CSR.
type PKISignRequest struct {
	NodeID string `json:"node_id"`
	CSR    string `json:"csr"`
}

// PKISignResponse represents a signed certificate response.
type PKISignResponse struct {
	Certificate string    `json:"certificate"`
	CABundle    string    `json:"ca_bundle"`
	Serial      string    `json:"serial"`
	Fingerprint string    `json:"fingerprint"`
	NotBefore   time.Time `json:"not_before"`
	NotAfter    time.Time `json:"not_after"`
}

// handlePKISign signs a node certificate signing request.
// POST /v1/pki/sign
func (s *controlPlaneServer) handlePKISign(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.Header().Set("Allow", http.MethodPost)
		writeErrorMessage(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	if s.store == nil {
		writeErrorMessage(w, http.StatusServiceUnavailable, "store unavailable")
		return
	}

	var req PKISignRequest
	if err := decodeJSON(r, &req); err != nil {
		writeErrorMessage(w, http.StatusBadRequest, "invalid JSON")
		return
	}

	if req.NodeID == "" {
		writeErrorMessage(w, http.StatusBadRequest, "node_id required")
		return
	}
	if req.CSR == "" {
		writeErrorMessage(w, http.StatusBadRequest, "csr required")
		return
	}

	// Parse node ID as UUID.
	nodeUUID, err := uuid.Parse(req.NodeID)
	if err != nil {
		writeErrorMessage(w, http.StatusBadRequest, "invalid node_id format")
		return
	}

	// Load CA from environment or configuration.
	// For now, we assume CA cert and key are available via environment variables
	// PLOY_SERVER_CA_CERT and PLOY_SERVER_CA_KEY as specified in SIMPLE.md.
	// In production, these would be loaded from secure storage.
	caCertPEM := os.Getenv("PLOY_SERVER_CA_CERT")
	caKeyPEM := os.Getenv("PLOY_SERVER_CA_KEY")
	if caCertPEM == "" || caKeyPEM == "" {
		log.Printf("pki: CA certificate or key not configured")
		writeErrorMessage(w, http.StatusServiceUnavailable, "PKI not configured")
		return
	}

	ca, err := pki.LoadCA(caCertPEM, caKeyPEM)
	if err != nil {
		log.Printf("pki: load CA: %v", err)
		writeErrorMessage(w, http.StatusInternalServerError, "failed to load CA")
		return
	}

	// Sign the CSR.
	now := time.Now().UTC()
	cert, err := pki.SignNodeCSR(ca, []byte(req.CSR), now)
	if err != nil {
		log.Printf("pki: sign CSR for node %s: %v", req.NodeID, err)
		writeErrorMessage(w, http.StatusBadRequest, "failed to sign CSR")
		return
	}

	// Persist certificate metadata to the nodes table.
	ctx := r.Context()
	params := store.UpdateNodeCertMetadataParams{
		ID:              pgtype.UUID{Bytes: nodeUUID, Valid: true},
		CertSerial:      &cert.Serial,
		CertFingerprint: &cert.Fingerprint,
		CertNotBefore:   pgtype.Timestamptz{Time: cert.NotBefore, Valid: true},
		CertNotAfter:    pgtype.Timestamptz{Time: cert.NotAfter, Valid: true},
	}
	if err := s.store.UpdateNodeCertMetadata(ctx, params); err != nil {
		log.Printf("pki: update node cert metadata for %s: %v", req.NodeID, err)
		writeErrorMessage(w, http.StatusInternalServerError, "failed to persist certificate metadata")
		return
	}

	resp := PKISignResponse{
		Certificate: cert.CertPEM,
		CABundle:    ca.CertPEM,
		Serial:      cert.Serial,
		Fingerprint: cert.Fingerprint,
		NotBefore:   cert.NotBefore,
		NotAfter:    cert.NotAfter,
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(resp)
}
