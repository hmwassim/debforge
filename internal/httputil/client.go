// Package httputil provides a hardened HTTP client for debforge that
// blocks private IP ranges, limits redirects, and enforces TLS 1.2+.
package httputil

import (
	"context"
	"crypto/tls"
	"fmt"
	"net"
	"net/http"
	"strings"
	"time"
)

// MaxDownloadSize is the maximum allowed download size (2 GiB).
const MaxDownloadSize = 2 << 30

// NewClient returns an HTTP client hardened for fetching untrusted
// remote resources as root. It enforces:
//   - 5 minute total request timeout
//   - Maximum 3 redirects
//   - TLS 1.2 minimum
//   - Blocking of private/link-local/loopback IPs
func NewClient() *http.Client {
	return &http.Client{
		Timeout: 5 * time.Minute,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if len(via) >= 3 {
				return fmt.Errorf("too many redirects (max 3)")
			}
			return nil
		},
		Transport: &http.Transport{
			DialContext:           hardenedDialContext,
			TLSClientConfig:      &tls.Config{MinVersion: tls.VersionTLS12},
			MaxIdleConns:          10,
			IdleConnTimeout:       30 * time.Second,
			TLSHandshakeTimeout:  10 * time.Second,
			ResponseHeaderTimeout: 30 * time.Second,
		},
	}
}

// NewVerifyClient returns a lightweight client for HEAD requests used
// in version tag verification. It enforces TLS 1.2+ and private IP
// blocking but uses a shorter timeout.
func NewVerifyClient() *http.Client {
	return &http.Client{
		Timeout: 15 * time.Second,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if len(via) >= 2 {
				return fmt.Errorf("too many redirects (max 2)")
			}
			return nil
		},
		Transport: &http.Transport{
			DialContext:           hardenedDialContext,
			TLSClientConfig:      &tls.Config{MinVersion: tls.VersionTLS12},
			TLSHandshakeTimeout:  5 * time.Second,
			ResponseHeaderTimeout: 10 * time.Second,
		},
	}
}

// hardenedDialContext resolves DNS, validates IPs against private
// ranges, then dials — preventing SSRF to internal networks.
func hardenedDialContext(ctx context.Context, network, addr string) (net.Conn, error) {
	host, port, err := net.SplitHostPort(addr)
	if err != nil {
		return nil, fmt.Errorf("split host port %q: %w", addr, err)
	}

	ips, err := net.DefaultResolver.LookupIPAddr(ctx, host)
	if err != nil {
		return nil, fmt.Errorf("resolve %q: %w", host, err)
	}
	if len(ips) == 0 {
		return nil, fmt.Errorf("no IPs resolved for %q", host)
	}

	for _, ip := range ips {
		if isBlockedIP(ip.IP) {
			return nil, fmt.Errorf("connection to %s blocked: private/link-local IP", ip.IP)
		}
	}

	var lastErr error
	for _, ip := range ips {
		dialer := &net.Dialer{Timeout: 10 * time.Second}
		conn, err := dialer.DialContext(ctx, network, net.JoinHostPort(ip.IP.String(), port))
		if err != nil {
			lastErr = err
			continue
		}
		return conn, nil
	}
	if lastErr != nil {
		return nil, lastErr
	}
	return nil, fmt.Errorf("failed to dial %s", addr)
}

// isBlockedIP returns true for loopback, link-local, and RFC 1918/4193
// private addresses that an attacker could use for SSRF.
func isBlockedIP(ip net.IP) bool {
	if ip.IsLoopback() {
		return true
	}
	if ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() {
		return true
	}
	if ip.IsUnspecified() {
		return true
	}

	if v4 := ip.To4(); v4 != nil {
		// 10.0.0.0/8
		if v4[0] == 10 {
			return true
		}
		// 172.16.0.0/12
		if v4[0] == 172 && v4[1] >= 16 && v4[1] <= 31 {
			return true
		}
		// 192.168.0.0/16
		if v4[0] == 192 && v4[1] == 168 {
			return true
		}
		// 169.254.0.0/16 (cloud metadata, link-local)
		if v4[0] == 169 && v4[1] == 254 {
			return true
		}
		// 127.0.0.0/8
		if v4[0] == 127 {
			return true
		}
	}

	if v6 := ip.To16(); v6 != nil {
		// fc00::/7 (unique local)
		if v6[0] == 0xfc || v6[0] == 0xfd {
			return true
		}
		// fe80::/10 (link-local)
		if v6[0] == 0xfe && (v6[1]&0xc0) == 0x80 {
			return true
		}
	}

	return false
}

// IsHTTPS reports whether url uses the HTTPS scheme.
func IsHTTPS(rawURL string) bool {
	return strings.HasPrefix(rawURL, "https://")
}
