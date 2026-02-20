package handlers

import (
	"crypto/ecdsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"encoding/pem"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"net/netip"
	"os"
	"strings"
	"time"

	domaintypes "github.com/iw2rmb/ploy/internal/domain/types"
	"github.com/iw2rmb/ploy/internal/pki"
	"github.com/iw2rmb/ploy/internal/server/auth"
	"github.com/iw2rmb/ploy/internal/store"
	"github.com/jackc/pgx/v5/pgtype"
)

var errCANotConfigured = errors.New("CA not configured")

// createBootstrapTokenHandler creates a short-lived bootstrap token for node provisioning.
// Requires control-plane or cli-admin role (enforced by middleware).
//
// POST /v1/bootstrap/tokens
// Request: { "node_id": "<nanoid>", "expires_in_minutes": 15 }
// Response: { "token": "eyJ...", "node_id": "...", "expires_at": "..." }
func createBootstrapTokenHandler(st store.Store, tokenSecret string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// Parse request with strict validation.
		var req struct {
			NodeID           domaintypes.NodeID `json:"node_id"`
			ExpiresInMinutes int                `json:"expires_in_minutes"`
		}

		if err := DecodeJSON(w, r, &req, DefaultMaxBodySize); err != nil {
			return
		}

		nodeID := req.NodeID

		// Default expiration to 15 minutes if not specified.
		if req.ExpiresInMinutes <= 0 {
			req.ExpiresInMinutes = 15
		}

		// Get cluster ID from environment.
		clusterID := os.Getenv("PLOY_CLUSTER_ID")
		if clusterID == "" {
			httpErr(w, http.StatusInternalServerError, "server misconfigured: PLOY_CLUSTER_ID not set")
			slog.Error("create bootstrap token: PLOY_CLUSTER_ID not set")
			return
		}

		// Generate bootstrap token.
		now := time.Now()
		expiresAt := now.Add(time.Duration(req.ExpiresInMinutes) * time.Minute)
		token, err := auth.GenerateBootstrapToken(tokenSecret, clusterID, nodeID, expiresAt)
		if err != nil {
			httpErr(w, http.StatusInternalServerError, "failed to generate token: %v", err)
			slog.Error("create bootstrap token: generation failed", "err", err)
			return
		}

		// Parse token to extract token ID.
		claims, err := auth.ValidateToken(token, tokenSecret)
		if err != nil {
			httpErr(w, http.StatusInternalServerError, "failed to validate generated token: %v", err)
			slog.Error("create bootstrap token: validation failed", "err", err)
			return
		}

		// Hash the token for storage.
		hash := sha256.Sum256([]byte(token))
		tokenHash := hex.EncodeToString(hash[:])

		// Get issuer identity from context.
		var issuedBy *string
		if identity, ok := auth.IdentityFromContext(r.Context()); ok {
			issuedBy = &identity.CommonName
		}

		// Store token in database.
		err = st.InsertBootstrapToken(r.Context(), store.InsertBootstrapTokenParams{
			TokenHash: tokenHash,
			TokenID:   claims.ID,
			NodeID:    &nodeID,
			ClusterID: &clusterID,
			IssuedAt:  pgtype.Timestamptz{Time: now, Valid: true},
			ExpiresAt: pgtype.Timestamptz{Time: expiresAt, Valid: true},
			IssuedBy:  issuedBy,
		})
		if err != nil {
			httpErr(w, http.StatusInternalServerError, "failed to store token: %v", err)
			slog.Error("create bootstrap token: database insert failed", "err", err)
			return
		}

		// Return token.
		resp := struct {
			Token     string             `json:"token"`
			NodeID    domaintypes.NodeID `json:"node_id"`
			ExpiresAt time.Time          `json:"expires_at"`
		}{
			Token:     token,
			NodeID:    nodeID,
			ExpiresAt: expiresAt,
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		if err := json.NewEncoder(w).Encode(resp); err != nil {
			slog.Error("create bootstrap token: encode response failed", "err", err)
		}

		slog.Info("bootstrap token created",
			"token_id", claims.ID,
			"node_id", nodeID.String(),
			"expires_at", expiresAt,
			"issued_by", issuedBy,
		)
	}
}

