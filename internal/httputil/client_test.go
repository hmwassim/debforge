package httputil

import (
	"net"
	"net/http"
	"net/url"
	"testing"
)

func TestIsBlockedIP(t *testing.T) {
	tests := []struct {
		name string
		ip   net.IP
		want bool
	}{
		{"loopback v4", net.IPv4(127, 0, 0, 1), true},
		{"loopback v4 full", net.IPv4(127, 255, 255, 255), true},
		{"loopback v6", net.IPv6loopback, true},
		{"link-local unicast v4", net.IPv4(169, 254, 1, 1), true},
		{"link-local multicast v4", net.IPv4(224, 0, 0, 1).To4(), true},
		{"non-link-local multicast v4", net.IPv4(239, 255, 0, 1).To4(), false},
		{"RFC1918 10.x", net.IPv4(10, 0, 0, 1), true},
		{"RFC1918 10.x boundary", net.IPv4(10, 255, 255, 255), true},
		{"RFC1918 172.16.x", net.IPv4(172, 16, 0, 1), true},
		{"RFC1918 172.31.x", net.IPv4(172, 31, 255, 255), true},
		{"RFC1918 172.15.x (not blocked)", net.IPv4(172, 15, 0, 1), false},
		{"RFC1918 172.32.x (not blocked)", net.IPv4(172, 32, 0, 1), false},
		{"RFC1918 192.168.x", net.IPv4(192, 168, 1, 1), true},
		{"cloud metadata 169.254.x", net.IPv4(169, 254, 169, 254), true},
		{"unspecified v4", net.IPv4zero, true},
		{"unspecified v6", net.IPv6zero, true},
		{"public IP google", net.IPv4(8, 8, 8, 8), false},
		{"public IP cloudflare", net.IPv4(1, 1, 1, 1), false},
		{"v6 unique local fc00::", net.IP{0xfc, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 1}, true},
		{"v6 unique local fd00::", net.IP{0xfd, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 1}, true},
		{"v6 link-local fe80::", net.IP{0xfe, 0x80, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 1}, true},
		{"v6 public", net.IP{0x20, 0x01, 0x0d, 0xb8, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 1}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := isBlockedIP(tt.ip); got != tt.want {
				t.Errorf("isBlockedIP(%v) = %v, want %v", tt.ip, got, tt.want)
			}
		})
	}
}

func TestIsHTTPS(t *testing.T) {
	tests := []struct {
		name string
		url  string
		want bool
	}{
		{"https basic", "https://example.com/pkg.deb", true},
		{"https with path", "https://deb.debian.org/debian/pool/main/f/foo/foo_1.0_amd64.deb", true},
		{"http basic", "http://example.com/pkg.deb", false},
		{"ftp", "ftp://example.com/pkg.deb", false},
		{"empty", "", false},
		{"no scheme", "example.com", false},
		{"https uppercase", "HTTPS://example.com", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := IsHTTPS(tt.url); got != tt.want {
				t.Errorf("IsHTTPS(%q) = %v, want %v", tt.url, got, tt.want)
			}
		})
	}
}

func TestNewClient_redirectLimit(t *testing.T) {
	c := NewClient()
	if c.CheckRedirect == nil {
		t.Fatal("CheckRedirect is nil")
	}
	reqs := make([]*http.Request, 4)
	for i := range reqs {
		reqs[i] = &http.Request{URL: &url.URL{Scheme: "https", Host: "example.com", Path: "/"}}
	}
	err := c.CheckRedirect(reqs[3], reqs[:3])
	if err == nil {
		t.Error("expected error for 4th redirect (via has 3 entries)")
	}
}

func TestNewVerifyClient_redirectLimit(t *testing.T) {
	c := NewVerifyClient()
	if c.CheckRedirect == nil {
		t.Fatal("CheckRedirect is nil")
	}
	reqs := make([]*http.Request, 3)
	for i := range reqs {
		reqs[i] = &http.Request{URL: &url.URL{Scheme: "https", Host: "example.com", Path: "/"}}
	}
	err := c.CheckRedirect(reqs[2], reqs[:2])
	if err == nil {
		t.Error("expected error for 3rd redirect (via has 2 entries)")
	}
}
