package httpapi

import (
	"net"
	"net/http"
	"strings"
)

type clientIPKey struct{}

func clientIP(r *http.Request, trusted []*net.IPNet) string {
	peer, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		peer = r.RemoteAddr
	}
	ip := net.ParseIP(peer)
	if ip == nil {
		return peer
	}
	isTrusted := func(ip net.IP) bool {
		for _, network := range trusted {
			if network.Contains(ip) {
				return true
			}
		}
		return false
	}
	if !isTrusted(ip) {
		return ip.String()
	}
	chain := strings.Split(r.Header.Get("X-Forwarded-For"), ",")
	if len(chain) > 20 {
		return ip.String()
	}
	for i := len(chain) - 1; i >= 0; i-- {
		hop := net.ParseIP(strings.TrimSpace(chain[i]))
		if hop == nil {
			return ip.String()
		}
		ip = hop
		if !isTrusted(ip) {
			return ip.String()
		}
	}
	return ip.String()
}
