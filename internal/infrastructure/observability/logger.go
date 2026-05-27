package observability

import (
	"context"
	"log/slog"
	"os"
)

type contextKey string

const traceIDKey contextKey = "trace_id"

// NewLogger creates a structured JSON logger suitable for production.
func NewLogger() *slog.Logger {
	return slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
		Level: slog.LevelInfo,
	}))
}

// WithTraceID attaches a trace ID to the context for downstream log correlation.
func WithTraceID(ctx context.Context, traceID string) context.Context {
	return context.WithValue(ctx, traceIDKey, traceID)
}

// TraceIDFromContext retrieves the trace ID from context, or "" if not set.
func TraceIDFromContext(ctx context.Context) string {
	if v, ok := ctx.Value(traceIDKey).(string); ok {
		return v
	}
	return ""
}

// FromContext returns a logger enriched with any trace/transfer IDs in ctx.
func FromContext(ctx context.Context, base *slog.Logger) *slog.Logger {
	if traceID := TraceIDFromContext(ctx); traceID != "" {
		return base.With("trace_id", traceID)
	}
	return base
}
