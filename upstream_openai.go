package main

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"
)

// OpenAIUpstream proxies requests to the OpenAI Chat Completions API.
type OpenAIUpstream struct {
	APIKey  string
	BaseURL string // https://api.openai.com/v1
}

// NewOpenAIUpstream creates an adapter from config.
func NewOpenAIUpstream(cfg *Config) *OpenAIUpstream {
	return &OpenAIUpstream{
		APIKey:  cfg.UpstreamAPIs.OpenAIKey,
		BaseURL: "https://api.openai.com/v1",
	}
}

// Proxy forwards an Anthropic-format request as an OpenAI-format request.
func (u *OpenAIUpstream) Proxy(w http.ResponseWriter, r *http.Request, model GatewayModel, body []byte) {
	// Convert Anthropic request → OpenAI request.
	oaiReq, err := anthropicToOpenAI(body)
	if err != nil {
		writeError(w, http.StatusBadRequest, "failed to convert request: "+err.Error())
		return
	}

	// Set stream=true for SSE.
	oaiReq["stream"] = true

	upstreamBody, err := json.Marshal(oaiReq)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to marshal request: "+err.Error())
		return
	}

	url := u.BaseURL + "/chat/completions"
	req, err := http.NewRequest(http.MethodPost, url, strings.NewReader(string(upstreamBody)))
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+u.APIKey)

	streamProxy(w, req, model)
}

// anthropicToOpenAI converts an Anthropic Messages API request body to OpenAI Chat Completions format.
func anthropicToOpenAI(body []byte) (map[string]interface{}, error) {
	var anthropicReq map[string]interface{}
	if err := json.Unmarshal(body, &anthropicReq); err != nil {
		return nil, err
	}

	result := map[string]interface{}{}

	// Model: replace with upstream model ID.
	result["model"] = anthropicReq["model"]

	// Messages: Anthropic messages → OpenAI messages.
	if msgs, ok := anthropicReq["messages"].([]interface{}); ok {
		var oaiMsgs []map[string]interface{}
		for _, m := range msgs {
			msg, _ := m.(map[string]interface{})
			oaiMsg := map[string]interface{}{
				"role":    msg["role"],
				"content": msg["content"],
			}
			oaiMsgs = append(oaiMsgs, oaiMsg)
		}
		result["messages"] = oaiMsgs
	}

	// System: Anthropic uses top-level "system", OpenAI uses a system message.
	if sys, ok := anthropicReq["system"]; ok && sys != nil {
		if msgs, ok := result["messages"].([]map[string]interface{}); ok {
			systemMsg := map[string]interface{}{
				"role":    "system",
				"content": sys,
			}
			result["messages"] = append([]map[string]interface{}{systemMsg}, msgs...)
		}
	}

	// MaxTokens.
	if mt, ok := anthropicReq["max_tokens"]; ok {
		result["max_tokens"] = mt
	}

	// Tools: basic passthrough (detailed conversion is in secterminal's converter.go).
	if tools, ok := anthropicReq["tools"]; ok && tools != nil {
		result["tools"] = tools
	}

	return result, nil
}

// streamProxy sends a request to upstream and streams the SSE response back to the client.
func streamProxy(w http.ResponseWriter, req *http.Request, model GatewayModel) {
	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		log.Printf("upstream request failed: %v", err)
		writeError(w, http.StatusBadGateway, "upstream request failed: "+err.Error())
		return
	}
	defer resp.Body.Close()

	// Check for non-200 upstream response.
	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		log.Printf("upstream error: status=%d model=%s body=%s", resp.StatusCode, model.ID, string(respBody))
		writeError(w, resp.StatusCode, fmt.Sprintf("upstream error: %s", string(respBody)))
		return
	}

	// Set SSE headers and stream response body back to client.
	flusher, ok := w.(http.Flusher)
	if !ok {
		// Client doesn't support flushing; buffer the entire response.
		w.Header().Set("Content-Type", "application/json")
		io.Copy(w, resp.Body)
		return
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")

	buf := make([]byte, 4096)
	for {
		n, err := resp.Body.Read(buf)
		if n > 0 {
			w.Write(buf[:n])
			flusher.Flush()
		}
		if err != nil {
			break
		}
	}
}
