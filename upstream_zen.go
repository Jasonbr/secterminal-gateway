package main

import (
	"encoding/json"
	"net/http"
	"strings"
)

// ZenUpstream proxies requests to the OpenCode Zen gateway (free tier).
// Zen free models only support the OpenAI Chat Completions format (/chat/completions),
// not the Anthropic Messages format (/messages).
type ZenUpstream struct {
	BaseURL string // https://opencode.ai/zen/v1
}

// NewZenUpstream creates an adapter from config.
func NewZenUpstream(cfg *Config) *ZenUpstream {
	return &ZenUpstream{
		BaseURL: cfg.UpstreamAPIs.ZenBaseURL,
	}
}

// Proxy converts an Anthropic-format request to OpenAI format and forwards it to Zen.
func (u *ZenUpstream) Proxy(w http.ResponseWriter, r *http.Request, model GatewayModel, body []byte) {
	// Convert Anthropic request → OpenAI request (reuse the converter from upstream_openai.go).
	oaiReq, err := anthropicToOpenAI(body)
	if err != nil {
		writeError(w, http.StatusBadRequest, "failed to convert request: "+err.Error())
		return
	}
	oaiReq["model"] = model.Upstream
	oaiReq["stream"] = true

	upstreamBody, err := json.Marshal(oaiReq)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	// Route to /chat/completions (OpenAI format) — Zen free models don't support /messages.
	url := u.BaseURL + "/chat/completions"
	req, err := http.NewRequest(http.MethodPost, url, strings.NewReader(string(upstreamBody)))
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer public") // Zen free-tier sentinel

	streamProxy(w, req, model)
}
