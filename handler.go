package main

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"time"
)

// Gateway holds shared state for HTTP handlers.
type Gateway struct {
	Config        *Config
	Store         RateLimitStore
	upstreams     map[string]Upstream
	ZenDiscovery  *ZenDiscovery
}

// NewGateway creates a Gateway instance with the given config.
func NewGateway(cfg *Config) *Gateway {
	g := &Gateway{
		Config: cfg,
		Store:  NewMemoryRateLimitStore(),
		upstreams: map[string]Upstream{
			"openai":    NewOpenAIUpstream(cfg),
			"anthropic": NewAnthropicUpstream(cfg),
			"zen":       NewZenUpstream(cfg),
		},
	}

	if cfg.UpstreamAPIs.ZenBaseURL != "" {
		g.ZenDiscovery = NewZenDiscovery(cfg.UpstreamAPIs.ZenBaseURL, 1*time.Hour)
		g.ZenDiscovery.Start()
	}

	return g
}

func (g *Gateway) selectUpstream(m *GatewayModel) Upstream {
	if u, ok := g.upstreams[m.UpstreamType]; ok {
		return u
	}
	return nil
}

// handleModels responds with the list of available free models.
func (g *Gateway) handleModels(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"object": "list",
		"data":   allModels(g.ZenDiscovery),
	})
}

// handleMessages processes Anthropic Messages API format requests.
func (g *Gateway) handleMessages(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	// Step 1: Authenticate.
	authInfo := authenticate(r)

	// Step 2: Validate device ID for free users.
	if authInfo.IsFree && !validateDeviceID(authInfo.DeviceID) {
		writeError(w, http.StatusBadRequest, "missing or invalid X-Device-ID header")
		return
	}

	// Step 3: Read body bytes and parse to determine model.
	bodyBytes, err := io.ReadAll(r.Body)
	if err != nil {
		writeError(w, http.StatusBadRequest, "failed to read body: "+err.Error())
		return
	}
	var body map[string]interface{}
	if err := json.Unmarshal(bodyBytes, &body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body: "+err.Error())
		return
	}

	modelID, _ := body["model"].(string)
	if modelID == "" {
		writeError(w, http.StatusBadRequest, "missing model field")
		return
	}

	modelInfo := findModel(modelID, g.ZenDiscovery)
	if modelInfo == nil {
		writeError(w, http.StatusNotFound, fmt.Sprintf("model %q not found", modelID))
		return
	}

	if authInfo.IsFree && !modelInfo.AllowFree {
		writeError(w, http.StatusForbidden, fmt.Sprintf("model %q requires a paid tier", modelID))
		return
	}

	ip := getClientIP(r)
	limiter := &FreeRateLimiter{
		IP:       ip,
		DeviceID: authInfo.DeviceID,
		ModelID:  modelInfo.ID,
		Limits:   modelInfo.RateLimit,
		Store:    g.Store,
	}
	if err := limiter.Check(); err != nil {
		log.Printf("rate limited: ip=%s device=%s model=%s err=%v", ip, authInfo.DeviceID, modelID, err)
		writeError(w, http.StatusTooManyRequests, err.Error())
		return
	}

	upstream := g.selectUpstream(modelInfo)
	if upstream == nil {
		writeError(w, http.StatusInternalServerError, fmt.Sprintf("no upstream adapter for type %q", modelInfo.UpstreamType))
		return
	}
	upstream.Proxy(w, r, *modelInfo, bodyBytes)
}

// handleChat processes OpenAI Chat Completions API format requests.
func (g *Gateway) handleChat(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	authInfo := authenticate(r)

	if authInfo.IsFree && !validateDeviceID(authInfo.DeviceID) {
		writeError(w, http.StatusBadRequest, "missing or invalid X-Device-ID header")
		return
	}

	bodyBytes, err := io.ReadAll(r.Body)
	if err != nil {
		writeError(w, http.StatusBadRequest, "failed to read body: "+err.Error())
		return
	}
	var body map[string]interface{}
	if err := json.Unmarshal(bodyBytes, &body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body: "+err.Error())
		return
	}

	modelID, _ := body["model"].(string)
	if modelID == "" {
		writeError(w, http.StatusBadRequest, "missing model field")
		return
	}

	modelInfo := findModel(modelID, g.ZenDiscovery)
	if modelInfo == nil {
		writeError(w, http.StatusNotFound, fmt.Sprintf("model %q not found", modelID))
		return
	}

	if authInfo.IsFree && !modelInfo.AllowFree {
		writeError(w, http.StatusForbidden, fmt.Sprintf("model %q requires a paid tier", modelID))
		return
	}

	ip := getClientIP(r)
	limiter := &FreeRateLimiter{
		IP:       ip,
		DeviceID: authInfo.DeviceID,
		ModelID:  modelInfo.ID,
		Limits:   modelInfo.RateLimit,
		Store:    g.Store,
	}
	if err := limiter.Check(); err != nil {
		log.Printf("rate limited: ip=%s device=%s model=%s err=%v", ip, authInfo.DeviceID, modelID, err)
		writeError(w, http.StatusTooManyRequests, err.Error())
		return
	}

	upstream := g.selectUpstream(modelInfo)
	if upstream == nil {
		writeError(w, http.StatusInternalServerError, fmt.Sprintf("no upstream adapter for type %q", modelInfo.UpstreamType))
		return
	}
	upstream.Proxy(w, r, *modelInfo, bodyBytes)
}

// writeJSON sends a JSON response with the given status code.
func writeJSON(w http.ResponseWriter, status int, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(data); err != nil {
		log.Printf("failed to write JSON response: %v", err)
	}
}

// writeError sends a JSON error response.
func writeError(w http.ResponseWriter, status int, message string) {
	writeJSON(w, status, map[string]interface{}{
		"error": map[string]interface{}{
			"message": message,
			"code":    status,
		},
	})
}

// loggingMiddleware logs each incoming request.
func loggingMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		log.Printf("%s %s %s", r.Method, r.URL.Path, r.RemoteAddr)
		next.ServeHTTP(w, r)
	})
}

// corsMiddleware adds permissive CORS headers for browser clients.
func corsMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Authorization, Content-Type, X-Device-ID, X-License-Tier")
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusOK)
			return
		}
		next.ServeHTTP(w, r)
	})
}
