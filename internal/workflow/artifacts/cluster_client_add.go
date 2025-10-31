// cluster_client_add.go contains IPFS Cluster upload helpers for ClusterClient.
package artifacts

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"strconv"
	"strings"
)

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
        // IPFS Cluster expects replication_factor_min in /add
        query.Set("replication_factor_min", strconv.Itoa(replMin))
    }
    replMax := firstNonZero(req.ReplicationFactorMax, c.defaultReplMax)
    if replMax != 0 {
        // IPFS Cluster expects replication_factor_max in /add
        query.Set("replication_factor_max", strconv.Itoa(replMax))
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

// addResponseMeta stores the subset of fields required from the cluster add response stream.
type addResponseMeta struct {
	cid  string
	name string
	size int64
}

// parseAddResponse extracts the first valid metadata object from the multipart add response.
func parseAddResponse(payload []byte) (addResponseMeta, error) {
    // Some IPFS Cluster versions return a single JSON array with one object
    // instead of a newline-delimited stream. Handle that format first.
    trimmed := bytes.TrimSpace(payload)
    if len(trimmed) > 0 && trimmed[0] == '[' {
        var arr []map[string]any
        if err := json.Unmarshal(trimmed, &arr); err == nil {
            for _, raw := range arr {
                cid := parseCID(raw)
                if cid == "" { continue }
                name := firstNonEmpty(asString(raw["Name"]), asString(raw["name"]))
                size := parseSize(raw["Size"], raw["Bytes"])
                return addResponseMeta{cid: cid, name: name, size: size}, nil
            }
        }
    }
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

// parseCID pulls the CID string out of any of the cluster response variants.
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

// parseSize converts the size or byte count fields to an int64 when present.
func parseSize(sizeVal any, bytesVal any) int64 {
	if val := toInt64(sizeVal); val > 0 {
		return val
	}
	if val := toInt64(bytesVal); val > 0 {
		return val
	}
	return 0
}
