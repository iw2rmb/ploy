// cluster_client_fetch.go isolates the ClusterClient download path.
package artifacts

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
)

// Fetch downloads an artifact payload from the cluster proxy.
func (c *ClusterClient) Fetch(ctx context.Context, cid string) (FetchResult, error) {
	if c == nil {
		return FetchResult{}, errors.New("artifacts: cluster client not configured")
	}
	trimmed := strings.TrimSpace(cid)
	if trimmed == "" {
		return FetchResult{}, errors.New("artifacts: cid required")
	}

    endpoint := c.resolveFetch("/ipfs/" + trimmed)
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint.String(), nil)
	if err != nil {
		return FetchResult{}, fmt.Errorf("artifacts: build fetch request: %w", err)
	}
	c.applyAuth(httpReq)

	resp, err := c.http.Do(httpReq)
	if err != nil {
		return FetchResult{}, fmt.Errorf("artifacts: fetch request: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return FetchResult{}, fmt.Errorf("artifacts: read fetch response: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return FetchResult{}, fmt.Errorf("artifacts: fetch failed: status %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}

	digest := sha256.Sum256(body)
	return FetchResult{
		CID:    trimmed,
		Data:   body,
		Size:   int64(len(body)),
		Digest: "sha256:" + hex.EncodeToString(digest[:]),
	}, nil
}
