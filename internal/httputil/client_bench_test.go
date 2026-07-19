package httputil

import (
	"net"
	"testing"
)

func BenchmarkIsBlockedIP(b *testing.B) {
	ips := []net.IP{
		net.ParseIP("127.0.0.1"),
		net.ParseIP("10.0.0.1"),
		net.ParseIP("172.16.0.1"),
		net.ParseIP("192.168.1.1"),
		net.ParseIP("169.254.1.1"),
		net.ParseIP("8.8.8.8"),
		net.ParseIP("::1"),
		net.ParseIP("fc00::1"),
		net.ParseIP("2001:4860:4860::8888"),
	}
	for _, ip := range ips {
		b.Run(ip.String(), func(b *testing.B) {
			var sink bool
			for b.Loop() {
				sink = isBlockedIP(ip)
			}
			_ = sink
		})
	}
}

func BenchmarkIsHTTPS(b *testing.B) {
	for _, tc := range []struct {
		name, url string
	}{
		{"https", "https://github.com/user/repo/releases/download/v1.0/pkg.deb"},
		{"http", "http://example.com/pkg.deb"},
	} {
		b.Run(tc.name, func(b *testing.B) {
			u := tc.url
			var sink bool
			for b.Loop() {
				sink = IsHTTPS(u)
			}
			_ = sink
		})
	}
}

func BenchmarkNewClient(b *testing.B) {
	for b.Loop() {
		_ = NewClient()
	}
}

func BenchmarkNewVerifyClient(b *testing.B) {
	for b.Loop() {
		_ = NewVerifyClient()
	}
}
