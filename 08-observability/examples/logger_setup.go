// logger_setup.go — Structured logging with slog (Go 1.21+) and zerolog
package main

import (
	"context"
	"log/slog"
	"os"
	"time"
)

// ============================================================
// slog — Standard library structured logging (Go 1.21+)
// ============================================================

func SetupSlog() *slog.Logger {
	// JSON format for production (machine-parseable)
	handler := slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
		Level:     slog.LevelInfo,
		AddSource: true, // include file:line in logs
	})
	logger := slog.New(handler)
	slog.SetDefault(logger)
	return logger
}

// Logging with context (for request tracing)
type contextKey string

const loggerKey contextKey = "logger"

func WithLogger(ctx context.Context, logger *slog.Logger) context.Context {
	return context.WithValue(ctx, loggerKey, logger)
}

func LoggerFromContext(ctx context.Context) *slog.Logger {
	if l, ok := ctx.Value(loggerKey).(*slog.Logger); ok {
		return l
	}
	return slog.Default()
}

// HTTP middleware that injects a logger with request ID
func loggingMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		requestID := r.Header.Get("X-Request-ID")
		if requestID == "" {
			requestID = generateID()
		}

		// Create a request-scoped logger with context
		logger := slog.Default().With(
			"request_id", requestID,
			"method", r.Method,
			"path", r.URL.Path,
			"remote_addr", r.RemoteAddr,
		)

		// Inject into context
		ctx := WithLogger(r.Context(), logger)

		// Wrap response writer to capture status code
		wrapped := &responseWriter{ResponseWriter: w, statusCode: 200}

		logger.Info("request started")
		next.ServeHTTP(wrapped, r.WithContext(ctx))

		logger.Info("request completed",
			"status", wrapped.statusCode,
			"duration_ms", time.Since(start).Milliseconds(),
		)
	})
}

type responseWriter struct {
	http.ResponseWriter
	statusCode int
}

func (rw *responseWriter) WriteHeader(code int) {
	rw.statusCode = code
	rw.ResponseWriter.WriteHeader(code)
}

// ============================================================
// Logging patterns
// ============================================================

func exampleServiceMethod(ctx context.Context, hotelID string) {
	log := LoggerFromContext(ctx)

	// ✅ Use structured fields — not string formatting
	log.Info("fetching hotel",
		"hotel_id", hotelID,
		"source", "database",
	)

	// ✅ Different levels for different severity
	log.Debug("cache miss", "hotel_id", hotelID)                     // verbose, disabled in prod
	log.Info("hotel fetched successfully", "hotel_id", hotelID)       // normal operations
	log.Warn("hotel rating unavailable, using default", "hotel_id", hotelID) // recoverable issue
	log.Error("failed to fetch hotel", "hotel_id", hotelID, "error", err)    // needs attention

	// ✅ Include error as structured field
	if err := doSomething(); err != nil {
		log.Error("operation failed",
			"hotel_id", hotelID,
			"error", err,
			"operation", "fetchAvailability",
		)
		return
	}

	// ❌ Avoid — string formatting loses structure
	// log.Info(fmt.Sprintf("fetching hotel %s from database", hotelID))
}

// ============================================================
// What NOT to log
// ============================================================

// ❌ Never log:
// - Passwords, tokens, API keys
// - Credit card numbers, CVV
// - Personal Identifiable Information (PII) — email, phone (or hash/mask it)
// - Full request/response bodies (may contain secrets)

func logSanitized(log *slog.Logger, userID, email, cardNumber string) {
	log.Info("payment processed",
		"user_id", userID,
		"email", maskEmail(email),         // ✅ mask PII
		"card_last4", cardNumber[len(cardNumber)-4:], // ✅ partial only
	)
}

func maskEmail(email string) string {
	at := len(email) / 2
	return email[:2] + "****" + email[at:]
}

func main() {
	logger := SetupSlog()

	ctx := WithLogger(context.Background(), logger.With(
		"service", "hotel-service",
		"version", "1.2.3",
		"env", "production",
	))

	logger.Info("service starting",
		"port", 8080,
		"log_level", "info",
	)

	// Simulate request handling
	exampleServiceMethod(ctx, "hotel-123")
}
