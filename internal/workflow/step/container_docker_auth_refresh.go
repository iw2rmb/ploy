package step

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"strings"
	"time"
)

type registryAuthRefreshRequest struct {
	RegistryHost    string `json:"registry_host"`
	ValidationImage string `json:"validation_image"`
}

type registryAuthRefreshResponse struct {
	OK    bool   `json:"ok"`
	Error string `json:"error,omitempty"`
}

func (r *DockerContainerRuntime) refreshRegistryAuth(ctx context.Context, imageRef string) error {
	socketPath := strings.TrimSpace(r.opts.RegistryAuthRefreshSocket)
	if socketPath == "" {
		return nil
	}

	var dialer net.Dialer
	conn, err := dialer.DialContext(ctx, "unix", socketPath)
	if err != nil {
		return fmt.Errorf("connect auth refresh socket %q: %w", socketPath, err)
	}
	defer func() { _ = conn.Close() }()
	_ = conn.SetDeadline(time.Now().Add(2 * time.Minute))

	req := registryAuthRefreshRequest{
		RegistryHost:    imageRegistryHost(imageRef),
		ValidationImage: imageRef,
	}
	if err := json.NewEncoder(conn).Encode(req); err != nil {
		return fmt.Errorf("send auth refresh request: %w", err)
	}

	var resp registryAuthRefreshResponse
	if err := json.NewDecoder(conn).Decode(&resp); err != nil {
		return fmt.Errorf("read auth refresh response: %w", err)
	}
	if !resp.OK {
		if strings.TrimSpace(resp.Error) == "" {
			return fmt.Errorf("auth refresh rejected")
		}
		return fmt.Errorf("%s", strings.TrimSpace(resp.Error))
	}
	return nil
}
