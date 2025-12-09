package deploy

import (
	"errors"
	"time"

	domaintypes "github.com/iw2rmb/ploy/internal/domain/types"
)

const (
	//nolint:unused // reserved for future CA rotation configuration defaults
	defaultCAValidity   = 365 * 24 * time.Hour //nolint:unused // reserved for future CA rotation defaults
	defaultLeafValidity = 90 * 24 * time.Hour  //nolint:unused // reserved for future CA rotation defaults
	certSerialBitSize   = 160                  //nolint:unused // reserved for future CA rotation defaults
	clusterSecurityRoot = "/ploy/clusters"     //nolint:unused // reserved for future CA rotation defaults

	certificateRoleControlPlane = "control-plane" //nolint:unused // reserved for future CA rotation ACLs
	certificateRoleWorker       = "worker"        //nolint:unused // reserved for future CA rotation ACLs
	certificateRoleCLIAdmin     = "cli-admin"     //nolint:unused // reserved for future CA rotation ACLs
)

var (
	// ErrPKINotBootstrapped indicates the cluster security materials have not been generated yet.
	ErrPKINotBootstrapped = errors.New("deploy: cluster PKI not bootstrapped")
	// ErrPKIAlreadyBootstrapped indicates bootstrap was attempted when materials already exist.
	ErrPKIAlreadyBootstrapped = errors.New("deploy: cluster PKI already bootstrapped")
	// ErrConcurrentRotation signals the CA changed while processing a rotation.
	ErrConcurrentRotation = errors.New("deploy: concurrent CA rotation detected")
	// ErrWorkerExists indicates the worker has already been registered.
	ErrWorkerExists = errors.New("deploy: worker already registered")
	// ErrWorkerNotFound indicates the worker is missing from the inventory.
	ErrWorkerNotFound = errors.New("deploy: worker not registered")
	// ErrConcurrentWorkerUpdate signals a concurrent modification to the worker inventory.
	ErrConcurrentWorkerUpdate = errors.New("deploy: concurrent worker update detected")
)

// BootstrapOptions configure the initial PKI generation.
type BootstrapOptions struct {
	BeaconIDs    []string
	WorkerIDs    []string
	CAValidity   time.Duration
	LeafValidity time.Duration
	RequestedAt  time.Time
}

// RotateOptions configure a CA rotation request.
type RotateOptions struct {
	DryRun      bool
	RequestedAt time.Time
	Operator    string
	Reason      string
}

// CAState describes the persisted CA state for a cluster.
// Uses domain type (ClusterID) for type-safe identification.
type CAState struct {
	ClusterID          domaintypes.ClusterID `json:"cluster_id"` // Cluster ID
	Nodes              NodeSet
	CurrentCA          CABundle
	BeaconCertificates map[string]LeafCertificate
	WorkerCertificates map[string]LeafCertificate
	Revoked            []RevokedRecord
	Revision           int64
	NodesRevision      int64
}

// CABundle captures certificate authority metadata and PEM-encoded material.
type CABundle struct {
	Version        string    `json:"version"`
	SerialNumber   string    `json:"serial_number"`
	Subject        string    `json:"subject"`
	IssuedAt       time.Time `json:"issued_at"`
	ExpiresAt      time.Time `json:"expires_at"`
	CertificatePEM string    `json:"certificate_pem"`
	KeyPEM         string    `json:"key_pem"`
	Revoked        bool      `json:"revoked,omitempty"`
	RevokedAt      time.Time `json:"revoked_at,omitempty"`
}

// LeafCertificate describes node certificates issued by the CA.
// Uses domain type (NodeID) for type-safe identification.
type LeafCertificate struct {
	NodeID          domaintypes.NodeID `json:"node_id"` // Node ID (NanoID-backed)
	Usage           string             `json:"usage"`
	Version         string             `json:"version"`
	ParentVersion   string             `json:"parent_version"`
	SerialNumber    string             `json:"serial_number"`
	CertificatePEM  string             `json:"certificate_pem"`
	KeyPEM          string             `json:"key_pem"`
	IssuedAt        time.Time          `json:"issued_at"`
	ExpiresAt       time.Time          `json:"expires_at"`
	PreviousVersion string             `json:"previous_version,omitempty"`
}

// RevokedRecord records a revoked CA version.
type RevokedRecord struct {
	Version   string    `json:"version"`
	Serial    string    `json:"serial_number"`
	RevokedAt time.Time `json:"revoked_at"`
}

// RotateResult summarises a rotation request.
type RotateResult struct {
	DryRun                    bool
	OldVersion                string
	NewVersion                string
	UpdatedBeaconCertificates []LeafCertificate
	UpdatedWorkerCertificates []LeafCertificate
	Revoked                   RevokedRecord
	State                     CAState
	Operator                  string
	Reason                    string
}

// NodeSet lists tracked beacon and worker identifiers.
type NodeSet struct {
	Beacons []string `json:"beacons"`
	Workers []string `json:"workers"`
}
