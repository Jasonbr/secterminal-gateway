package main

import (
	"net/http"
	"time"
)

// sharedHTTPClient is reused across all upstream requests to avoid creating
// new clients per request (which leaks transport connections).
// A 120-second timeout covers long streaming responses while preventing
// indefinite hangs.
var sharedHTTPClient = &http.Client{
	Timeout: 120 * time.Second,
	Transport: &http.Transport{
		MaxIdleConns:        50,
		MaxIdleConnsPerHost: 10,
		IdleConnTimeout:     90 * time.Second,
	},
}