// bootstrapCertificateHandler exchanges a bootstrap token for a signed certificate.
// Requires bootstrap token in Authorization header.
//
// POST /v1/pki/bootstrap
// Request: { "csr": "-----BEGIN CERTIFICATE REQUEST-----..." }
// Response: { "certificate": "...", "ca_bundle": "...", "serial": "...", ... }
func bootstrapCertificateHandler(st store.Store, tokenSecret string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// Extract and validate bootstrap token from Authorization header.
		authHeader := r.Header.Get("Authorization")
		if !strings.HasPrefix(authHeader, "Bearer ") {
			httpErr(w, http.StatusUnauthorized, "missing or invalid Authorization header")
			return
		}

		tokenString := strings.TrimPrefix(authHeader, "Bearer ")

		// Validate token.
		claims, err := auth.ValidateToken(tokenString, tokenSecret)
		if err != nil {
			httpErr(w, http.StatusUnauthorized, "invalid token: %v", err)
			slog.Warn("bootstrap certificate: invalid token", "err", err)
			return
		}

		// Verify token is a bootstrap token.
		if claims.TokenType != auth.TokenTypeBootstrap {
			httpErr(w, http.StatusUnauthorized, "invalid token type: expected bootstrap token")
			return
		}

		// Verify token is not expired.
		if time.Now().After(claims.ExpiresAt.Time) {
			httpErr(w, http.StatusUnauthorized, "token expired")
			return
		}

		// Check if token is revoked.
		revoked, err := st.CheckBootstrapTokenRevoked(r.Context(), claims.ID)
		if err == nil && revoked.Valid {
			httpErr(w, http.StatusUnauthorized, "token revoked")
			return
		}

		// Get bootstrap token info from database.
		tokenInfo, err := st.GetBootstrapToken(r.Context(), claims.ID)
		if err != nil {
			httpErr(w, http.StatusUnauthorized, "token not found or invalid")
			slog.Warn("bootstrap certificate: token not found in database", "token_id", claims.ID, "err", err)
			return
		}

		// Verify token hasn't been used yet.
		if tokenInfo.UsedAt.Valid {
			// If cert was already issued, this is idempotent retry - we could allow it
			// For now, reject to enforce single-use
			httpErr(w, http.StatusUnauthorized, "token already used")
			slog.Warn("bootstrap certificate: token already used", "token_id", claims.ID)
			return
		}

		// Parse request body with strict validation.
		var req struct {
			CSR string `json:"csr"`
		}

		if err := DecodeJSON(w, r, &req, DefaultMaxBodySize); err != nil {
			return
		}

		// Validate CSR is not empty.
		if strings.TrimSpace(req.CSR) == "" {
			httpErr(w, http.StatusBadRequest, "csr field is required")
			return
		}

		// Parse and validate CSR CN matches token's node_id.
		block, _ := pem.Decode([]byte(req.CSR))
		if block == nil || block.Type != "CERTIFICATE REQUEST" {
			httpErr(w, http.StatusBadRequest, "invalid CSR PEM")
			return
		}

		parsedCSR, err := x509.ParseCertificateRequest(block.Bytes)
		if err != nil {
			httpErr(w, http.StatusBadRequest, "parse CSR: %v", err)
			return
		}

		if err := parsedCSR.CheckSignature(); err != nil {
			httpErr(w, http.StatusBadRequest, "verify CSR signature: %v", err)
			return
		}

		// Verify CSR CN matches token's node_id.
		expectedCN := "node:" + claims.NodeID.String()
		if strings.TrimSpace(parsedCSR.Subject.CommonName) != expectedCN {
			httpErr(w, http.StatusBadRequest, "CSR subject common name must match node_id from token")
			slog.Warn("bootstrap certificate: CN mismatch",
				"expected", expectedCN,
				"actual", parsedCSR.Subject.CommonName,
			)
			return
		}

		// Load cluster CA.
		ca, rawCACert, err := loadClusterCA()
		if err != nil {
			if errors.Is(err, errCANotConfigured) {
				httpErr(w, http.StatusServiceUnavailable, "PKI not configured")
			} else {
				httpErr(w, http.StatusInternalServerError, "failed to load CA")
			}
			slog.Error("bootstrap certificate: load CA failed", "err", err)
			return
		}

		// Sign the CSR.
		cert, err := pki.SignNodeCSR(ca, []byte(req.CSR), time.Now())
		if err != nil {
			httpErr(w, http.StatusBadRequest, "sign failed: %v", err)
			slog.Warn("bootstrap certificate: sign CSR failed", "node_id", claims.NodeID, "err", err)
			return
		}

		// Register node in database if it doesn't exist yet.
		// Use the node_id from the bootstrap token and default values for other fields.
		nodeID := claims.NodeID
		if nodeID.IsZero() {
			httpErr(w, http.StatusInternalServerError, "invalid node_id in token")
			slog.Error("bootstrap certificate: invalid node_id", "node_id", claims.NodeID.String())
			return
		}

		// Check if node already exists
		_, err = st.GetNode(r.Context(), nodeID)
		if err != nil {
			// Node doesn't exist, create it with default values.
			// CreateNode now requires an app-supplied ID (NanoID-backed).
			ipAddr, _ := netip.ParseAddr("0.0.0.0")
			_, err = st.CreateNode(r.Context(), store.CreateNodeParams{
				ID:          nodeID,
				Name:        "node-" + nodeID.String(), // Use node ID as name suffix
				IpAddress:   ipAddr,                    // Placeholder IP, will be updated on first heartbeat
				Version:     nil,                       // Will be updated on first heartbeat
				Concurrency: 1,                         // Default concurrency
			})
			if err != nil {
				httpErr(w, http.StatusInternalServerError, "failed to register node: %v", err)
				slog.Error("bootstrap certificate: failed to register node", "node_id", claims.NodeID.String(), "err", err)
				return
			}
			slog.Info("node registered", "node_id", claims.NodeID.String())
		}

		// Mark bootstrap token as used.
		err = st.UpdateBootstrapTokenLastUsed(r.Context(), claims.ID)
		if err != nil {
			slog.Error("bootstrap certificate: failed to mark token as used", "token_id", claims.ID, "err", err)
			// Don't fail the request - cert was already issued
		}

		// Mark cert as issued.
		err = st.MarkBootstrapTokenCertIssued(r.Context(), claims.ID)
		if err != nil {
			slog.Error("bootstrap certificate: failed to mark cert as issued", "token_id", claims.ID, "err", err)
			// Don't fail the request - cert was already issued
		}

		// Generate a long-lived worker bearer token for the node to use for API authentication.
		// The certificate is for the node's own HTTPS server; the bearer token is for control plane auth.
		tokenSecret := os.Getenv("PLOY_AUTH_SECRET")
		if tokenSecret == "" {
			httpErr(w, http.StatusInternalServerError, "server misconfigured: PLOY_AUTH_SECRET not set")
			slog.Error("bootstrap certificate: PLOY_AUTH_SECRET not set")
			return
		}

		clusterID := os.Getenv("PLOY_CLUSTER_ID")
		if clusterID == "" {
			httpErr(w, http.StatusInternalServerError, "server misconfigured: PLOY_CLUSTER_ID not set")
			slog.Error("bootstrap certificate: PLOY_CLUSTER_ID not set")
			return
		}

		// Generate worker token with 1 year expiration
		tokenExpiry := time.Now().AddDate(1, 0, 0)
		workerToken, err := auth.GenerateAPIToken(tokenSecret, clusterID, string(auth.RoleWorker), tokenExpiry)
		if err != nil {
			httpErr(w, http.StatusInternalServerError, "failed to generate worker token: %v", err)
			slog.Error("bootstrap certificate: generate worker token failed", "err", err)
			return
		}

		// Build response with both certificate and bearer token.
		resp := struct {
			Certificate string `json:"certificate"`
			CABundle    string `json:"ca_bundle"`
			Serial      string `json:"serial"`
			Fingerprint string `json:"fingerprint"`
			NotBefore   string `json:"not_before"`
			NotAfter    string `json:"not_after"`
			BearerToken string `json:"bearer_token"`
		}{
			Certificate: cert.CertPEM,
			CABundle:    rawCACert,
			Serial:      cert.Serial,
			Fingerprint: cert.Fingerprint,
			NotBefore:   cert.NotBefore.Format(time.RFC3339),
			NotAfter:    cert.NotAfter.Format(time.RFC3339),
			BearerToken: workerToken,
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		if err := json.NewEncoder(w).Encode(resp); err != nil {
			slog.Error("bootstrap certificate: encode response failed", "err", err)
		}

		slog.Info("bootstrap certificate issued",
			"token_id", claims.ID,
			"node_id", claims.NodeID.String(),
			"serial", cert.Serial,
			"fingerprint", cert.Fingerprint,
			"not_before", cert.NotBefore.Format(time.RFC3339),
			"not_after", cert.NotAfter.Format(time.RFC3339),
		)
	}
}

