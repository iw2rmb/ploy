// cluster_client_status.go keeps replication status helpers for ClusterClient.
package artifacts

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
)

// Status retrieves the replication status of a pinned artifact.
func (c *ClusterClient) Status(ctx context.Context, cid string) (StatusResult, error) {
	if c == nil {
		return StatusResult{}, errors.New("artifacts: cluster client not configured")
	}
	trimmed := strings.TrimSpace(cid)
	if trimmed == "" {
		return StatusResult{}, errors.New("artifacts: cid required for status")
	}

	endpoint := c.resolve("/pins/" + trimmed)
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint.String(), nil)
	if err != nil {
		return StatusResult{}, fmt.Errorf("artifacts: build status request: %w", err)
	}
	c.applyAuth(httpReq)

	resp, err := c.http.Do(httpReq)
	if err != nil {
		return StatusResult{}, fmt.Errorf("artifacts: status request: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return StatusResult{}, fmt.Errorf("artifacts: read status response: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return StatusResult{}, fmt.Errorf("artifacts: status failed: status %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}

	parsed, err := parseStatusResponse(body)
	if err != nil {
		return StatusResult{}, err
	}
	return parsed, nil
}

// parseStatusResponse converts the cluster JSON payload into a StatusResult.
func parseStatusResponse(payload []byte) (StatusResult, error) {
	var resp struct {
		CID struct {
			Path string `json:"/"`
		} `json:"cid"`
		Name       string `json:"name"`
		PinOptions struct {
			ReplicationFactorMin int `json:"replication_factor_min"`
			ReplicationFactorMax int `json:"replication_factor_max"`
		} `json:"pin_options"`
		Status struct {
			Summary string `json:"summary"`
			Peers   map[string]struct {
				Status string `json:"status"`
			} `json:"peers"`
			PeerMap map[string]struct {
				Status string `json:"status"`
			} `json:"peer_map"`
		} `json:"status"`
	}
	if err := json.Unmarshal(payload, &resp); err != nil {
		return StatusResult{}, fmt.Errorf("artifacts: parse status response: %w", err)
	}
	result := StatusResult{
		CID:                  strings.TrimSpace(resp.CID.Path),
		Name:                 strings.TrimSpace(resp.Name),
		Summary:              strings.TrimSpace(resp.Status.Summary),
		ReplicationFactorMin: resp.PinOptions.ReplicationFactorMin,
		ReplicationFactorMax: resp.PinOptions.ReplicationFactorMax,
	}
	peers := make([]StatusPeer, 0, len(resp.Status.Peers)+len(resp.Status.PeerMap))
	for id, peer := range resp.Status.Peers {
		peers = append(peers, StatusPeer{
			PeerID: strings.TrimSpace(id),
			Status: strings.TrimSpace(peer.Status),
		})
	}
	for id, peer := range resp.Status.PeerMap {
		peers = append(peers, StatusPeer{
			PeerID: strings.TrimSpace(id),
			Status: strings.TrimSpace(peer.Status),
		})
	}
	result.Peers = peers
	return result, nil
}
