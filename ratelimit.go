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
	GetTokenUsage(key string) int
	AddTokenUsage(key string, tokens int) int
}

// MemoryRateLimitStore is an in-memory implementation for Phase 1.
// Phase 2 will replace this with Redis.
type MemoryRateLimitStore struct {
	mu     sync.Mutex
	counts map[string]int // key → request count
	tokens map[string]int // key → token usage
}

// NewMemoryRateLimitStore creates a new in-memory store.
func NewMemoryRateLimitStore() *MemoryRateLimitStore {
	return &MemoryRateLimitStore{
		counts: make(map[string]int),
		tokens: make(map[string]int),
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

// GetTokenUsage returns the token usage for a given key.
func (s *MemoryRateLimitStore) GetTokenUsage(key string) int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.tokens[key]
}

// AddTokenUsage adds tokens to the usage counter and returns the new total.
func (s *MemoryRateLimitStore) AddTokenUsage(key string, tokens int) int {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.tokens[key] += tokens
	return s.tokens[key]
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

// Check verifies whether the request is within rate limits.
func (r *FreeRateLimiter) Check() error {
	if r.Store == nil {
		return nil
	}

	// Check IP dimension.
	ipKey := dateKey("ip:" + r.IP)
	ipCount := r.Store.GetDailyRequests(ipKey)
	if ipCount >= r.Limits.DailyRequests {
		return fmt.Errorf("daily request limit exceeded for IP (%d/%d)", ipCount, r.Limits.DailyRequests)
	}

	// Check DeviceID dimension (if provided).
	if r.DeviceID != "" {
		deviceKey := dateKey("device:" + r.DeviceID)
		deviceCount := r.Store.GetDailyRequests(deviceKey)
		if deviceCount >= r.Limits.DailyRequests {
			return fmt.Errorf("daily request limit exceeded for device (%d/%d)", deviceCount, r.Limits.DailyRequests)
		}
	}

	// Check token usage.
	if r.DeviceID != "" {
		tokenKey := "tokens:device:" + r.DeviceID
		tokenUsage := r.Store.GetTokenUsage(tokenKey)
		if tokenUsage >= r.Limits.PromoTokens {
			return fmt.Errorf("token limit exceeded for device (%d/%d)", tokenUsage, r.Limits.PromoTokens)
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
func (r *FreeRateLimiter) RecordTokenUsage(inputTokens, outputTokens int) {
	if r.Store == nil || r.DeviceID == "" {
		return
	}
	tokenKey := "tokens:device:" + r.DeviceID
	r.Store.AddTokenUsage(tokenKey, inputTokens+outputTokens)
}
