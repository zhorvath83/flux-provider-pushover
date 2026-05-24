package server

import (
	"net"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/zhorvath83/flux-provider-pushover/internal/types"
	"golang.org/x/time/rate"
)
type RateLimiterConfig struct {
	Rate  rate.Limit   // Tokens per second
	Burst int          // Maximum burst size
	TTL   time.Duration // Idle bucket cleanup interval
}

// DefaultRateLimiterConfig provides sensible defaults.
var DefaultRateLimiterConfig = RateLimiterConfig{
	Rate:  10,             // 10 requests per second
	Burst: 30,             // Allow bursts of 30
	TTL:   1 * time.Hour,  // Clean up idle buckets after 1 hour
}

// IPRateLimiter provides per-IP token bucket rate limiting.
type IPRateLimiter struct {
	mu      sync.Mutex
	config  RateLimiterConfig
	buckets map[string]*ipBucket
	stop    chan struct{}
	stopped bool
}

type ipBucket struct {
	limiter  *rate.Limiter
	lastSeen time.Time
}

// NewIPRateLimiter creates a new per-IP rate limiter.
func NewIPRateLimiter(cfg RateLimiterConfig) *IPRateLimiter {
	rl := &IPRateLimiter{
		config:  cfg,
		buckets: make(map[string]*ipBucket),
		stop:    make(chan struct{}),
	}
	go rl.cleanupLoop()
	return rl
}

// Stop terminates the cleanup goroutine and rejects all future requests.
func (rl *IPRateLimiter) Stop() {
	rl.mu.Lock()
	defer rl.mu.Unlock()
	if rl.stopped {
		return
	}
	rl.stopped = true
	close(rl.stop)
}

// Allow checks if a request from the given IP is allowed.
// Returns an error if the rate limiter has been stopped.
func (rl *IPRateLimiter) Allow(ip string) bool {
	rl.mu.Lock()
	defer rl.mu.Unlock()

	if rl.stopped {
		return false
	}

	bucket, ok := rl.buckets[ip]
	if !ok {
		bucket = &ipBucket{
			limiter: rate.NewLimiter(rl.config.Rate, rl.config.Burst),
		}
		rl.buckets[ip] = bucket
	}
	bucket.lastSeen = time.Now()
	return bucket.limiter.Allow()
}

// cleanup removes idle buckets periodically.
func (rl *IPRateLimiter) cleanupLoop() {
	ticker := time.NewTicker(rl.config.TTL)
	defer ticker.Stop()
	for {
		select {
		case <-rl.stop:
			return
		case <-ticker.C:
			rl.mu.Lock()
			now := time.Now()
			for ip, bucket := range rl.buckets {
				if now.Sub(bucket.lastSeen) > rl.config.TTL {
					delete(rl.buckets, ip)
				}
			}
			rl.mu.Unlock()
		}
	}
}

// extractIP returns the client IP from the request.
// Note: X-Forwarded-For is trusted — only deploy behind a reverse proxy
// that strips untrusted X-Forwarded-For headers.
func extractIP(r *http.Request) string {
	if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
		ips := strings.SplitN(xff, ",", 2)
		clientIP := strings.TrimSpace(ips[0])
		if ip := net.ParseIP(clientIP); ip != nil {
			return ip.String()
		}
	}
	ip, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return ip
}

// RateLimitMiddleware returns middleware that applies per-IP rate limiting.
func RateLimitMiddleware(rl *IPRateLimiter, logger Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ip := extractIP(r)
			if !rl.Allow(ip) {
				logger.Warn("rate limit exceeded", "remote_addr", r.RemoteAddr, "ip", ip)
				writeJSONResponse(w, http.StatusTooManyRequests, types.ResponseRateLimitError)
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}