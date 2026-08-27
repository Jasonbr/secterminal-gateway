package main

import (
	"net/http"
	"strings"
)

// FreeSentinelKey is the bearer token value that identifies anonymous free-tier users.
const FreeSentinelKey = "sect-free"

// AuthInfo holds the authentication result for a single request.
type AuthInfo struct {
	IsFree   bool   `json:"isFree"`
	DeviceID string `json:"deviceId"`
	Tier     string `json:"tier"` // "free" | "pro" | "plus"
}

// authenticate extracts the bearer token from the Authorization header and
// determines whether the caller is a free-tier user.
func authenticate(r *http.Request) *AuthInfo {
	token := extractBearerToken(r)
	if token == FreeSentinelKey || token == "" {
		return &AuthInfo{
			IsFree:   true,
			DeviceID: r.Header.Get("X-Device-ID"),
			Tier:     "free",
		}
	}
	// Phase 3: validate License key (sect-pro-...) and set Tier accordingly.
	return &AuthInfo{
		IsFree:   false,
		DeviceID: r.Header.Get("X-Device-ID"),
		Tier:     "pro",
	}
}

// extractBearerToken pulls the token portion from an "Authorization: Bearer xxx" header.
func extractBearerToken(r *http.Request) string {
	auth := r.Header.Get("Authorization")
	if auth == "" {
		return ""
	}
	parts := strings.SplitN(auth, " ", 2)
	if len(parts) == 2 && strings.EqualFold(parts[0], "Bearer") {
		return strings.TrimSpace(parts[1])
	}
	return ""
}
