package main

import (
	"crypto/rand"
	"encoding/hex"
	"log"
	"net/http"
	"os"
	"time"
)

// RequestIDHeader is the HTTP header name for request correlation IDs.
const RequestIDHeader = "X-Request-ID"

// statusResponseWriter wraps http.ResponseWriter to capture the status code.
type statusResponseWriter struct {
	http.ResponseWriter
	status int
}

func (w *statusResponseWriter) WriteHeader(code int) {
	w.status = code
	w.ResponseWriter.WriteHeader(code)
}

// generateRequestID creates a short random hex string for request correlation.
func generateRequestID() string {
	b := make([]byte, 8)
	if _, err := rand.Read(b); err != nil {
		return "unknown"
	}
	return hex.EncodeToString(b)
}

// loggingMiddleware logs each incoming request in a structured format and
// injects a request ID into both the response header and request context.
func loggingMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()

		// Generate or reuse request ID.
		requestID := r.Header.Get(RequestIDHeader)
		if requestID == "" {
			requestID = generateRequestID()
		}
		w.Header().Set(RequestIDHeader, requestID)

		// Wrap to capture status code.
		srw := &statusResponseWriter{ResponseWriter: w, status: http.StatusOK}

		next.ServeHTTP(srw, r)

		// Structured log line: timestamp method path status duration request_id remote_addr
		log.Printf(
			"request method=%s path=%s status=%d duration_ms=%d request_id=%s ip=%s",
			r.Method,
			r.URL.Path,
			srw.status,
			time.Since(start).Milliseconds(),
			requestID,
			getClientIP(r),
		)
	})
}

// initLogging configures the standard logger with flags suitable for production.
func initLogging() {
	log.SetOutput(os.Stdout)
	log.SetFlags(log.LstdFlags | log.LUTC)
}
