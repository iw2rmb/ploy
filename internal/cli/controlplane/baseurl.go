package controlplane

import (
	"fmt"
	"net"
	"net/url"
	"strings"

	"github.com/iw2rmb/ploy/internal/cli/config"
)

// BaseURLFromDescriptor converts a descriptor into a control-plane base URL.
// Rules:
//   - If Address already includes a scheme, return it unchanged.
//   - Otherwise, use Scheme (default https) and port 8443 when no port present.
func BaseURLFromDescriptor(desc config.Descriptor) (string, error) {
	address := strings.TrimSpace(desc.Address)
	if address == "" {
		return "", fmt.Errorf("descriptor: address required")
	}
	// Pass through full URLs.
	if strings.HasPrefix(address, "http://") || strings.HasPrefix(address, "https://") {
		return address, nil
	}
	scheme := strings.TrimSpace(desc.Scheme)
	if scheme == "" {
		scheme = "https"
	}
	host := address
	// If no port in address, default to 8443
	if _, _, err := net.SplitHostPort(address); err != nil {
		host = net.JoinHostPort(address, "8443")
	}
	u := url.URL{Scheme: scheme, Host: host}
	return u.String(), nil
}
