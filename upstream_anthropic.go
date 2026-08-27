package main

import (
	"encoding/json"
	"net/http"
	"strings"
)

// AnthropicUpstream proxies requests to the Anthropic Messages API.
type AnthropicUpstream struct {
	APIKey  string
	BaseURL string // https://api.anthropic.com
}

// NewAnthropicUpstream creates an adapter from config.
func NewAnthropicUpstream(cfg *Config) *AnthropicUpstream {
	return &AnthropicUpstream{
		APIKey:  cfg.UpstreamAPIs.AnthropicKey,
		BaseURL: "https://api.anthropic.com",
	}
}

// Proxy forwards an Anthropic-format request directly (no conversion needed).
func (u *AnthropicUpstream) Proxy(w http.ResponseWriter, r *http.Request, model GatewayModel, body []byte) {
	// Replace model ID with upstream model.
	var reqBody map[string]interface{}
	if err := json.Unmarshal(body, &reqBody); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body: "+err.Error())
		return
	}
	reqBody["model"] = model.Upstream
	reqBody["stream"] = true

	upstreamBody, err := json.Marshal(reqBody)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	url := u.BaseURL + "/v1/messages"
	req, err := http.NewRequest(http.MethodPost, url, strings.NewReader(string(upstreamBody)))
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("x-api-key", u.APIKey)
	req.Header.Set("anthropic-version", "2023-06-01")

	streamProxy(w, req, model)
}
