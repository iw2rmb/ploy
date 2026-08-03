package handlers

import (
	"context"
	"fmt"

	"github.com/iw2rmb/ploy/internal/server/blobpersist"
	"github.com/iw2rmb/ploy/internal/store"
)

type runSpecBundleStore struct {
	store store.Store
	blobs *blobpersist.Service
}

func (s runSpecBundleStore) Ensure(ctx context.Context, cid string, archive []byte) (string, error) {
	computedCID, digest := computeCIDAndDigest(archive)
	if cid != computedCID {
		return "", fmt.Errorf("spec bundle cid %q does not match content cid %q", cid, computedCID)
	}
	bundle, _, err := ensureSpecBundle(ctx, s.store, s.blobs, cid, digest, archive, nil)
	if err != nil {
		return "", err
	}
	return bundle.ID, nil
}
