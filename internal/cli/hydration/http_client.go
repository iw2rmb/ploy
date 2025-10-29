package hydration

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// HTTPClient implements the hydration CLI client over the control-plane HTTP API.
type HTTPClient struct {
	BaseURL    *url.URL
	HTTPClient *http.Client
}

// Inspect fetches the hydration policy for the provided ticket.
func (c HTTPClient) Inspect(ctx context.Context, ticket string) (Policy, error) {
	ticket = strings.TrimSpace(ticket)
	if ticket == "" {
		return Policy{}, errors.New("hydration: ticket required")
	}
	endpoint, err := c.resolve("v1", "mods", ticket, "hydration")
	if err != nil {
		return Policy{}, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint.String(), nil)
	if err != nil {
		return Policy{}, err
	}
	resp, err := c.httpClient().Do(req)
	if err != nil {
		return Policy{}, err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		return Policy{}, decodeHTTPError(resp)
	}
	var payload hydrationResponse
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return Policy{}, err
	}
	return payload.Policy.toCLI(ticket)
}

// Tune updates the hydration policy using PATCH semantics.
func (c HTTPClient) Tune(ctx context.Context, ticket string, req TuneRequest) (Policy, error) {
	ticket = strings.TrimSpace(ticket)
	if ticket == "" {
		return Policy{}, errors.New("hydration: ticket required")
	}
	endpoint, err := c.resolve("v1", "mods", ticket, "hydration")
	if err != nil {
		return Policy{}, err
	}
	body := make(map[string]any)
	if trimmed := strings.TrimSpace(req.TTL); trimmed != "" {
		body["ttl"] = trimmed
	}
	if req.ReplicationMin != nil {
		body["replication_min"] = *req.ReplicationMin
	}
	if req.ReplicationMax != nil {
		body["replication_max"] = *req.ReplicationMax
	}
	if req.Share != nil {
		body["share"] = *req.Share
	}
	payload, err := json.Marshal(body)
	if err != nil {
		return Policy{}, err
	}
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPatch, endpoint.String(), bytes.NewReader(payload))
	if err != nil {
		return Policy{}, err
	}
	httpReq.Header.Set("Content-Type", "application/json")
	resp, err := c.httpClient().Do(httpReq)
	if err != nil {
		return Policy{}, err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		return Policy{}, decodeHTTPError(resp)
	}
	var payloadResp hydrationResponse
	if err := json.NewDecoder(resp.Body).Decode(&payloadResp); err != nil {
		return Policy{}, err
	}
	return payloadResp.Policy.toCLI(ticket)
}

func (c HTTPClient) resolve(segments ...string) (*url.URL, error) {
	if c.BaseURL == nil {
		return nil, errors.New("hydration: base url required")
	}
	path := c.BaseURL.String()
	endpoint, err := url.JoinPath(path, segments...)
	if err != nil {
		return nil, err
	}
	return url.Parse(endpoint)
}

func (c HTTPClient) httpClient() *http.Client {
	if c.HTTPClient != nil {
		return c.HTTPClient
	}
	return http.DefaultClient
}

type hydrationResponse struct {
	Policy hydrationDTO `json:"hydration"`
}

type hydrationDTO struct {
	SharedCID      string              `json:"shared_cid"`
	TTL            string              `json:"ttl"`
	ReplicationMin int                 `json:"replication_min"`
	ReplicationMax int                 `json:"replication_max"`
	Share          bool                `json:"share"`
	ExpiresAt      string              `json:"expires_at"`
	RepoURL        string              `json:"repo_url"`
	Revision       string              `json:"revision"`
	Candidates     []string            `json:"reuse_candidates"`
	Global         *hydrationGlobalDTO `json:"global"`
}

type hydrationGlobalDTO struct {
	PolicyID           string            `json:"policy_id"`
	PinnedBytes        hydrationByteDTO  `json:"pinned_bytes"`
	Snapshots          hydrationCountDTO `json:"snapshots"`
	Replicas           hydrationCountDTO `json:"replicas"`
	ActiveFingerprints []string          `json:"active_fingerprints"`
}

type hydrationByteDTO struct {
	Used int64 `json:"used"`
	Soft int64 `json:"soft"`
	Hard int64 `json:"hard"`
}

type hydrationCountDTO struct {
	Used int `json:"used"`
	Soft int `json:"soft"`
	Hard int `json:"hard"`
}

func (dto hydrationDTO) toCLI(ticket string) (Policy, error) {
	expires := time.Time{}
	trimmed := strings.TrimSpace(dto.ExpiresAt)
	if trimmed != "" {
		parsed, err := time.Parse(time.RFC3339Nano, trimmed)
		if err != nil {
			return Policy{}, err
		}
		expires = parsed
	}
	policy := Policy{
		Ticket:          ticket,
		SharedCID:       strings.TrimSpace(dto.SharedCID),
		TTL:             strings.TrimSpace(dto.TTL),
		ReplicationMin:  dto.ReplicationMin,
		ReplicationMax:  dto.ReplicationMax,
		Share:           dto.Share,
		ExpiresAt:       expires,
		RepoURL:         strings.TrimSpace(dto.RepoURL),
		Revision:        strings.TrimSpace(dto.Revision),
		ReuseCandidates: append([]string(nil), dto.Candidates...),
	}
	if dto.Global != nil {
		policy.Global = &GlobalPolicy{
			PolicyID: dto.Global.PolicyID,
			PinnedBytes: ByteUsage{
				Used: dto.Global.PinnedBytes.Used,
				Soft: dto.Global.PinnedBytes.Soft,
				Hard: dto.Global.PinnedBytes.Hard,
			},
			Snapshots: CountUsage{
				Used: dto.Global.Snapshots.Used,
				Soft: dto.Global.Snapshots.Soft,
				Hard: dto.Global.Snapshots.Hard,
			},
			Replicas: CountUsage{
				Used: dto.Global.Replicas.Used,
				Soft: dto.Global.Replicas.Soft,
				Hard: dto.Global.Replicas.Hard,
			},
			ActiveFingerprints: append([]string(nil), dto.Global.ActiveFingerprints...),
		}
	}
	return policy, nil
}

func decodeHTTPError(resp *http.Response) error {
	data := make(map[string]any)
	_ = json.NewDecoder(resp.Body).Decode(&data)
	if msg, ok := data["error"].(string); ok && strings.TrimSpace(msg) != "" {
		return fmt.Errorf("hydration: %s", strings.TrimSpace(msg))
	}
	return fmt.Errorf("hydration: http %d", resp.StatusCode)
}
