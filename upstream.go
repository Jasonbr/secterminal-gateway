package main

import (
	"net/http"
)

// Upstream is the interface for provider-specific proxy adapters.
type Upstream interface {
	// Proxy forwards the request body to the upstream provider and streams
	// the SSE response back to the client.
	Proxy(w http.ResponseWriter, r *http.Request, model GatewayModel, body []byte)
}
