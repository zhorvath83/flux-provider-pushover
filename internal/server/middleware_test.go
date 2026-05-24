package server

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestRequestIDMiddleware_GeneratesID(t *testing.T) {
	var capturedID string
	handler := RequestIDMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedID = RequestIDFromContext(r.Context())
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest("GET", "/test", nil)
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if capturedID == "" {
		t.Error("Expected request ID to be generated")
	}
	if len(capturedID) != 20 {
		t.Errorf("Expected request ID length 20, got %d", len(capturedID))
	}

	responseID := rr.Header().Get("X-Request-ID")
	if responseID != capturedID {
		t.Errorf("Response header %q does not match context ID %q", responseID, capturedID)
	}
}

func TestRequestIDMiddleware_PreservesExistingID(t *testing.T) {
	existingID := "existing-request-id-123"
	var capturedID string
	handler := RequestIDMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedID = RequestIDFromContext(r.Context())
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest("GET", "/test", nil)
	req.Header.Set("X-Request-ID", existingID)
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if capturedID != existingID {
		t.Errorf("Expected preserved ID %q, got %q", existingID, capturedID)
	}

	responseID := rr.Header().Get("X-Request-ID")
	if responseID != existingID {
		t.Errorf("Response header %q does not match existing ID %q", responseID, existingID)
	}
}

func TestRequestIDMiddleware_UniqueIDs(t *testing.T) {
	ids := make(map[string]bool)
	handler := RequestIDMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	for i := 0; i < 100; i++ {
		req := httptest.NewRequest("GET", "/test", nil)
		rr := httptest.NewRecorder()
		handler.ServeHTTP(rr, req)

		id := rr.Header().Get("X-Request-ID")
		if ids[id] {
			t.Errorf("Duplicate request ID generated: %s", id)
		}
		ids[id] = true
	}
}

func TestRequestIDFromContext_Empty(t *testing.T) {
	id := RequestIDFromContext(nil)
	if id != "" {
		t.Errorf("Expected empty string for nil context, got %q", id)
	}
}

func TestGenerateRequestID_Format(t *testing.T) {
	id := generateRequestID()
	if len(id) != 20 {
		t.Errorf("Expected 20 character ID, got %d", len(id))
	}
	for _, c := range id {
		if !((c >= 'a' && c <= 'z') || (c >= '2' && c <= '7')) {
			t.Errorf("Invalid character in request ID: %c", c)
		}
	}
}

func TestRequestIDFromContext_NonStringValue(t *testing.T) {
	// Store a non-string value under the requestIDKey — should return ""
	ctx := context.WithValue(context.Background(), requestIDKey, 12345)
	id := RequestIDFromContext(ctx)
	if id != "" {
		t.Errorf("Expected empty string for non-string value, got %q", id)
	}
}
