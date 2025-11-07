package config

import (
	"errors"
	"fmt"
	"net"
	"net/netip"
	"strconv"
	"strings"
)

// ValidateAddress parses and validates a TCP listen address of the form
// "host:port", "ip:port", "[ipv6]:port" or ":port". It accepts port 0
// (ephemeral port). When the host component is empty (e.g. ":8443"), the
// returned address uses the unspecified IPv6 address ("::").
func ValidateAddress(s string) (netip.AddrPort, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return netip.AddrPort{}, errors.New("invalid address: empty string")
	}

	host, portStr, err := net.SplitHostPort(s)
	if err != nil {
		return netip.AddrPort{}, fmt.Errorf("invalid address %q: %w", s, err)
	}

	// Parse port allowing 0..65535.
	p, err := strconv.ParseUint(portStr, 10, 16)
	if err != nil {
		return netip.AddrPort{}, fmt.Errorf("invalid port %q: %w", portStr, err)
	}

	var addr netip.Addr
	if host == "" {
		// Use IPv6 unspecified (::) to represent an empty host (":PORT").
		addr = netip.AddrFrom16([16]byte{})
	} else {
		a, perr := netip.ParseAddr(host)
		if perr != nil {
			return netip.AddrPort{}, fmt.Errorf("invalid ip %q: %w", host, perr)
		}
		addr = a
	}

	return netip.AddrPortFrom(addr, uint16(p)), nil
}
