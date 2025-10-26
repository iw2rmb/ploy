package httpserver

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	controlplaneartifacts "github.com/iw2rmb/ploy/internal/controlplane/artifacts"
	gonanoid "github.com/matoous/go-nanoid/v2"
)

const (
	defaultArtifactLimit = 50
	maxArtifactLimit     = 200
)

func generateArtifactID() string {
	id, err := gonanoid.Generate("abcdefghijklmnopqrstuvwxyz0123456789", 16)
	if err != nil {
		return fmt.Sprintf("artifact-%d", time.Now().UnixNano())
	}
	return "artifact-" + id
}

func parseArtifactLimit(raw string) (int, error) {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return defaultArtifactLimit, nil
	}
	limit, err := strconv.Atoi(trimmed)
	if err != nil || limit <= 0 {
		return 0, fmt.Errorf("invalid limit")
	}
	if limit > maxArtifactLimit {
		limit = maxArtifactLimit
	}
	return limit, nil
}

func parseOptionalIntParam(raw string) (int, error) {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return 0, nil
	}
	value, err := strconv.Atoi(trimmed)
	if err != nil || value < 0 {
		return 0, fmt.Errorf("invalid integer")
	}
	return value, nil
}

func firstNonZero(values ...int) int {
	for _, v := range values {
		if v > 0 {
			return v
		}
	}
	return 0
}

func firstNonZero64(values ...int64) int64 {
	for _, v := range values {
		if v > 0 {
			return v
		}
	}
	return 0
}

type artifactDTO struct {
	ID                   string `json:"id"`
	JobID                string `json:"job_id"`
	Stage                string `json:"stage,omitempty"`
	Kind                 string `json:"kind,omitempty"`
	NodeID               string `json:"node_id,omitempty"`
	CID                  string `json:"cid"`
	Digest               string `json:"digest"`
	Size                 int64  `json:"size"`
	Name                 string `json:"name,omitempty"`
	Source               string `json:"source,omitempty"`
	TTL                  string `json:"ttl,omitempty"`
	ExpiresAt            string `json:"expires_at,omitempty"`
	ReplicationFactorMin int    `json:"replication_factor_min,omitempty"`
	ReplicationFactorMax int    `json:"replication_factor_max,omitempty"`
	PinState             string `json:"pin_state,omitempty"`
	PinReplicas          int    `json:"pin_replicas,omitempty"`
	PinRetryCount        int    `json:"pin_retry_count,omitempty"`
	PinError             string `json:"pin_error,omitempty"`
	PinUpdatedAt         string `json:"pin_updated_at,omitempty"`
	PinNextAttemptAt     string `json:"pin_next_attempt_at,omitempty"`
	CreatedAt            string `json:"created_at"`
	UpdatedAt            string `json:"updated_at"`
	DeletedAt            string `json:"deleted_at,omitempty"`
}

func artifactDTOFrom(meta controlplaneartifacts.Metadata) artifactDTO {
	dto := artifactDTO{
		ID:                   meta.ID,
		JobID:                meta.JobID,
		Stage:                meta.Stage,
		Kind:                 meta.Kind,
		NodeID:               meta.NodeID,
		CID:                  meta.CID,
		Digest:               meta.Digest,
		Size:                 meta.Size,
		Name:                 meta.Name,
		Source:               meta.Source,
		TTL:                  meta.TTL,
		ReplicationFactorMin: meta.ReplicationFactorMin,
		ReplicationFactorMax: meta.ReplicationFactorMax,
		PinState:             string(meta.PinState),
		PinReplicas:          meta.PinReplicas,
		PinRetryCount:        meta.PinRetryCount,
		PinError:             meta.PinError,
		CreatedAt:            formatTime(meta.CreatedAt),
		UpdatedAt:            formatTime(meta.UpdatedAt),
	}
	if !meta.ExpiresAt.IsZero() {
		dto.ExpiresAt = formatTime(meta.ExpiresAt)
	}
	if !meta.PinUpdatedAt.IsZero() {
		dto.PinUpdatedAt = formatTime(meta.PinUpdatedAt)
	}
	if !meta.PinNextAttemptAt.IsZero() {
		dto.PinNextAttemptAt = formatTime(meta.PinNextAttemptAt)
	}
	if !meta.DeletedAt.IsZero() {
		dto.DeletedAt = formatTime(meta.DeletedAt)
	}
	return dto
}

func recordArtifactRequest(method string, status int) {
	method = strings.ToUpper(strings.TrimSpace(method))
	if method == "" {
		method = "UNKNOWN"
	}
	artifactRequestsTotal.WithLabelValues(method, strconv.Itoa(status)).Inc()
}

func recordArtifactPayload(operation string, bytesCopied int64) {
	if bytesCopied <= 0 {
		return
	}
	operation = strings.TrimSpace(operation)
	if operation == "" {
		operation = "unknown"
	}
	artifactPayloadBytes.WithLabelValues(operation).Add(float64(bytesCopied))
}

func wantsDownload(raw string) bool {
	trimmed := strings.ToLower(strings.TrimSpace(raw))
	if trimmed == "" || trimmed == "0" || trimmed == "false" {
		return false
	}
	return true
}
