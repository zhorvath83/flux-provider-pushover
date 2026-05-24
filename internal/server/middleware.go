package server

import (
	"context"
	"crypto/rand"
	"encoding/base32"
	"net/http"
	"strings"
)

const requestIDHeader = "X-Request-ID"

// contextKey is an unexported type for context keys defined in this package.
type contextKey string

const requestIDKey contextKey = "request_id"

// RequestIDFromContext extracts the request ID from the context.
func RequestIDFromContext(ctx context.Context) string {
	if ctx == nil {
		return ""
	}
	v := ctx.Value(requestIDKey)
	if s, ok := v.(string); ok {
		return s
	}
	return ""
}

// generateRequestID creates a 20-character base32 request ID using crypto/rand.
func generateRequestID() string {
	b := make([]byte, 13) // 13 bytes → 21 base32 chars, we take 20
	_, _ = rand.Read(b)
	encoding := base32.StdEncoding.WithPadding(base32.NoPadding)
	return strings.ToLower(encoding.EncodeToString(b)[:20])
}

// RequestIDMiddleware adds a request ID to each request.
// If X-Request-ID header is present, it preserves it. Otherwise generates a new one.
// The request ID is added to the response header and context.
func RequestIDMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestID := r.Header.Get(requestIDHeader)
		if requestID == "" {
			requestID = generateRequestID()
		}

		ctx := contextWithRequestID(r.Context(), requestID)
		r = r.WithContext(ctx)

		w.Header().Set(requestIDHeader, requestID)
		next.ServeHTTP(w, r)
	})
}

// contextWithRequestID stores the request ID in the context.
func contextWithRequestID(ctx context.Context, requestID string) context.Context {
	return context.WithValue(ctx, requestIDKey, requestID)
}
