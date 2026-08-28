package api

import (
	"encoding/json"

	"github.com/iw2rmb/ploy/internal/domain/types"
)

type Node struct {
	ID              types.NodeID `json:"id"`
	Name            string       `json:"name"`
	IPAddress       string       `json:"ip_address"`
	Version         *string      `json:"version,omitempty"`
	Concurrency     int32        `json:"concurrency"`
	CPUTotalMillis  int32        `json:"cpu_total_millis"`
	CPUFreeMillis   int32        `json:"cpu_free_millis"`
	MemTotalBytes   int64        `json:"mem_total_bytes"`
	MemFreeBytes    int64        `json:"mem_free_bytes"`
	DiskTotalBytes  int64        `json:"disk_total_bytes"`
	DiskFreeBytes   int64        `json:"disk_free_bytes"`
	CertSerial      *string      `json:"cert_serial,omitempty"`
	CertFingerprint *string      `json:"cert_fingerprint,omitempty"`
	CertNotBefore   *string      `json:"cert_not_before,omitempty"`
	CertNotAfter    *string      `json:"cert_not_after,omitempty"`
	LastHeartbeat   *string      `json:"last_heartbeat,omitempty"`
	Drained         bool         `json:"drained"`
	CreatedAt       string       `json:"created_at"`
}

type NodeDiagnostic struct {
	NodeID        types.NodeID    `json:"node_id"`
	Component     string          `json:"component"`
	Status        string          `json:"status"`
	LastError     *string         `json:"last_error,omitempty"`
	Version       *string         `json:"version,omitempty"`
	ImageRef      *string         `json:"image_ref,omitempty"`
	LocalImageID  *string         `json:"local_image_id,omitempty"`
	RemoteImageID *string         `json:"remote_image_id,omitempty"`
	Details       json.RawMessage `json:"details"`
	LastCheckedAt *string         `json:"last_checked_at,omitempty"`
	LastSuccessAt *string         `json:"last_success_at,omitempty"`
	UpdatedAt     string          `json:"updated_at"`
}
