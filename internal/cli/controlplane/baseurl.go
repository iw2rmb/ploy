package controlplane

import (
	"fmt"
	"net"
	"net/url"
	"strings"
)

// BaseURLFromServerURL converts a raw PLOY_SERVER_URL value into a control-plane base URL.
// Rules:
//   - If serverURL already includes a scheme, return it unchanged.
//   - Otherwise, use http and port 8080 when no port is present.
func BaseURLFromServerURL(serverURL string) (string, error) {
	address := strings.TrimSpace(serverURL)
	if address == "" {
		return "", fmt.Errorf("PLOY_SERVER_URL is required")
	}
	// Pass through full URLs.
	if strings.HasPrefix(address, "http://") || strings.HasPrefix(address, "https://") {
		return address, nil
	}
	host := address
	// If no port in address, default to 8080
	if _, _, err := net.SplitHostPort(address); err != nil {
		host = net.JoinHostPort(address, "8080")
	}
	u := url.URL{Scheme: "http", Host: host}
	return u.String(), nil
}
