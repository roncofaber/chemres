package handlers

import (
	"net"
	"net/http"
	"strings"
)

func realClientIP(r *http.Request) string {
	// CF-Connecting-IP: set by Cloudflare, cannot be spoofed by clients.
	if ip := r.Header.Get("CF-Connecting-IP"); ip != "" {
		if net.ParseIP(ip) != nil {
			return ip
		}
	}
	// Fly-Client-IP: set by Fly's edge, cannot be spoofed even if Cloudflare is bypassed.
	if ip := r.Header.Get("Fly-Client-IP"); ip != "" {
		if net.ParseIP(ip) != nil {
			return ip
		}
	}
	// XFF: best-effort for self-hosted deployments behind nginx or other proxies.
	if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
		candidate := strings.TrimSpace(strings.SplitN(xff, ",", 2)[0])
		if net.ParseIP(candidate) != nil {
			return candidate
		}
	}
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}
