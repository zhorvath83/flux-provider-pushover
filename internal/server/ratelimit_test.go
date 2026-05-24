package server

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestIPRateLimiter_AllowWithinBurst(t *testing.T) {
	rl := NewIPRateLimiter(RateLimiterConfig{
		Rate:  10,
		Burst: 30,
		TTL:   1 * time.Hour,
	})
	defer rl.Stop()

	for i := 0; i < 30; i++ {
		if !rl.Allow("1.2.3.4") {
			t.Errorf("Request %d should be allowed within burst", i+1)
		}
	}
}

func TestIPRateLimiter_RejectOverBurst(t *testing.T) {
	rl := NewIPRateLimiter(RateLimiterConfig{
		Rate:  1,
		Burst: 5,
		TTL:   1 * time.Hour,
	})
	defer rl.Stop()

	for i := 0; i < 5; i++ {
		rl.Allow("1.2.3.4")
	}
	if rl.Allow("1.2.3.4") {
		t.Error("6th request should be rejected")
	}
}

func TestIPRateLimiter_DifferentIPs(t *testing.T) {
	rl := NewIPRateLimiter(RateLimiterConfig{
		Rate:  1,
		Burst: 2,
		TTL:   1 * time.Hour,
	})
	defer rl.Stop()

	rl.Allow("1.2.3.4")
	rl.Allow("1.2.3.4")
	if rl.Allow("1.2.3.4") {
		t.Error("3rd request from same IP should be rejected")
	}
	if !rl.Allow("5.6.7.8") {
		t.Error("Request from different IP should be allowed")
	}
}

func TestExtractIP(t *testing.T) {
	tests := []struct {
		name       string
		remoteAddr string
		xff        string
		expected   string
	}{
		{"direct", "1.2.3.4:1234", "", "1.2.3.4"},
		{"xff", "5.6.7.8:1234", "9.8.7.6", "9.8.7.6"},
		{"xff multi", "5.6.7.8:1234", "9.8.7.6, 10.0.0.1", "9.8.7.6"},
		{"no port", "1.2.3.4", "", "1.2.3.4"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest("GET", "/", nil)
			req.RemoteAddr = tt.remoteAddr
			if tt.xff != "" {
				req.Header.Set("X-Forwarded-For", tt.xff)
			}
			ip := extractIP(req)
			if ip != tt.expected {
				t.Errorf("Expected %s, got %s", tt.expected, ip)
			}
		})
	}
}

func TestRateLimitMiddleware(t *testing.T) {
	rl := NewIPRateLimiter(RateLimiterConfig{
		Rate:  1,
		Burst: 2,
		TTL:   1 * time.Hour,
	})
	defer rl.Stop()

	logger := &MockLogger{}

	handler := RateLimitMiddleware(rl, logger)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	// Allow 2 requests
	for i := 0; i < 2; i++ {
		req := httptest.NewRequest("GET", "/webhook", nil)
		req.RemoteAddr = "1.2.3.4:1234"
		rr := httptest.NewRecorder()
		handler.ServeHTTP(rr, req)
		if rr.Code != http.StatusOK {
			t.Errorf("Request %d should be allowed, got %d", i+1, rr.Code)
		}
	}

	// 3rd request should be rejected
	req := httptest.NewRequest("GET", "/webhook", nil)
	req.RemoteAddr = "1.2.3.4:1234"
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusTooManyRequests {
		t.Errorf("3rd request should be rate limited, got %d", rr.Code)
	}
}
func TestIPRateLimiter_Stop(t *testing.T) {
	rl := NewIPRateLimiter(RateLimiterConfig{
		Rate:  10,
		Burst: 30,
		TTL:   1 * time.Hour,
	})

	rl.Allow("1.2.3.4")

	// Stop terminates the cleanup goroutine and rejects further requests.
	rl.Stop()

	// Double Stop should not panic.
	rl.Stop()

	// Allow must return false after Stop — no unbounded bucket growth.
	if rl.Allow("5.6.7.8") {
		t.Error("Allow should reject requests after Stop")
	}
}

func TestIPRateLimiter_CleanupRemovesIdleBuckets(t *testing.T) {
	rl := NewIPRateLimiter(RateLimiterConfig{
		Rate:  10,
		Burst: 30,
		TTL:   50 * time.Millisecond,
	})
	defer rl.Stop()

	rl.Allow("1.2.3.4")

	rl.mu.Lock()
	countBefore := len(rl.buckets)
	rl.mu.Unlock()

	if countBefore != 1 {
		t.Fatalf("Expected 1 bucket, got %d", countBefore)
	}

	// Wait for cleanup to run
	time.Sleep(100 * time.Millisecond)

	rl.mu.Lock()
	countAfter := len(rl.buckets)
	rl.mu.Unlock()

	if countAfter != 0 {
		t.Errorf("Expected 0 buckets after cleanup, got %d", countAfter)
	}
}

func TestIPRateLimiter_Concurrent(t *testing.T) {
	rl := NewIPRateLimiter(RateLimiterConfig{
		Rate:  1000,
		Burst: 5000,
		TTL:   1 * time.Hour,
	})
	defer rl.Stop()

	var wg sync.WaitGroup
	allowed := int64(0)
	rejected := int64(0)
	var countMu sync.Mutex

	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func(ip string) {
			defer wg.Done()
			for j := 0; j < 50; j++ {
				if rl.Allow(ip) {
					countMu.Lock()
					allowed++
					countMu.Unlock()
				} else {
					countMu.Lock()
					rejected++
					countMu.Unlock()
				}
			}
		}(fmt.Sprintf("10.0.%d.%d", i/256, i%256))
	}
	wg.Wait()

	if allowed == 0 {
		t.Error("Expected some requests to be allowed")
	}
	// With 100 IPs × 50 requests = 5000 total, burst 5000 per IP, rate 1000/s
	// all should be allowed in the first burst
	t.Logf("Concurrent test: %d allowed, %d rejected", allowed, rejected)
}

func TestRateLimitMiddleware_ResponseFormat(t *testing.T) {
	rl := NewIPRateLimiter(RateLimiterConfig{
		Rate:  1,
		Burst: 1,
		TTL:   1 * time.Hour,
	})
	defer rl.Stop()

	logger := &MockLogger{}

	handler := RateLimitMiddleware(rl, logger)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	// Exhaust burst
	req := httptest.NewRequest("GET", "/webhook", nil)
	req.RemoteAddr = "1.2.3.4:1234"
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	// Next request should be rate limited
	rr = httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusTooManyRequests {
		t.Errorf("Expected %d, got %d", http.StatusTooManyRequests, rr.Code)
	}

	if ct := rr.Header().Get("Content-Type"); ct != "application/json" {
		t.Errorf("Expected Content-Type application/json, got %q", ct)
	}

	body := strings.TrimSpace(rr.Body.String())
	if !strings.HasPrefix(body, "{") || !strings.HasSuffix(body, "}") {
		t.Errorf("Expected JSON response body, got %q", body)
	}
	if !strings.Contains(body, "Rate limit exceeded") {
		t.Errorf("Expected error message in body, got %q", body)
	}
}
