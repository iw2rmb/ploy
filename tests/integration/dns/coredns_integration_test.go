//go:build integration

package integration

import (
	"errors"
	"net"
	"os"
	"syscall"
	"testing"
	"time"

	"github.com/miekg/dns"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/iw2rmb/ploy/internal/testing/helpers"
)

func TestCoreDNSAuthoritativeRecords(t *testing.T) {
	t.Helper()

	addr := helpers.GetEnvOrDefault("PLOY_COREDNS_ADDR", "127.0.0.1:1053")
	client := &dns.Client{Timeout: 2 * time.Second, UDPSize: 4096}

	probe := new(dns.Msg)
	probe.SetQuestion(dns.Fqdn("nomad.control.ploy.local."), dns.TypeA)
	if _, _, err := client.Exchange(probe, addr); err != nil {
		if isConnectionRefused(err) {
			t.Skipf("CoreDNS endpoint %s not reachable: %v", addr, err)
		}
		t.Fatalf("CoreDNS probe failed: %v", err)
	}

	t.Run("nomad control A record", func(t *testing.T) {
		query := new(dns.Msg)
		query.SetQuestion(dns.Fqdn("nomad.control.ploy.local."), dns.TypeA)

		resp, _, err := client.Exchange(query, addr)
		require.NoError(t, err, "CoreDNS query failed")
		require.Equal(t, dns.RcodeSuccess, resp.Rcode, "unexpected dns response code")
		require.NotEmpty(t, resp.Answer, "expected at least one A record")

		for _, ans := range resp.Answer {
			arec, ok := ans.(*dns.A)
			if !ok {
				continue
			}
			assert.NotEmpty(t, arec.A.String(), "IPv4 address should be populated")
			ttl := arec.Hdr.Ttl
			assert.Greater(t, ttl, uint32(0), "TTL must be > 0")
			assert.LessOrEqual(t, ttl, uint32(300), "TTL should remain bounded to avoid long caches")
		}
	})

	t.Run("seaweedfs filer SRV", func(t *testing.T) {
		query := new(dns.Msg)
		query.SetQuestion(dns.Fqdn("_seaweedfs._tcp.seaweedfs-filer.storage.ploy.local."), dns.TypeSRV)

		resp, _, err := client.Exchange(query, addr)
		require.NoError(t, err, "CoreDNS query failed")
		require.Equal(t, dns.RcodeSuccess, resp.Rcode, "unexpected dns response code")
		require.NotEmpty(t, resp.Answer, "expected SRV records for seaweedfs")

		for _, ans := range resp.Answer {
			srv, ok := ans.(*dns.SRV)
			if !ok {
				continue
			}
			ttl := srv.Hdr.Ttl
			assert.Greater(t, ttl, uint32(0), "TTL must be > 0")
			assert.LessOrEqual(t, ttl, uint32(300), "TTL should remain bounded to avoid long caches")
			assert.NotZero(t, srv.Port, "SRV port must be populated")
			assert.NotEmpty(t, srv.Target, "SRV target must be populated")
		}
	})

	t.Run("retry behaviour", func(t *testing.T) {
		query := new(dns.Msg)
		query.SetQuestion(dns.Fqdn("nomad.control.ploy.local."), dns.TypeA)

		var lastErr error
		for i := 0; i < 3; i++ {
			if resp, _, err := client.Exchange(query, addr); err == nil && len(resp.Answer) > 0 {
				return
			} else {
				lastErr = err
				time.Sleep(100 * time.Millisecond)
			}
		}
		t.Fatalf("CoreDNS query failed after retries: %v", lastErr)
	})
}

func isConnectionRefused(err error) bool {
	var opErr *net.OpError
	if errors.As(err, &opErr) {
		if opErr.Err == syscall.ECONNREFUSED {
			return true
		}
		var sysErr *os.SyscallError
		if errors.As(opErr.Err, &sysErr) && sysErr.Err == syscall.ECONNREFUSED {
			return true
		}
	}
	return errors.Is(err, syscall.ECONNREFUSED)
}
