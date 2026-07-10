package gitlabtoken

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
)

const (
	RunMetadataKey       = "ploy.gitlab_token_hash"
	MissingTokenMessage  = "run requires an ephemeral GitLab token, but the token is no longer available on this server"
	ProvidedTokenContext = "provided GitLab token"
)

var ErrMissingToken = errors.New(MissingTokenMessage)

func Hash(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}

func ValidateRequest(token string) (string, string, error) {
	token = strings.TrimSpace(token)
	if token == "" {
		return "", "", errors.New("gitlab_token must be non-empty")
	}
	return Hash(token), token, nil
}

func RunStatsWithMarker(hash string) ([]byte, error) {
	hash = strings.TrimSpace(hash)
	if hash == "" {
		return nil, nil
	}
	if len(hash) != 64 {
		return nil, fmt.Errorf("gitlab token hash must be a lowercase sha256 hex digest")
	}
	for _, c := range hash {
		if !('0' <= c && c <= '9') && !('a' <= c && c <= 'f') {
			return nil, fmt.Errorf("gitlab token hash must be a lowercase sha256 hex digest")
		}
	}
	return json.Marshal(map[string]map[string]string{
		"metadata": {
			RunMetadataKey: hash,
		},
	})
}

func HashFromRunStats(raw []byte) string {
	if len(raw) == 0 {
		return ""
	}
	var stats struct {
		Metadata map[string]string `json:"metadata"`
	}
	if err := json.Unmarshal(raw, &stats); err != nil {
		return ""
	}
	return strings.TrimSpace(stats.Metadata[RunMetadataKey])
}
