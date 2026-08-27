package main

import (
	"encoding/json"
	"log"
	"net/http"
	"strings"
	"sync"
	"time"
)

// ZenModel represents a model from Zen's /models endpoint.
type ZenModel struct {
	ID      string `json:"id"`
	Object  string `json:"object"`
	Created int64  `json:"created"`
	OwnedBy string `json:"owned_by"`
}

// ZenModelsResponse is the response from Zen's /models endpoint.
type ZenModelsResponse struct {
	Object string     `json:"object"`
	Data   []ZenModel `json:"data"`
}

// ZenDiscovery periodically fetches available models from Zen upstream.
type ZenDiscovery struct {
	baseURL         string
	authToken       string
	refreshInterval time.Duration
	mu              sync.RWMutex
	models          map[string]GatewayModel // id -> model (only free models)
	stopCh          chan struct{}
	ready           bool // true after the first successful fetch
	httpClient      *http.Client
}

// NewZenDiscovery creates a new Zen model discovery instance.
func NewZenDiscovery(baseURL, authToken string, refreshInterval time.Duration) *ZenDiscovery {
	return &ZenDiscovery{
		baseURL:         baseURL,
		authToken:       authToken,
		refreshInterval: refreshInterval,
		models:          make(map[string]GatewayModel),
		stopCh:          make(chan struct{}),
		httpClient:      &http.Client{Timeout: 30 * time.Second},
	}
}

// Start begins periodic model discovery in a background goroutine.
// The first fetch runs asynchronously so the gateway starts serving immediately
// with just the static models. Zen models appear once the fetch completes.
func (d *ZenDiscovery) Start() {
	go func() {
		// Initial fetch — runs in background, doesn't block gateway startup.
		d.fetchModels()

		ticker := time.NewTicker(d.refreshInterval)
		defer ticker.Stop()

		for {
			select {
			case <-ticker.C:
				d.fetchModels()
			case <-d.stopCh:
				return
			}
		}
	}()
}

// Stop halts the discovery loop.
func (d *ZenDiscovery) Stop() {
	close(d.stopCh)
}

// GetModels returns a snapshot of discovered Zen models.
func (d *ZenDiscovery) GetModels() []GatewayModel {
	d.mu.RLock()
	defer d.mu.RUnlock()

	result := make([]GatewayModel, 0, len(d.models))
	for _, m := range d.models {
		result = append(result, m)
	}
	return result
}

// HasModel checks if a model ID exists in discovered models.
func (d *ZenDiscovery) HasModel(id string) bool {
	d.mu.RLock()
	defer d.mu.RUnlock()
	_, ok := d.models[id]
	return ok
}

// GetModel returns a specific discovered model.
func (d *ZenDiscovery) GetModel(id string) (GatewayModel, bool) {
	d.mu.RLock()
	defer d.mu.RUnlock()
	m, ok := d.models[id]
	return m, ok
}

// IsReady returns true after the first successful model fetch completes.
func (d *ZenDiscovery) IsReady() bool {
	d.mu.RLock()
	defer d.mu.RUnlock()
	return d.ready
}

func (d *ZenDiscovery) fetchModels() {
	url := d.baseURL + "/models"
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		log.Printf("zen discovery: failed to create request: %v", err)
		return
	}
	req.Header.Set("Authorization", "Bearer "+d.authToken)

	resp, err := d.httpClient.Do(req)
	if err != nil {
		log.Printf("zen discovery: failed to fetch models: %v", err)
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		log.Printf("zen discovery: unexpected status %d", resp.StatusCode)
		return
	}

	var modelsResp ZenModelsResponse
	if err := json.NewDecoder(resp.Body).Decode(&modelsResp); err != nil {
		log.Printf("zen discovery: failed to decode response: %v", err)
		return
	}

	// Filter free models by the "-free" suffix convention.
	// Zen's /models endpoint does not include a pricing field, but free
	// models are named with a "-free" suffix (e.g. "hy3-free"). This
	// matches the OpenCode client's convention for displaying free models.
	newModels := make(map[string]GatewayModel)
	freeCount := 0
	for _, m := range modelsResp.Data {
		if !strings.HasSuffix(m.ID, "-free") {
			continue
		}
		gatewayID := "zen-" + m.ID
		newModels[gatewayID] = GatewayModel{
			ID:           gatewayID,
			DisplayName:  "Zen " + m.ID,
			AllowFree:    true,
			Upstream:     m.ID,
			UpstreamType: "zen",
			RateLimit:    RateLimit{DailyRequests: 20, PromoTokens: 20000, HourlyTokens: 3000},
		}
		freeCount++
	}

	d.mu.Lock()
	defer d.mu.Unlock()

	// Preserve any non-Zen models (shouldn't exist, but defensive).
	for id, m := range d.models {
		if m.UpstreamType != "zen" || id == "sect-free-zen" {
			if _, exists := newModels[id]; !exists {
				newModels[id] = m
			}
		}
	}

	d.models = newModels
	d.ready = true
	log.Printf("zen discovery: found %d free models (out of %d total)", freeCount, len(modelsResp.Data))
}
