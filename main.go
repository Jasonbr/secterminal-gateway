package main

import (
	"context"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"
)

func main() {
	initLogging()

	cfg := loadConfig()
	gateway := NewGateway(cfg)

	mux := http.NewServeMux()
	mux.HandleFunc("/health", gateway.handleHealth)
	mux.HandleFunc("/v1/models", gateway.handleModels)
	mux.HandleFunc("/v1/messages", gateway.handleProxy)
	mux.HandleFunc("/v1/chat/completions", gateway.handleProxy)

	handler := corsMiddleware(loggingMiddleware(mux))

	srv := &http.Server{
		Addr:         ":" + cfg.Port,
		Handler:      handler,
		ReadTimeout:  30 * time.Second,
		WriteTimeout: 120 * time.Second,
	}

	go func() {
		sigCh := make(chan os.Signal, 1)
		signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
		sig := <-sigCh
		log.Printf("received signal %s, shutting down gracefully...", sig)

		if gateway.ZenDiscovery != nil {
			gateway.ZenDiscovery.Stop()
		}

		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if err := srv.Shutdown(ctx); err != nil {
			log.Printf("graceful shutdown failed: %v", err)
		}
	}()

	log.Printf("secterminal AI gateway listening on :%s", cfg.Port)
	log.Printf("  GET  /health")
	log.Printf("  GET  /v1/models")
	log.Printf("  POST /v1/messages         (Anthropic format)")
	log.Printf("  POST /v1/chat/completions  (OpenAI format)")
	log.Printf("  Free sentinel: Bearer %s", FreeSentinelKey)

	if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		log.Fatalf("server failed: %v", err)
	}

	log.Printf("server stopped")
}

// handleHealth returns a simple health check response.
func (g *Gateway) handleHealth(w http.ResponseWriter, r *http.Request) {
	zenReady := false
	if g.ZenDiscovery != nil {
		zenReady = g.ZenDiscovery.IsReady()
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"status":    "ok",
		"zenReady":  zenReady,
		"timestamp": time.Now().UTC().Format(time.RFC3339),
	})
}
