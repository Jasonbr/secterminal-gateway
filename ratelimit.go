package main

import (
	"fmt"
	"sync"
	"time"
)

// RateLimitStore is the interface for rate-limit storage backends.
type RateLimitStore interface {
	GetDailyRequests(key string) int
	IncrementDailyRequests(key string) int
	GetHourlyTokenUsage(key string) int
	AddHourlyTokenUsage(key string, tokens int) int
	GetTokenUsage(key string) int
	AddTokenUsage(key string, tokens int) int
}

// MemoryRateLimitStore is an in-memory implementation for Phase 1.
// Phase 2 will replace this with Redis for multi-instance persistence.
type MemoryRateLimitStore struct {
	mu          sync.Mutex
	counts      map[string]int // key → request count (daily)
	tokensDaily map[string]int // key → token usage (daily, resets at midnight)
	tokensHour  map[string]int // key → token usage (hourly, resets each hour)
}

// NewMemoryRateLimitStore creates a new in-memory store.
func NewMemoryRateLimitStore() *MemoryRateLimitStore {
	return &MemoryRateLimitStore{
		counts:      make(map[string]int),
		tokensDaily: make(map[string]int),
		tokensHour:  make(map[string]int),
	}
}

// GetDailyRequests returns the request count for a given key.
func (s *MemoryRateLimitStore) GetDailyRequests(key string) int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.counts[key]
}

// IncrementDailyRequests increments and returns the request count.
func (s *MemoryRateLimitStore) IncrementDailyRequests(key string) int {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.counts[key]++
	return s.counts[key]
}

// GetHourlyTokenUsage returns the hourly token usage for a given key.
func (s *MemoryRateLimitStore) GetHourlyTokenUsage(key string) int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.tokensHour[key]
}

// AddHourlyTokenUsage adds tokens to the hourly counter and returns the new total.
func (s *MemoryRateLimitStore) AddHourlyTokenUsage(key string, tokens int) int {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.tokensHour[key] += tokens
	return s.tokensHour[key]
}

// GetTokenUsage returns the daily token usage for a given key.
func (s *MemoryRateLimitStore) GetTokenUsage(key string) int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.tokensDaily[key]
}

// AddTokenUsage adds tokens to the daily counter and returns the new total.
func (s *MemoryRateLimitStore) AddTokenUsage(key string, tokens int) int {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.tokensDaily[key] += tokens
	return s.tokensDaily[key]
}

// FreeRateLimiter enforces IP + DeviceID dual-dimension rate limits.
type FreeRateLimiter struct {
	IP       string
	DeviceID string
	ModelID  string
	Limits   RateLimit
	Store    RateLimitStore
}

// dateKey returns a key suffixed with today's date for daily reset.
func dateKey(prefix string) string {
	return prefix + ":" + time.Now().Format("2006-01-02")
}

// hourKey returns a key suffixed with today's date + current hour for hourly reset.
func hourKey(prefix string) string {
	return prefix + ":" + time.Now().Format("2006-01-02-15")
}

// Check verifies whether the request is within rate limits.
func (r *FreeRateLimiter) Check() error {
	if r.Store == nil {
		return nil
	}

	// Check IP dimension (daily request count).
	ipKey := dateKey("ip:" + r.IP)
	ipCount := r.Store.GetDailyRequests(ipKey)
	if ipCount >= r.Limits.DailyRequests {
		return fmt.Errorf("daily request limit exceeded for IP (%d/%d)", ipCount, r.Limits.DailyRequests)
	}

	// Check DeviceID dimension (daily request count).
	if r.DeviceID != "" {
		deviceKey := dateKey("device:" + r.DeviceID)
		deviceCount := r.Store.GetDailyRequests(deviceKey)
		if deviceCount >= r.Limits.DailyRequests {
			return fmt.Errorf("daily request limit exceeded for device (%d/%d)", deviceCount, r.Limits.DailyRequests)
		}
	}

	// Check daily token usage.
	if r.DeviceID != "" {
		tokenKey := dateKey("tokens:device:" + r.DeviceID)
		tokenUsage := r.Store.GetTokenUsage(tokenKey)
		if tokenUsage >= r.Limits.PromoTokens {
			return fmt.Errorf("daily token limit exceeded for device (%d/%d)", tokenUsage, r.Limits.PromoTokens)
		}
	}

	// Check hourly token usage.
	if r.DeviceID != "" && r.Limits.HourlyTokens > 0 {
		hourTokenKey := hourKey("tokens:device:" + r.DeviceID)
		hourTokenUsage := r.Store.GetHourlyTokenUsage(hourTokenKey)
		if hourTokenUsage >= r.Limits.HourlyTokens {
			return fmt.Errorf("hourly token limit exceeded for device (%d/%d)", hourTokenUsage, r.Limits.HourlyTokens)
		}
	}

	return nil
}

// RecordRequest increments the request counters after a successful request.
func (r *FreeRateLimiter) RecordRequest() {
	if r.Store == nil {
		return
	}
	ipKey := dateKey("ip:" + r.IP)
	r.Store.IncrementDailyRequests(ipKey)

	if r.DeviceID != "" {
		deviceKey := dateKey("device:" + r.DeviceID)
		r.Store.IncrementDailyRequests(deviceKey)
	}
}

// RecordTokenUsage adds token usage after a request completes.
// Both daily and hourly counters are updated with the same amount.
func (r *FreeRateLimiter) RecordTokenUsage(inputTokens, outputTokens int) {
	if r.Store == nil || r.DeviceID == "" {
		return
	}
	total := inputTokens + outputTokens
	if total <= 0 {
		return
	}
	tokenKey := dateKey("tokens:device:" + r.DeviceID)
	r.Store.AddTokenUsage(tokenKey, total)

	hourTokenKey := hourKey("tokens:device:" + r.DeviceID)
	r.Store.AddHourlyTokenUsage(hourTokenKey, total)
}
