package httpapi

import (
	"net"
	"net/http/httptest"
	"testing"
)

func TestClientIPRequiresTrustedProxy(t *testing.T) {
	r := httptest.NewRequest("GET", "http://localhost/", nil)
	r.RemoteAddr = "192.0.2.10:8080"
	r.Header.Set("X-Forwarded-For", "198.51.100.5")
	if got := clientIP(r, nil); got != "192.0.2.10" {
		t.Fatal("untrusted header used", got)
	}
	_, network, _ := net.ParseCIDR("192.0.2.0/24")
	if got := clientIP(r, []*net.IPNet{network}); got != "198.51.100.5" {
		t.Fatal(got)
	}
	r.Header.Set("X-Forwarded-For", "203.0.113.1, 198.51.100.5, 192.0.2.30")
	if got := clientIP(r, []*net.IPNet{network}); got != "198.51.100.5" {
		t.Fatal("spoofed leftmost used", got)
	}
	r.Header.Set("X-Forwarded-For", "not-an-ip")
	if got := clientIP(r, []*net.IPNet{network}); got != "192.0.2.10" {
		t.Fatal("invalid proxy chain", got)
	}
}
