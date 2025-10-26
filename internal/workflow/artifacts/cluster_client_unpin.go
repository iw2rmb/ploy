// cluster_client_unpin.go handles pin removal helpers.
package artifacts

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
)

// Unpin removes an artifact pin from the cluster.
func (c *ClusterClient) Unpin(ctx context.Context, cid string) error {
	if c == nil {
		return errors.New("artifacts: cluster client not configured")
	}
	trimmed := strings.TrimSpace(cid)
	if trimmed == "" {
		return errors.New("artifacts: cid required for unpin")
	}

	endpoint := c.resolve("/pins/" + trimmed)
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodDelete, endpoint.String(), nil)
	if err != nil {
		return fmt.Errorf("artifacts: build unpin request: %w", err)
	}
	c.applyAuth(httpReq)

	resp, err := c.http.Do(httpReq)
	if err != nil {
		return fmt.Errorf("artifacts: unpin request: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusAccepted && resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusNoContent {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		return fmt.Errorf("artifacts: unpin failed: status %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}
	return nil
}
