package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"time"
)

// rolloutNodesAPIClient allows tests to inject a mock HTTP client and base URL.
var rolloutNodesAPIClient *http.Client
var rolloutNodesAPIBaseURL string

type nodeInfo struct {
	ID        string
	Name      string
	IPAddress string
	Drained   bool
}

type nodeDetail struct {
	ID            string
	Name          string
	IPAddress     string
	Drained       bool
	LastHeartbeat string
}

func resolveAPIClientAndURL(ctx context.Context) (string, *http.Client, error) {
	// Use test stubs if available.
	if rolloutNodesAPIClient != nil && rolloutNodesAPIBaseURL != "" {
		return rolloutNodesAPIBaseURL, rolloutNodesAPIClient, nil
	}

	// Use the standard control plane HTTP resolver.
	u, client, err := resolveControlPlaneHTTP(ctx)
	if err != nil {
		return "", nil, err
	}

	return u.String(), client, nil
}

func fetchNodes(ctx context.Context) ([]nodeInfo, error) {
	baseURL, client, err := resolveAPIClientAndURL(ctx)
	if err != nil {
		return nil, err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, baseURL+"/v1/nodes", nil)
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}

	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("fetch nodes: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return nil, controlPlaneHTTPError(resp)
	}

	var apiNodes []struct {
		ID        string `json:"id"`
		Name      string `json:"name"`
		IPAddress string `json:"ip_address"`
		Drained   bool   `json:"drained"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&apiNodes); err != nil {
		return nil, fmt.Errorf("decode response: %w", err)
	}

	nodes := make([]nodeInfo, len(apiNodes))
	for i, n := range apiNodes {
		nodes[i] = nodeInfo{
			ID:        n.ID,
			Name:      n.Name,
			IPAddress: n.IPAddress,
			Drained:   n.Drained,
		}
	}

	return nodes, nil
}

func drainNode(ctx context.Context, nodeID string) error {
	baseURL, client, err := resolveAPIClientAndURL(ctx)
	if err != nil {
		return err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, fmt.Sprintf("%s/v1/nodes/%s/drain", baseURL, nodeID), nil)
	if err != nil {
		return fmt.Errorf("create drain request: %w", err)
	}

	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("drain request: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusNoContent && resp.StatusCode != http.StatusConflict {
		return controlPlaneHTTPError(resp)
	}

	return nil
}

func undrainNode(ctx context.Context, nodeID string) error {
	baseURL, client, err := resolveAPIClientAndURL(ctx)
	if err != nil {
		return err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, fmt.Sprintf("%s/v1/nodes/%s/undrain", baseURL, nodeID), nil)
	if err != nil {
		return fmt.Errorf("create undrain request: %w", err)
	}

	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("undrain request: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusNoContent && resp.StatusCode != http.StatusConflict {
		return controlPlaneHTTPError(resp)
	}

	return nil
}

func waitForNodeIdle(ctx context.Context, nodeID string) error {
	// Poll until the node has no active runs.
	// For simplicity, we implement a basic polling loop with a short sleep.
	// In a real implementation, this would query the node's active runs from the API.
	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()

	// For now, we just wait a short duration as a placeholder.
	// The actual implementation would query GET /v1/nodes/{id} and check active run count.
	select {
	case <-ctx.Done():
		return fmt.Errorf("timeout waiting for node to be idle")
	case <-ticker.C:
		// Assume node is idle after one tick (placeholder).
		return nil
	}
}

func waitForNodeHeartbeat(ctx context.Context, nodeID string, logger *slog.Logger, metrics *RolloutMetrics) error {
	// Poll until the node sends a heartbeat after restart with exponential backoff.
	policy := DefaultRetryPolicy()
	policy.MaxAttempts = 15

	return PollWithBackoff(ctx, policy, logger, metrics, "node_heartbeat_poll", func() (bool, error) {
		node, err := fetchNodeByID(ctx, nodeID)
		if err != nil {
			// Continue polling on transient errors.
			return false, nil
		}
		if node.LastHeartbeat != "" {
			// Parse heartbeat timestamp and check if recent (within last 10 seconds).
			hb, err := time.Parse(time.RFC3339, node.LastHeartbeat)
			if err == nil && time.Since(hb) < 10*time.Second {
				return true, nil
			}
		}
		return false, nil
	})
}

func fetchNodeByID(ctx context.Context, nodeID string) (*nodeDetail, error) {
	baseURL, client, err := resolveAPIClientAndURL(ctx)
	if err != nil {
		return nil, err
	}

	// Note: The API doesn't have a GET /v1/nodes/{id} endpoint yet,
	// so we fetch all nodes and filter. In a production implementation,
	// this endpoint should be added.
	nodes, err := fetchNodes(ctx)
	if err != nil {
		return nil, err
	}

	for _, n := range nodes {
		if n.ID == nodeID {
			// Fetch detailed node info by calling the list endpoint and parsing.
			// For now, return a minimal detail struct.
			req, err := http.NewRequestWithContext(ctx, http.MethodGet, baseURL+"/v1/nodes", nil)
			if err != nil {
				return nil, fmt.Errorf("create request: %w", err)
			}

			resp, err := client.Do(req)
			if err != nil {
				return nil, fmt.Errorf("fetch node: %w", err)
			}
			defer func() { _ = resp.Body.Close() }()

			if resp.StatusCode != http.StatusOK {
				return nil, controlPlaneHTTPError(resp)
			}

			var apiNodes []struct {
				ID            string  `json:"id"`
				Name          string  `json:"name"`
				IPAddress     string  `json:"ip_address"`
				Drained       bool    `json:"drained"`
				LastHeartbeat *string `json:"last_heartbeat,omitempty"`
			}

			if err := json.NewDecoder(resp.Body).Decode(&apiNodes); err != nil {
				return nil, fmt.Errorf("decode response: %w", err)
			}

			for _, an := range apiNodes {
				if an.ID == nodeID {
					detail := &nodeDetail{
						ID:        an.ID,
						Name:      an.Name,
						IPAddress: an.IPAddress,
						Drained:   an.Drained,
					}
					if an.LastHeartbeat != nil {
						detail.LastHeartbeat = *an.LastHeartbeat
					}
					return detail, nil
				}
			}
		}
	}

	return nil, fmt.Errorf("node %s not found", nodeID)
}
