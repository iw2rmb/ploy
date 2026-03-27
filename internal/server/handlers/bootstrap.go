package handlers

import (
	"context"
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
		claims, err := validateBootstrapToken(r, st, tokenSecret)
		if err != nil {
			httpErr(w, http.StatusUnauthorized, "%s", err)
			return
		}

		// Parse request body with strict validation.
		var req struct {
			CSR string `json:"csr"`
		}
		if err := DecodeJSON(w, r, &req, DefaultMaxBodySize); err != nil {
			return
		}

		parsedCSR, err := parseAndVerifyCSR(req.CSR, "node:"+claims.NodeID.String())
		if err != nil {
			httpErr(w, http.StatusBadRequest, "%s", err)
			return
		}
		_ = parsedCSR // signature/CN already verified

		// Load cluster CA and sign the CSR.
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

		cert, err := pki.SignNodeCSR(ca, []byte(req.CSR), time.Now())
		if err != nil {
			httpErr(w, http.StatusBadRequest, "sign failed: %v", err)
			slog.Warn("bootstrap certificate: sign CSR failed", "node_id", claims.NodeID, "err", err)
			return
		}

		if err := registerNodeIfNew(r.Context(), st, claims.NodeID); err != nil {
			httpErr(w, http.StatusInternalServerError, "failed to register node: %v", err)
			return
		}

		// Mark bootstrap token as used (best-effort, cert already issued).
		if err := st.UpdateBootstrapTokenLastUsed(r.Context(), claims.ID); err != nil {
			slog.Error("bootstrap certificate: failed to mark token as used", "token_id", claims.ID, "err", err)
		}
		if err := st.MarkBootstrapTokenCertIssued(r.Context(), claims.ID); err != nil {
			slog.Error("bootstrap certificate: failed to mark cert as issued", "token_id", claims.ID, "err", err)
		}

		workerToken, err := issueWorkerToken()
		if err != nil {
			httpErr(w, http.StatusInternalServerError, "%s", err)
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

// validateBootstrapToken extracts and validates a bootstrap token from the
// Authorization header. Returns validated claims or an error suitable for
// the HTTP response.
func validateBootstrapToken(r *http.Request, st store.Store, tokenSecret string) (*auth.TokenClaims, error) {
	authHeader := r.Header.Get("Authorization")
	if !strings.HasPrefix(authHeader, "Bearer ") {
		return nil, fmt.Errorf("missing or invalid Authorization header")
	}

	tokenString := strings.TrimPrefix(authHeader, "Bearer ")

	claims, err := auth.ValidateToken(tokenString, tokenSecret)
	if err != nil {
		slog.Warn("bootstrap certificate: invalid token", "err", err)
		return nil, fmt.Errorf("invalid token: %w", err)
	}

	if claims.TokenType != auth.TokenTypeBootstrap {
		return nil, fmt.Errorf("invalid token type: expected bootstrap token")
	}

	if time.Now().After(claims.ExpiresAt.Time) {
		return nil, fmt.Errorf("token expired")
	}

	revoked, err := st.CheckBootstrapTokenRevoked(r.Context(), claims.ID)
	if err == nil && revoked.Valid {
		return nil, fmt.Errorf("token revoked")
	}

	tokenInfo, err := st.GetBootstrapToken(r.Context(), claims.ID)
	if err != nil {
		slog.Warn("bootstrap certificate: token not found in database", "token_id", claims.ID, "err", err)
		return nil, fmt.Errorf("token not found or invalid")
	}

	if tokenInfo.UsedAt.Valid {
		slog.Warn("bootstrap certificate: token already used", "token_id", claims.ID)
		return nil, fmt.Errorf("token already used")
	}

	return claims, nil
}

// parseAndVerifyCSR parses a PEM-encoded CSR, verifies its signature, and
// checks that the CN matches expectedCN.
func parseAndVerifyCSR(csrPEM string, expectedCN string) (*x509.CertificateRequest, error) {
	if strings.TrimSpace(csrPEM) == "" {
		return nil, fmt.Errorf("csr field is required")
	}

	block, _ := pem.Decode([]byte(csrPEM))
	if block == nil || block.Type != "CERTIFICATE REQUEST" {
		return nil, fmt.Errorf("invalid CSR PEM")
	}

	parsed, err := x509.ParseCertificateRequest(block.Bytes)
	if err != nil {
		return nil, fmt.Errorf("parse CSR: %w", err)
	}

	if err := parsed.CheckSignature(); err != nil {
		return nil, fmt.Errorf("verify CSR signature: %w", err)
	}

	if strings.TrimSpace(parsed.Subject.CommonName) != expectedCN {
		slog.Warn("bootstrap certificate: CN mismatch",
			"expected", expectedCN,
			"actual", parsed.Subject.CommonName,
		)
		return nil, fmt.Errorf("CSR subject common name must match node_id from token")
	}

	return parsed, nil
}

// registerNodeIfNew creates a node record with defaults if it doesn't already exist.
func registerNodeIfNew(ctx context.Context, st store.Store, nodeID domaintypes.NodeID) error {
	if nodeID.IsZero() {
		return fmt.Errorf("invalid node_id")
	}

	if _, err := st.GetNode(ctx, nodeID); err == nil {
		return nil // already exists
	}

	ipAddr, _ := netip.ParseAddr("0.0.0.0")
	if _, err := st.CreateNode(ctx, store.CreateNodeParams{
		ID:          nodeID,
		Name:        "node-" + nodeID.String(),
		IpAddress:   ipAddr,
		Version:     nil,
		Concurrency: 1,
	}); err != nil {
		slog.Error("bootstrap certificate: failed to register node", "node_id", nodeID.String(), "err", err)
		return err
	}
	slog.Info("node registered", "node_id", nodeID.String())
	return nil
}

// issueWorkerToken generates a long-lived worker bearer token for API authentication.
func issueWorkerToken() (string, error) {
	secret := os.Getenv("PLOY_AUTH_SECRET")
	if secret == "" {
		slog.Error("bootstrap certificate: PLOY_AUTH_SECRET not set")
		return "", fmt.Errorf("server misconfigured: PLOY_AUTH_SECRET not set")
	}

	clusterID := os.Getenv("PLOY_CLUSTER_ID")
	if clusterID == "" {
		slog.Error("bootstrap certificate: PLOY_CLUSTER_ID not set")
		return "", fmt.Errorf("server misconfigured: PLOY_CLUSTER_ID not set")
	}

	tokenExpiry := time.Now().AddDate(1, 0, 0)
	token, err := auth.GenerateAPIToken(secret, clusterID, string(auth.RoleWorker), tokenExpiry)
	if err != nil {
		slog.Error("bootstrap certificate: generate worker token failed", "err", err)
		return "", fmt.Errorf("failed to generate worker token: %w", err)
	}
	return token, nil
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
