package main

// GatewayModel defines a model exposed by the gateway.
type GatewayModel struct {
	ID           string    `json:"id"`
	DisplayName  string    `json:"displayName"`
	AllowFree    bool      `json:"allowFree"`
	Upstream     string    `json:"upstream"`
	UpstreamType string    `json:"upstreamType"` // "openai" | "anthropic" | "zen"
	RateLimit    RateLimit `json:"rateLimit"`
}

// RateLimit defines per-model usage limits for the free tier.
type RateLimit struct {
	DailyRequests int `json:"dailyRequests"`
	PromoTokens   int `json:"promoTokens"`
	HourlyTokens  int `json:"hourlyTokens"`
}

// FreeModels is the static list of free models served by the gateway.
var FreeModels = []GatewayModel{
	{
		ID:           "sect-free-fast",
		DisplayName:  "Free Fast",
		AllowFree:    true,
		Upstream:     "gpt-4o-mini",
		UpstreamType: "openai",
		RateLimit:    RateLimit{DailyRequests: 50, PromoTokens: 50000, HourlyTokens: 10000},
	},
	{
		ID:           "sect-free-quality",
		DisplayName:  "Free Quality",
		AllowFree:    true,
		Upstream:     "claude-3-5-haiku",
		UpstreamType: "anthropic",
		RateLimit:    RateLimit{DailyRequests: 30, PromoTokens: 30000, HourlyTokens: 5000},
	},
	{
		ID:           "sect-free-zen",
		DisplayName:  "Free Zen",
		AllowFree:    true,
		Upstream:     "big-pickle",
		UpstreamType: "zen",
		RateLimit:    RateLimit{DailyRequests: 20, PromoTokens: 20000, HourlyTokens: 3000},
	},
}

// allModels returns static FreeModels plus any dynamically discovered Zen models.
func allModels(zenDiscovery *ZenDiscovery) []GatewayModel {
	result := make([]GatewayModel, len(FreeModels))
	copy(result, FreeModels)
	if zenDiscovery != nil {
		for _, m := range zenDiscovery.GetModels() {
			if m.ID != "sect-free-zen" {
				result = append(result, m)
			}
		}
	}
	return result
}

// findModel looks up a model by ID in static + dynamic models.
func findModel(id string, zenDiscovery *ZenDiscovery) *GatewayModel {
	for i := range FreeModels {
		if FreeModels[i].ID == id {
			return &FreeModels[i]
		}
	}
	if zenDiscovery != nil {
		if m, ok := zenDiscovery.GetModel(id); ok {
			return &m
		}
	}
	return nil
}
