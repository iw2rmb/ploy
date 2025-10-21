package artifacts

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

// ClusterClientOptions configures the IPFS Cluster client.
type ClusterClientOptions struct {
	BaseURL string

	AuthToken            string
	BasicAuthUsername    string
	BasicAuthPassword    string
	ReplicationFactorMin int
	ReplicationFactorMax int

	HTTPClient *http.Client
}

// ClusterClient provides helpers for interacting with an IPFS Cluster REST API.
type ClusterClient struct {
	base           *url.URL
	http           *http.Client
	authHeader     string
	defaultReplMin int
	defaultReplMax int
}

// NewClusterClient constructs an IPFS Cluster client with sane defaults.
func NewClusterClient(opts ClusterClientOptions) (*ClusterClient, error) {
	trimmed := strings.TrimSpace(opts.BaseURL)
	if trimmed == "" {
		return nil, errors.New("artifacts: cluster base url required")
	}
	parsed, err := url.Parse(trimmed)
	if err != nil {
		return nil, fmt.Errorf("artifacts: parse cluster base url: %w", err)
	}
	if parsed.Scheme == "" || parsed.Host == "" {
		return nil, errors.New("artifacts: cluster base url must include scheme and host")
	}
	sanitized := *parsed
	if sanitized.Path == "" {
		sanitized.Path = ""
	}
	sanitized.RawQuery = ""
	sanitized.Fragment = ""

	httpClient := opts.HTTPClient
	if httpClient == nil {
		httpClient = &http.Client{Timeout: 30 * time.Second}
	}

	authHeader := ""
	if token := strings.TrimSpace(opts.AuthToken); token != "" {
		authHeader = "Bearer " + token
	} else if strings.TrimSpace(opts.BasicAuthUsername) != "" || strings.TrimSpace(opts.BasicAuthPassword) != "" {
		creds := opts.BasicAuthUsername + ":" + opts.BasicAuthPassword
		authHeader = "Basic " + base64.StdEncoding.EncodeToString([]byte(creds))
	}

	return &ClusterClient{
		base:           &sanitized,
		http:           httpClient,
		authHeader:     authHeader,
		defaultReplMin: opts.ReplicationFactorMin,
		defaultReplMax: opts.ReplicationFactorMax,
	}, nil
}

