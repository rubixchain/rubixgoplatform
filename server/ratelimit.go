package server

import (
	"net"
	"net/http"
	"sync"
	"time"

	"github.com/rubixchain/rubixgoplatform/core/model"
	"github.com/rubixchain/rubixgoplatform/wrapper/ensweb"
	"golang.org/x/time/rate"
)

// ipLimiter holds a rate limiter and the last time it was accessed.
type ipLimiter struct {
	limiter  *rate.Limiter
	lastSeen time.Time
}

// IPRateLimiter manages per-IP rate limiters with automatic cleanup.
type IPRateLimiter struct {
	mu       sync.RWMutex
	limiters map[string]*ipLimiter
	rate     rate.Limit // requests per second
	burst    int        // max burst size
}

// NewIPRateLimiter creates a new IPRateLimiter.
func NewIPRateLimiter(r rate.Limit, b int) *IPRateLimiter {
	rl := &IPRateLimiter{
		limiters: make(map[string]*ipLimiter),
		rate:     r,
		burst:    b,
	}
	// Start background cleanup goroutine to evict stale entries
	go rl.cleanupLoop()
	return rl
}

// getLimiter returns the rate limiter for the given IP, creating one if needed.
func (rl *IPRateLimiter) getLimiter(ip string) *rate.Limiter {
	rl.mu.Lock()
	defer rl.mu.Unlock()

	if entry, exists := rl.limiters[ip]; exists {
		entry.lastSeen = time.Now()
		return entry.limiter
	}

	limiter := rate.NewLimiter(rl.rate, rl.burst)
	rl.limiters[ip] = &ipLimiter{
		limiter:  limiter,
		lastSeen: time.Now(),
	}
	return limiter
}

// cleanupLoop removes IP entries that haven't been seen in 10 minutes.
func (rl *IPRateLimiter) cleanupLoop() {
	ticker := time.NewTicker(5 * time.Minute)
	defer ticker.Stop()
	for range ticker.C {
		rl.mu.Lock()
		for ip, entry := range rl.limiters {
			if time.Since(entry.lastSeen) > 10*time.Minute {
				delete(rl.limiters, ip)
			}
		}
		rl.mu.Unlock()
	}
}

// Global package-level rate limiter for the public fullnode API.
// Allows 1 request per 10 seconds per IP, with a burst of 3.
var syncRateLimiter = NewIPRateLimiter(rate.Every(10*time.Second), 3)

// checkRateLimit checks the client IP against the rate limiter.
// Returns an HTTP 429 result if limit exceeded, otherwise nil.
func (s *Server) checkRateLimit(req *ensweb.Request) *ensweb.Result {
	// Extract client IP from the request
	var ip string
	if req.Connection != nil {
		ip = req.Connection.RemoteAddr
	}
	if ip == "" {
		// Fallback: parse from the raw HTTP request
		httpReq := req.GetHTTPRequest()
		if httpReq != nil {
			ip, _, _ = net.SplitHostPort(httpReq.RemoteAddr)
		}
	}
	if ip == "" {
		ip = "unknown"
	}

	if !syncRateLimiter.getLimiter(ip).Allow() {
		return s.RenderJSON(req, &model.BasicResponse{
			Status:  false,
			Message: "rate limit exceeded, please try again later",
		}, http.StatusTooManyRequests)
	}

	return nil
}
