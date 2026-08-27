package main

import (
	"log"
	"net/http"
)

func main() {
	cfg := loadConfig()
	gateway := NewGateway(cfg)

	mux := http.NewServeMux()
	mux.HandleFunc("/v1/models", gateway.handleModels)
	mux.HandleFunc("/v1/messages", gateway.handleMessages)
	mux.HandleFunc("/v1/chat/completions", gateway.handleChat)

	// Apply middleware: CORS → logging → router.
	handler := corsMiddleware(loggingMiddleware(mux))

	addr := ":" + cfg.Port
	log.Printf("secterminal AI gateway listening on %s", addr)
	log.Printf("  GET  /v1/models")
	log.Printf("  POST /v1/messages         (Anthropic format)")
	log.Printf("  POST /v1/chat/completions  (OpenAI format)")
	log.Printf("  Free sentinel: Bearer %s", FreeSentinelKey)

	if err := http.ListenAndServe(addr, handler); err != nil {
		log.Fatalf("server failed: %v", err)
	}
}
