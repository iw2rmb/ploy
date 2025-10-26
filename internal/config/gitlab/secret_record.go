package gitlab

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"go.etcd.io/etcd/api/v3/mvccpb"
)

type secretRecord struct {
	SecretName string   `json:"secret_name"`
	Ciphertext string   `json:"ciphertext"`
	Scopes     []string `json:"scopes"`
	ScopeJSON  string   `json:"scope_json"`
	UpdatedAt  string   `json:"updated_at"`
}

type issuedToken struct {
	tokenID   string
	nodeID    string
	issuedAt  time.Time
	expiresAt time.Time
}

func (r secretRecord) updatedAt() time.Time {
	if r.UpdatedAt == "" {
		return time.Time{}
	}
	ts, err := time.Parse(time.RFC3339Nano, r.UpdatedAt)
	if err != nil {
		return time.Time{}
	}
	return ts
}

func (r secretRecord) decrypt(ctx context.Context, cipher Cipher) (string, error) {
	raw, err := base64.StdEncoding.DecodeString(r.Ciphertext)
	if err != nil {
		return "", fmt.Errorf("gitlab signer: decode ciphertext: %w", err)
	}
	plain, err := cipher.Decrypt(ctx, raw)
	if err != nil {
		return "", fmt.Errorf("gitlab signer: decrypt api key: %w", err)
	}
	return string(plain), nil
}

func decodeSecretRecord(kv *mvccpb.KeyValue, prefix string) (secretRecord, error) {
	var record secretRecord
	if err := json.Unmarshal(kv.Value, &record); err != nil {
		return secretRecord{}, fmt.Errorf("gitlab signer: decode secret payload: %w", err)
	}
	if record.SecretName == "" {
		record.SecretName = strings.TrimPrefix(string(kv.Key), prefix)
	}
	if len(record.Scopes) == 0 && record.ScopeJSON != "" {
		var scopes []string
		if err := json.Unmarshal([]byte(record.ScopeJSON), &scopes); err == nil {
			record.Scopes = scopes
		}
	}
	return record, nil
}

func normalizeScopes(scopes []string) []string {
	cleaned := cleanList(scopes, false)
	result := make([]string, 0, len(cleaned))
	for _, scope := range cleaned {
		if scope != "" {
			result = append(result, scope)
		}
	}
	return result
}

func ensureScopesAllowed(allowed, requested []string) error {
	set := make(map[string]struct{}, len(allowed))
	for _, scope := range allowed {
		set[scope] = struct{}{}
	}
	for _, scope := range requested {
		if _, ok := set[scope]; !ok {
			return fmt.Errorf("gitlab signer: scope %q not permitted", scope)
		}
	}
	return nil
}
