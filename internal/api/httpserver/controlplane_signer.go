package httpserver

import (
	"context"
	"github.com/iw2rmb/ploy/internal/config/gitlab"
	"net/http"
	"strconv"
	"strings"
	"time"
)

func (s *controlPlaneServer) handleSignerSecrets(w http.ResponseWriter, r *http.Request) {
	if s.signer == nil {
		http.Error(w, "gitlab signer unavailable", http.StatusServiceUnavailable)
		return
	}
	if r.Method != http.MethodPut {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var req struct {
		Secret string   `json:"secret"`
		APIKey string   `json:"api_key"`
		Scopes []string `json:"scopes"`
	}
	if err := decodeJSON(r, &req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	result, err := s.signer.RotateSecret(r.Context(), gitlab.RotateSecretRequest{
		SecretName: req.Secret,
		APIKey:     req.APIKey,
		Scopes:     req.Scopes,
	})
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	payload := map[string]any{
		"secret":     strings.TrimSpace(req.Secret),
		"revision":   result.Revision,
		"updated_at": result.UpdatedAt.UTC().Format(time.RFC3339Nano),
	}
	writeJSON(w, http.StatusOK, payload)
}

func (s *controlPlaneServer) handleSignerTokens(w http.ResponseWriter, r *http.Request) {
	if s.signer == nil {
		http.Error(w, "gitlab signer unavailable", http.StatusServiceUnavailable)
		return
	}
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var req struct {
		Secret     string   `json:"secret"`
		Scopes     []string `json:"scopes"`
		TTLSeconds int64    `json:"ttl_seconds"`
		NodeID     string   `json:"node_id"`
	}
	if err := decodeJSON(r, &req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if strings.TrimSpace(req.NodeID) == "" {
		http.Error(w, "node_id required", http.StatusBadRequest)
		return
	}
	ttl := time.Duration(req.TTLSeconds) * time.Second
	token, err := s.signer.IssueToken(r.Context(), gitlab.IssueTokenRequest{
		SecretName: req.Secret,
		Scopes:     req.Scopes,
		TTL:        ttl,
		NodeID:     req.NodeID,
	})
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	payload := map[string]any{
		"secret":      token.SecretName,
		"token":       token.Value,
		"scopes":      token.Scopes,
		"issued_at":   token.IssuedAt.UTC().Format(time.RFC3339Nano),
		"expires_at":  token.ExpiresAt.UTC().Format(time.RFC3339Nano),
		"ttl_seconds": int64(token.ExpiresAt.Sub(token.IssuedAt).Seconds()),
		"token_id":    token.TokenID,
	}
	writeJSON(w, http.StatusOK, payload)
}

func (s *controlPlaneServer) handleSignerRotations(w http.ResponseWriter, r *http.Request) {
	if s.rotations == nil {
		http.Error(w, "gitlab rotations unavailable", http.StatusServiceUnavailable)
		return
	}
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	timeout := 30 * time.Second
	if raw := strings.TrimSpace(r.URL.Query().Get("timeout")); raw != "" {
		dur, err := time.ParseDuration(raw)
		if err != nil {
			http.Error(w, "invalid timeout duration", http.StatusBadRequest)
			return
		}
		if dur > 0 {
			timeout = dur
		} else {
			timeout = 0
		}
	}

	var since int64
	if raw := strings.TrimSpace(r.URL.Query().Get("since")); raw != "" {
		revision, err := strconv.ParseInt(raw, 10, 64)
		if err != nil {
			http.Error(w, "invalid since revision", http.StatusBadRequest)
			return
		}
		since = revision
	}

	ctx := r.Context()
	if timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, timeout)
		defer cancel()
	}

	secret := strings.TrimSpace(r.URL.Query().Get("secret"))
	evt, ok := s.rotations.Wait(ctx, secret, since)
	if !ok {
		w.WriteHeader(http.StatusNoContent)
		return
	}

	payload := map[string]any{
		"secret":   evt.SecretName,
		"revision": evt.Revision,
	}
	if !evt.UpdatedAt.IsZero() {
		payload["updated_at"] = evt.UpdatedAt.UTC().Format(time.RFC3339Nano)
	}
	writeJSON(w, http.StatusOK, payload)
}
