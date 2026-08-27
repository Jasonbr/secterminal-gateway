package main

import (
	"os"
)

// Config holds all gateway configuration.
type Config struct {
	Port          string            `yaml:"port"`
	UpstreamAPIs  UpstreamAPIConfig `yaml:"upstreamAPIs"`
	RedisURL      string            `yaml:"redisURL"`
}

// UpstreamAPIConfig stores API keys for each upstream provider.
type UpstreamAPIConfig struct {
	OpenAIKey    string `yaml:"openaiKey"`
	AnthropicKey string `yaml:"anthropicKey"`
	ZenBaseURL   string `yaml:"zenBaseURL"`
}

// loadConfig reads configuration from environment variables with sensible defaults.
func loadConfig() *Config {
	cfg := &Config{
		Port: getEnv("GATEWAY_PORT", "8080"),
		UpstreamAPIs: UpstreamAPIConfig{
			OpenAIKey:     getEnv("OPENAI_API_KEY", ""),
			AnthropicKey:  getEnv("ANTHROPIC_API_KEY", ""),
			ZenBaseURL:    getEnv("ZEN_BASE_URL", "https://opencode.ai/zen/v1"),
		},
		RedisURL: getEnv("REDIS_URL", ""),
	}
	return cfg
}

// getEnv reads an environment variable with a fallback default.
func getEnv(key, fallback string) string {
	if val := os.Getenv(key); val != "" {
		return val
	}
	return fallback
}
