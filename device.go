package main

import (
	"net"
	"net/http"
	"strings"
)

// validateDeviceID checks that a device ID is non-empty and has a reasonable length.
func validateDeviceID(deviceID string) bool {
	return len(deviceID) >= 8 && len(deviceID) <= 128
}

// getClientIP extracts the client's real IP address.
//
// When behind a trusted reverse proxy (nginx), X-Real-IP is the most reliable
// header. If only X-Forwarded-For is available, we take the LAST IP in the
// chain (the one added by our trusted proxy), not the first (which can be
// spoofed by the client). If neither header is present, we fall back to
// RemoteAddr.
func getClientIP(r *http.Request) string {
	// X-Real-IP is set by nginx and is the single, trusted client IP.
	if realIP := r.Header.Get("X-Real-IP"); realIP != "" {
		return strings.TrimSpace(realIP)
	}

	// X-Forwarded-For: "client, proxy1, proxy2" — the LAST entry is from
	// our trusted proxy. The first entry is client-controlled and can be
	// spoofed, so we must NOT use it.
	xff := r.Header.Get("X-Forwarded-For")
	if xff != "" {
		parts := strings.Split(xff, ",")
		// Take the last (rightmost) IP, which was added by our trusted proxy.
		last := strings.TrimSpace(parts[len(parts)-1])
		if last != "" {
			return last
		}
	}

	// Fall back to RemoteAddr (TCP connection source).
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}