// Add uploads an artifact payload to the cluster and returns the resulting pin metadata.
func (c *ClusterClient) Add(ctx context.Context, req AddRequest) (AddResponse, error) {
	if c == nil {
		return AddResponse{}, errors.New("artifacts: cluster client not configured")
	}
	if len(req.Payload) == 0 {
		return AddResponse{}, errors.New("artifacts: payload required")
	}
	name := strings.TrimSpace(req.Name)
	if name == "" {
		name = "artifact.bin"
	}
	digest := sha256.Sum256(req.Payload)
	digestValue := "sha256:" + hex.EncodeToString(digest[:])

	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)
	part, err := writer.CreateFormFile("file", name)
	if err != nil {
		return AddResponse{}, fmt.Errorf("artifacts: create multipart payload: %w", err)
	}
	if _, err := part.Write(req.Payload); err != nil {
		return AddResponse{}, fmt.Errorf("artifacts: write artifact payload: %w", err)
	}
	if err := writer.Close(); err != nil {
		return AddResponse{}, fmt.Errorf("artifacts: finalise multipart payload: %w", err)
	}

	endpoint := c.resolve("/add")
	query := endpoint.Query()
	query.Set("stream-channels", "false")
	replMin := firstNonZero(req.ReplicationFactorMin, c.defaultReplMin)
	if replMin != 0 {
		query.Set("repl_min", strconv.Itoa(replMin))
	}
	replMax := firstNonZero(req.ReplicationFactorMax, c.defaultReplMax)
	if replMax != 0 {
		query.Set("repl_max", strconv.Itoa(replMax))
	}
	if req.Local {
		query.Set("local", "true")
	}
	if req.Kind != "" {
		query.Set("tag", string(req.Kind))
	}
	query.Set("name", name)
	endpoint.RawQuery = query.Encode()

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint.String(), body)
	if err != nil {
		return AddResponse{}, fmt.Errorf("artifacts: build add request: %w", err)
	}
	httpReq.Header.Set("Content-Type", writer.FormDataContentType())
	c.applyAuth(httpReq)

	resp, err := c.http.Do(httpReq)
	if err != nil {
		return AddResponse{}, fmt.Errorf("artifacts: add request: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	payload, err := io.ReadAll(resp.Body)
	if err != nil {
		return AddResponse{}, fmt.Errorf("artifacts: read add response: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return AddResponse{}, fmt.Errorf("artifacts: add failed: status %d: %s", resp.StatusCode, strings.TrimSpace(string(payload)))
	}

	addMeta, err := parseAddResponse(payload)
	if err != nil {
		return AddResponse{}, err
	}

	return AddResponse{
		CID:                  addMeta.cid,
		Name:                 firstNonEmpty(addMeta.name, name),
		Size:                 addMeta.size,
		Digest:               digestValue,
		ReplicationFactorMin: replMin,
		ReplicationFactorMax: replMax,
	}, nil
}

// Fetch downloads an artifact payload from the cluster proxy.
func (c *ClusterClient) Fetch(ctx context.Context, cid string) (FetchResult, error) {
	if c == nil {
		return FetchResult{}, errors.New("artifacts: cluster client not configured")
	}
	trimmed := strings.TrimSpace(cid)
	if trimmed == "" {
		return FetchResult{}, errors.New("artifacts: cid required")
	}

	endpoint := c.resolve("/ipfs/" + trimmed)
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

func (c *ClusterClient) resolve(path string) *url.URL {
	relative := &url.URL{Path: path}
	return c.base.ResolveReference(relative)
}

func (c *ClusterClient) applyAuth(req *http.Request) {
	if c.authHeader != "" {
		req.Header.Set("Authorization", c.authHeader)
	}
}

type addResponseMeta struct {
	cid  string
	name string
	size int64
}

func parseAddResponse(payload []byte) (addResponseMeta, error) {
	lines := bytes.Split(payload, []byte("\n"))
	for _, line := range lines {
		line = bytes.TrimSpace(line)
		if len(line) == 0 {
			continue
		}
		var raw map[string]any
		if err := json.Unmarshal(line, &raw); err != nil {
			continue
		}
		cid := parseCID(raw)
		if cid == "" {
			continue
		}
		name := firstNonEmpty(asString(raw["Name"]), asString(raw["name"]))
		size := parseSize(raw["Size"], raw["Bytes"])
		return addResponseMeta{
			cid:  cid,
			name: name,
			size: size,
		}, nil
	}
	return addResponseMeta{}, fmt.Errorf("artifacts: add response missing cid: %s", strings.TrimSpace(string(payload)))
}

func parseCID(raw map[string]any) string {
	if hash := strings.TrimSpace(asString(raw["Hash"])); hash != "" {
		return hash
	}
	if cid := raw["Cid"]; cid != nil {
		switch value := cid.(type) {
		case string:
			if trimmed := strings.TrimSpace(value); trimmed != "" {
				return trimmed
			}
		case map[string]any:
			if nested := strings.TrimSpace(asString(value["/"])); nested != "" {
				return nested
			}
		}
	}
	if cid := raw["cid"]; cid != nil {
		switch value := cid.(type) {
		case string:
			if trimmed := strings.TrimSpace(value); trimmed != "" {
				return trimmed
			}
		case map[string]any:
			if nested := strings.TrimSpace(asString(value["/"])); nested != "" {
				return nested
			}
		}
	}
	return ""
}

func parseSize(sizeVal any, bytesVal any) int64 {
	if val := toInt64(sizeVal); val > 0 {
		return val
	}
	if val := toInt64(bytesVal); val > 0 {
		return val
	}
	return 0
}

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

func firstNonZero(values ...int) int {
	for _, value := range values {
		if value != 0 {
			return value
		}
	}
	return 0
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if trimmed := strings.TrimSpace(value); trimmed != "" {
			return trimmed
		}
	}
	return ""
}

func asString(value any) string {
	switch v := value.(type) {
	case string:
		return v
	case fmt.Stringer:
		return v.String()
	default:
		return ""
	}
}

func toInt64(value any) int64 {
	switch v := value.(type) {
	case string:
		if strings.TrimSpace(v) == "" {
			return 0
		}
		num, err := strconv.ParseInt(strings.TrimSpace(v), 10, 64)
		if err != nil {
			return 0
		}
		return num
	case float64:
		return int64(v)
	case json.Number:
		num, err := v.Int64()
		if err != nil {
			return 0
		}
		return num
	default:
		return 0
	}
}
