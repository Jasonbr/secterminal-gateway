package main

import (
	"encoding/json"
	"log"
	"net/http"
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
	baseURL        string
	refreshInterval time.Duration
	mu             sync.RWMutex
	models         map[string]GatewayModel // id -> model
	stopCh         chan struct{}
}

// NewZenDiscovery creates a new Zen model discovery instance.
func NewZenDiscovery(baseURL string, refreshInterval time.Duration) *ZenDiscovery {
	return &ZenDiscovery{
		baseURL:        baseURL,
		refreshInterval: refreshInterval,
		models:         make(map[string]GatewayModel),
		stopCh:         make(chan struct{}),
	}
}

// Start begins periodic model discovery.
func (d *ZenDiscovery) Start() {
	// Initial fetch
	d.fetchModels()

	go func() {
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

func (d *ZenDiscovery) fetchModels() {
	url := d.baseURL + "/models"
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		log.Printf("zen discovery: failed to create request: %v", err)
		return
	}
	req.Header.Set("Authorization", "Bearer public")

	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(req)
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

	d.mu.Lock()
	defer d.mu.Unlock()

	// Update models map - all Zen models are free
	newModels := make(map[string]GatewayModel)
	for _, m := range modelsResp.Data {
		newModels[m.ID] = GatewayModel{
			ID:           "zen-" + m.ID, // Prefix with "zen-" to avoid conflicts
			DisplayName:  "Zen " + m.ID,
			AllowFree:    true,
			Upstream:     m.ID, // Original ID for upstream
			UpstreamType: "zen",
			RateLimit:    RateLimit{DailyRequests: 20, PromoTokens: 20000, HourlyTokens: 3000},
		}
	}

	// Preserve any manually configured models (like sect-free-zen)
	for id, m := range d.models {
		if m.UpstreamType != "zen" || id == "sect-free-zen" {
			newModels[id] = m
		}
	}

	d.models = newModels
	log.Printf("zen discovery: found %d models", len(modelsResp.Data))
}
