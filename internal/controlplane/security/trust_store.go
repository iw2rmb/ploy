package security

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	clientv3 "go.etcd.io/etcd/client/v3"
)

// ErrTrustBundleNotFound is returned when the trust bundle has not been written yet.
var ErrTrustBundleNotFound = errors.New("trust bundle not found")

// TrustBundle captures the active CA bundle exposed to nodes.
type TrustBundle struct {
	Version      string    `json:"version"`
	UpdatedAt    time.Time `json:"updated_at"`
	CABundlePEM  string    `json:"ca_bundle_pem"`
	CABundleHash string    `json:"ca_bundle_hash"`
}

// TrustStore manages trust bundle state inside etcd.
type TrustStore struct {
	client *clientv3.Client
	prefix string
}

const trustStoreRoot = "/ploy/clusters"

// NewTrustStore constructs a TrustStore scoped to the provided cluster identifier.
func NewTrustStore(client *clientv3.Client, clusterID string) (*TrustStore, error) {
	if client == nil {
		return nil, errors.New("trust store: etcd client required")
	}
	trimmed := strings.TrimSpace(clusterID)
	if trimmed == "" {
		return nil, errors.New("trust store: cluster id required")
	}
	prefix := fmt.Sprintf("%s/%s/security/trust-store", trustStoreRoot, trimmed)
	return &TrustStore{client: client, prefix: prefix}, nil
}

func (s *TrustStore) bundleKey() string {
	return s.prefix + "/bundle"
}

// Update records the supplied trust bundle.
func (s *TrustStore) Update(ctx context.Context, bundle TrustBundle) error {
	if strings.TrimSpace(bundle.Version) == "" {
		return errors.New("trust store: bundle version required")
	}
	if bundle.UpdatedAt.IsZero() {
		bundle.UpdatedAt = time.Now().UTC()
	}
	if strings.TrimSpace(bundle.CABundlePEM) == "" {
		return errors.New("trust store: CA bundle PEM required")
	}
	if strings.TrimSpace(bundle.CABundleHash) == "" {
		sum := sha256.Sum256([]byte(bundle.CABundlePEM))
		bundle.CABundleHash = hex.EncodeToString(sum[:])
	}
	payload, err := json.Marshal(bundle)
	if err != nil {
		return fmt.Errorf("trust store: encode bundle: %w", err)
	}
	if _, err := s.client.Put(ctx, s.bundleKey(), string(payload)); err != nil {
		return fmt.Errorf("trust store: write bundle: %w", err)
	}
	return nil
}

// Current returns the active trust bundle and its etcd revision.
func (s *TrustStore) Current(ctx context.Context) (TrustBundle, int64, error) {
	resp, err := s.client.Get(ctx, s.bundleKey())
	if err != nil {
		return TrustBundle{}, 0, fmt.Errorf("trust store: read bundle: %w", err)
	}
	if len(resp.Kvs) == 0 {
		return TrustBundle{}, 0, ErrTrustBundleNotFound
	}
	var bundle TrustBundle
	if err := json.Unmarshal(resp.Kvs[0].Value, &bundle); err != nil {
		return TrustBundle{}, 0, fmt.Errorf("trust store: decode bundle: %w", err)
	}
	return bundle, resp.Kvs[0].ModRevision, nil
}
