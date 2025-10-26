package artifacts

import (
	"time"

	"github.com/iw2rmb/ploy/internal/workflow/runtime/step"
)

// AddRequest describes an artifact payload to pin within IPFS Cluster.
type AddRequest struct {
	// Name is recorded alongside the pin metadata for operator visibility.
	Name string
	// Kind identifies the artifact category (diffs, logs, etc).
	Kind step.ArtifactKind
	// Payload contains the artifact bytes that should be uploaded.
	Payload []byte
	// ReplicationFactorMin overrides the client default minimum replication factor when >0.
	ReplicationFactorMin int
	// ReplicationFactorMax overrides the client default maximum replication factor when >0.
	ReplicationFactorMax int
	// Local instructs the cluster to keep blocks local to the ingesting peer.
	Local bool
}

// AddResponse summarises an artifact pin result.
type AddResponse struct {
	CID                  string
	Name                 string
	Size                 int64
	Digest               string
	ReplicationFactorMin int
	ReplicationFactorMax int
}

// PinOptions customises replication behaviour for re-pin attempts.
type PinOptions struct {
	ReplicationFactorMin int
	ReplicationFactorMax int
}

// FetchResult returns the payload and metadata for an artifact resolved from the cluster.
type FetchResult struct {
	CID       string
	Digest    string
	Data      []byte
	Size      int64
	MediaType string
}

// StatusPeer captures pin health for a single cluster peer.
type StatusPeer struct {
	PeerID string
	Status string
}

// StatusResult reports the replication status for a pinned artifact.
type StatusResult struct {
	CID                  string
	Name                 string
	Summary              string
	Peers                []StatusPeer
	ReplicationFactorMin int
	ReplicationFactorMax int
	PinState             string
	PinReplicas          int
	PinError             string
	PinRetryCount        int
	PinNextAttemptAt     time.Time
}
