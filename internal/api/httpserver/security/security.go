package security

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"time"
)

// TokenVerifier validates bearer tokens and returns the associated principal.
type TokenVerifier interface {
	Verify(ctx context.Context, token string) (Principal, error)
}

const (
	// ScopeAdmin grants full control-plane administrative access.
	ScopeAdmin = "admin"
	// ScopeMods authorizes access to Mods control-plane APIs.
	ScopeMods = "mods"
	// ScopeJobs authorizes access to job lifecycle APIs.
	ScopeJobs = "jobs"
	// ScopeArtifactsRead authorizes read-only access to artifact APIs.
	ScopeArtifactsRead = "artifact.read"
	// ScopeArtifactsWrite authorizes artifact upload and deletion APIs.
	ScopeArtifactsWrite = "artifact.write"
	// ScopeRegistryPull authorizes registry read operations.
	ScopeRegistryPull = "registry.pull"
	// ScopeRegistryPush authorizes registry write operations.
	ScopeRegistryPush = "registry.push"
)

// Principal represents an authenticated caller.
type Principal struct {
	SecretName string
	TokenID    string
	Scopes     []string
	IssuedAt   time.Time
	ExpiresAt  time.Time
}

// HasScope reports whether the principal includes the provided scope value.
func (p Principal) HasScope(scope string) bool {
	scope = strings.TrimSpace(scope)
	if scope == "" {
		return false
	}
	for _, candidate := range p.Scopes {
		if candidate == scope {
			return true
		}
	}
	return false
}

// Manager constructs middleware that enforces TLS, bearer authentication, and scopes.
type Manager struct {
	verifier TokenVerifier
}

// NewManager returns a new Manager configured with the supplied TokenVerifier.
func NewManager(verifier TokenVerifier) *Manager {
	return &Manager{verifier: verifier}
}

// Middleware wraps handlers with mutual TLS, bearer token, and scope enforcement.
func (m *Manager) Middleware(requiredScopes ...string) func(http.Handler) http.Handler {
	normalizedScopes := normalizeScopes(requiredScopes)
	return func(next http.Handler) http.Handler {
		if m == nil || m.verifier == nil {
			return next
		}
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if !hasClientCertificate(r) {
				writeError(w, http.StatusBadRequest, "mutual TLS required")
				return
			}

			token, ok := extractBearer(r.Header.Get("Authorization"))
			if !ok {
				writeUnauthorized(w, "bearer token required")
				return
			}

			principal, err := m.verifier.Verify(r.Context(), token)
			if err != nil {
				writeUnauthorized(w, "invalid bearer token")
				return
			}

			if len(normalizedScopes) > 0 {
				for _, scope := range normalizedScopes {
					if !principal.HasScope(scope) {
						writeError(w, http.StatusForbidden, "insufficient scope")
						return
					}
				}
			}

			ctx := WithPrincipal(r.Context(), principal)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

type principalContextKey struct{}

// WithPrincipal stores the authenticated principal on the context.
func WithPrincipal(ctx context.Context, principal Principal) context.Context {
	return context.WithValue(ctx, principalContextKey{}, principal)
}

// PrincipalFromContext retrieves the principal from context.
func PrincipalFromContext(ctx context.Context) (Principal, bool) {
	if ctx == nil {
		return Principal{}, false
	}
	value := ctx.Value(principalContextKey{})
	if value == nil {
		return Principal{}, false
	}
	principal, ok := value.(Principal)
	return principal, ok
}

func hasClientCertificate(r *http.Request) bool {
	if r == nil || r.TLS == nil {
		return false
	}
	if len(r.TLS.PeerCertificates) > 0 {
		return true
	}
	return len(r.TLS.VerifiedChains) > 0
}

func extractBearer(header string) (string, bool) {
	header = strings.TrimSpace(header)
	if header == "" {
		return "", false
	}
	if !strings.Contains(header, " ") {
		return "", false
	}
	parts := strings.SplitN(header, " ", 2)
	if len(parts) != 2 {
		return "", false
	}
	if !strings.EqualFold(parts[0], "Bearer") {
		return "", false
	}
	token := strings.TrimSpace(parts[1])
	if token == "" {
		return "", false
	}
	return token, true
}

func normalizeScopes(scopes []string) []string {
	if len(scopes) == 0 {
		return nil
	}
	seen := make(map[string]struct{}, len(scopes))
	out := make([]string, 0, len(scopes))
	for _, scope := range scopes {
		scope = strings.TrimSpace(scope)
		if scope == "" {
			continue
		}
		if _, ok := seen[scope]; ok {
			continue
		}
		seen[scope] = struct{}{}
		out = append(out, scope)
	}
	return out
}

func writeUnauthorized(w http.ResponseWriter, message string) {
	w.Header().Set("WWW-Authenticate", `Bearer realm="ploy-control-plane"`)
	writeError(w, http.StatusUnauthorized, message)
}

func writeError(w http.ResponseWriter, status int, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]string{
		"error": message,
	})
}
