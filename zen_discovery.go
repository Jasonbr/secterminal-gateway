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
	models          map[string]GatewayModel // id -> model
	stopCh          chan struct{}
	freeModels      map[string]bool // cache of known-free model IDs (by upstream ID)
	ready           bool            // true after the first successful fetch
	httpClient      *http.Client
}

// NewZenDiscovery creates a new Zen model discovery instance.
func NewZenDiscovery(baseURL, authToken string, refreshInterval time.Duration) *ZenDiscovery {
	return &ZenDiscovery{
		baseURL:         baseURL,
		authToken:       authToken,
		refreshInterval: refreshInterval,
		models:          make(map[string]GatewayModel),
		freeModels:      make(map[string]bool),
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

	var wg sync.WaitGroup
	var mu sync.Mutex
	newFreeModels := make(map[string]bool)
	newModels := make(map[string]GatewayModel)

	d.mu.RLock()
	for id := range d.freeModels {
		newFreeModels[id] = true
	}
	d.mu.RUnlock()

	// Concurrency limited to 3 to reduce load on Zen's API.
	sem := make(chan struct{}, 3)
	for _, m := range modelsResp.Data {
		wg.Add(1)
		go func(model ZenModel) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()

			gatewayID := "zen-" + model.ID

			mu.Lock()
			_, knownFree := newFreeModels[model.ID]
			mu.Unlock()

			if !knownFree {
				if d.probeModel(model.ID) {
					mu.Lock()
					newFreeModels[model.ID] = true
					mu.Unlock()
					log.Printf("zen discovery: model %q is FREE", model.ID)
				} else {
					log.Printf("zen discovery: model %q requires paid API key, skipping", model.ID)
					return
				}
			}

			mu.Lock()
			newModels[gatewayID] = GatewayModel{
				ID:           gatewayID,
				DisplayName:  "Zen " + model.ID,
				AllowFree:    true,
				Upstream:     model.ID,
				UpstreamType: "zen",
				RateLimit:    RateLimit{DailyRequests: 20, PromoTokens: 20000, HourlyTokens: 3000},
			}
			mu.Unlock()
		}(m)
	}
	wg.Wait()

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
	d.freeModels = newFreeModels
	d.ready = true
	log.Printf("zen discovery: found %d free models (out of %d total)", len(newModels), len(modelsResp.Data))
}

// probeModel checks if a model supports free access (Bearer public).
// Returns true if the model is free (200, 429, 400), false if it requires
// a paid API key (401). Uses a short 5-second timeout to avoid blocking
// the discovery loop.
func (d *ZenDiscovery) probeModel(modelID string) bool {
	url := d.baseURL + "/chat/completions"
	body := `{"model":"` + modelID + `","messages":[{"role":"user","content":"hi"}],"max_tokens":1}`

	req, err := http.NewRequest(http.MethodPost, url, strings.NewReader(body))
	if err != nil {
		return false
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+d.authToken)

	// Short timeout: we only care about the status code, not the full response.
	probeClient := &http.Client{Timeout: 5 * time.Second}
	resp, err := probeClient.Do(req)
	if err != nil {
		return false
	}
	defer resp.Body.Close()

	switch resp.StatusCode {
	case http.StatusOK, http.StatusTooManyRequests, http.StatusBadRequest:
		return true
	case http.StatusUnauthorized:
		return false
	default:
		return resp.StatusCode >= 200 && resp.StatusCode < 500
	}
}
