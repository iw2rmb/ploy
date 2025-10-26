// cluster_client_pin.go handles explicit re-pin requests.
package artifacts

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
)

// Pin ensures the provided CID stays pinned with the requested replication factors.
func (c *ClusterClient) Pin(ctx context.Context, cid string, opts PinOptions) error {
	if c == nil {
		return errors.New("artifacts: cluster client not configured")
	}
	trimmed := strings.TrimSpace(cid)
	if trimmed == "" {
		return errors.New("artifacts: cid required for pin")
	}
	endpoint := c.resolve("/pins/" + trimmed)

	var body bytes.Buffer
	if opts.ReplicationFactorMin > 0 || opts.ReplicationFactorMax > 0 {
		payload := map[string]int{}
		if opts.ReplicationFactorMin > 0 {
			payload["replication_factor_min"] = opts.ReplicationFactorMin
		}
		if opts.ReplicationFactorMax > 0 {
			payload["replication_factor_max"] = opts.ReplicationFactorMax
		}
		if err := json.NewEncoder(&body).Encode(payload); err != nil {
			return fmt.Errorf("artifacts: encode pin payload: %w", err)
		}
	}

	method := http.MethodPost
	req, err := http.NewRequestWithContext(ctx, method, endpoint.String(), bytes.NewReader(body.Bytes()))
	if err != nil {
		return fmt.Errorf("artifacts: build pin request: %w", err)
	}
	if body.Len() > 0 {
		req.Header.Set("Content-Type", "application/json")
	}
	c.applyAuth(req)

	resp, err := c.http.Do(req)
	if err != nil {
		return fmt.Errorf("artifacts: pin request: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode >= 300 {
		return fmt.Errorf("artifacts: pin request failed: status %d", resp.StatusCode)
	}
	return nil
}
