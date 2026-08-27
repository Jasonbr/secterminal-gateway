package main

import (
	"net/http"
	"strings"
)

// FreeSentinelKey is the bearer token value that identifies anonymous free-tier users.
const FreeSentinelKey = "sect-free"

// ProLicensePrefix is the prefix that identifies a valid pro-tier license key.
// Phase 3 will replace this prefix check with full cryptographic validation.
const ProLicensePrefix = "sect-pro-"

// AuthInfo holds the authentication result for a single request.
type AuthInfo struct {
	IsFree   bool   `json:"isFree"`
	DeviceID string `json:"deviceId"`
	Tier     string `json:"tier"` // "free" | "pro" | "plus"
}

// authenticate extracts the bearer token from the Authorization header and
// determines whether the caller is a free-tier or pro-tier user.
//
// Returns an error if the token is non-empty, not the free sentinel, and
// does not match a valid pro license key prefix. The caller should respond
// with 401 Unauthorized in that case.
func authenticate(r *http.Request) (*AuthInfo, error) {
	token := extractBearerToken(r)

	// No token or free sentinel → free tier.
	if token == "" || token == FreeSentinelKey {
		return &AuthInfo{
			IsFree:   true,
			DeviceID: r.Header.Get("X-Device-ID"),
			Tier:     "free",
		}, nil
	}

	// Pro tier: must start with the license key prefix.
	// Phase 3: validate full license key (signature, expiry, device binding).
	if strings.HasPrefix(token, ProLicensePrefix) {
		return &AuthInfo{
			IsFree:   false,
			DeviceID: r.Header.Get("X-Device-ID"),
			Tier:     "pro",
		}, nil
	}

	// Unknown token — reject instead of silently upgrading to pro.
	return nil, errInvalidToken
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
