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

// getClientIP extracts the client's real IP address, respecting X-Forwarded-For.
func getClientIP(r *http.Request) string {
	// Check X-Forwarded-For first (common when behind a reverse proxy like Nginx).
	xff := r.Header.Get("X-Forwarded-For")
	if xff != "" {
		// Take the first IP in the list.
		if idx := strings.Index(xff, ","); idx > 0 {
			return strings.TrimSpace(xff[:idx])
		}
		return strings.TrimSpace(xff)
	}
	// Fall back to RemoteAddr.
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}