// loadClusterCA loads the cluster CA certificate and private key from environment variables.
// Returns the parsed CA bundle and the raw CA cert PEM for distribution.
func loadClusterCA() (*pki.CABundle, string, error) {
	// Decode base64-encoded PEM from environment (systemd EnvironmentFile doesn't support multi-line)
	caCertB64 := strings.TrimSpace(os.Getenv("PLOY_SERVER_CA_CERT"))
	caKeyB64 := strings.TrimSpace(os.Getenv("PLOY_SERVER_CA_KEY"))

	if caCertB64 == "" || caKeyB64 == "" {
		return nil, "", errCANotConfigured
	}

	caCertBytes, err := base64.StdEncoding.DecodeString(caCertB64)
	if err != nil {
		return nil, "", fmt.Errorf("decode CA cert: %w", err)
	}

	caKeyBytes, err := base64.StdEncoding.DecodeString(caKeyB64)
	if err != nil {
		return nil, "", fmt.Errorf("decode CA key: %w", err)
	}

	caCertPEM := string(caCertBytes)
	caKeyPEM := string(caKeyBytes)

	if caCertPEM == "" || caKeyPEM == "" {
		return nil, "", errCANotConfigured
	}

	// Parse CA certificate
	block, _ := pem.Decode([]byte(caCertPEM))
	if block == nil || block.Type != "CERTIFICATE" {
		return nil, "", fmt.Errorf("invalid CA cert PEM")
	}

	caCert, err := x509.ParseCertificate(block.Bytes)
	if err != nil {
		return nil, "", fmt.Errorf("parse CA cert: %w", err)
	}

	// Parse CA private key
	keyBlock, _ := pem.Decode([]byte(caKeyPEM))
	if keyBlock == nil {
		return nil, "", fmt.Errorf("invalid CA key PEM")
	}

	var caKey *ecdsa.PrivateKey
	switch keyBlock.Type {
	case "EC PRIVATE KEY":
		key, err := x509.ParseECPrivateKey(keyBlock.Bytes)
		if err != nil {
			return nil, "", fmt.Errorf("parse EC private key: %w", err)
		}
		caKey = key
	case "PRIVATE KEY":
		key, err := x509.ParsePKCS8PrivateKey(keyBlock.Bytes)
		if err != nil {
			return nil, "", fmt.Errorf("parse PKCS8 private key: %w", err)
		}
		ecKey, ok := key.(*ecdsa.PrivateKey)
		if !ok {
			return nil, "", fmt.Errorf("expected ECDSA private key, got %T", key)
		}
		caKey = ecKey
	default:
		return nil, "", fmt.Errorf("unsupported key type: %s", keyBlock.Type)
	}

	ca := &pki.CABundle{
		CertPEM: caCertPEM,
		KeyPEM:  caKeyPEM,
		Cert:    caCert,
		Key:     caKey,
	}

	return ca, caCertPEM, nil
}
