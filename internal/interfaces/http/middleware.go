package http

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"time"

	"github.com/google/uuid"

	"github.com/bryanwahyu/flip-style-transfer-engine/internal/application/port"
	"github.com/bryanwahyu/flip-style-transfer-engine/internal/infrastructure/observability"
)

type contextKey string

const (
	idempotencyKeyCtx  contextKey = "idempotency_key"
	requestIDCtx       contextKey = "request_id"
)

// requestIDMiddleware injects a unique request ID (trace ID) into each request context.
func requestIDMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		traceID := r.Header.Get("X-Request-ID")
		if traceID == "" {
			traceID = uuid.New().String()
		}
		ctx := observability.WithTraceID(r.Context(), traceID)
		ctx = context.WithValue(ctx, requestIDCtx, traceID)
		w.Header().Set("X-Request-ID", traceID)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// loggingMiddleware logs every request with duration and status code.
func loggingMiddleware(log *slog.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			start := time.Now()
			ww := &responseWriter{ResponseWriter: w, status: http.StatusOK}
			next.ServeHTTP(ww, r)
			log.Info("http request",
				"method", r.Method,
				"path", r.URL.Path,
				"status", ww.status,
				"duration_ms", time.Since(start).Milliseconds(),
				"trace_id", observability.TraceIDFromContext(r.Context()),
			)
		})
	}
}

// idempotencyMiddleware enforces Idempotency-Key on all state-mutating requests.
// GET requests are passed through unchanged.
func idempotencyMiddleware(store port.IdempotencyStore) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.Method == http.MethodGet {
				next.ServeHTTP(w, r)
				return
			}

			key := r.Header.Get("Idempotency-Key")
			if key == "" {
				writeError(w, http.StatusBadRequest, "idempotency_key_required", "Idempotency-Key header is required")
				return
			}

			// Check if we have a cached response for this key.
			cached, found, err := store.Get(r.Context(), key)
			if err == nil && found {
				w.Header().Set("Content-Type", "application/json")
				w.Header().Set("X-Idempotent-Replayed", "true")
				w.WriteHeader(http.StatusOK)
				w.Write(cached) //nolint:errcheck
				return
			}

			ctx := context.WithValue(r.Context(), idempotencyKeyCtx, key)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

func idempotencyKeyFromCtx(ctx context.Context) string {
	if v, ok := ctx.Value(idempotencyKeyCtx).(string); ok {
		return v
	}
	return ""
}

type responseWriter struct {
	http.ResponseWriter
	status int
}

func (rw *responseWriter) WriteHeader(code int) {
	rw.status = code
	rw.ResponseWriter.WriteHeader(code)
}

type errorResponse struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

func writeError(w http.ResponseWriter, status int, code, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(errorResponse{Code: code, Message: message}) //nolint:errcheck
}
