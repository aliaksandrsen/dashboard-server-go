package middleware

import (
	"log/slog"
	"math"
	"net"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/aliaksandrsen/dashboard-server-go/internal/httputil"
)

type rateBucket struct {
	count   int
	resetAt time.Time
}

type RateLimiter struct {
	mu      sync.Mutex
	buckets map[string]*rateBucket
	ttl     time.Duration
	limit   int
	log     *slog.Logger
	stopCh  chan struct{}
	once    sync.Once
}

func NewRateLimiter(ttl time.Duration, limit int, log *slog.Logger) *RateLimiter {
	rl := &RateLimiter{
		buckets: make(map[string]*rateBucket),
		ttl:     ttl,
		limit:   limit,
		log:     log,
		stopCh:  make(chan struct{}),
	}

	go rl.cleanupLoop()
	return rl
}

func (rl *RateLimiter) cleanupLoop() {
	ticker := time.NewTicker(rl.ttl)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			now := time.Now()
			rl.mu.Lock()
			for key, b := range rl.buckets {
				if !now.Before(b.resetAt) {
					delete(rl.buckets, key)
				}
			}
			rl.mu.Unlock()
		case <-rl.stopCh:
			return
		}
	}
}

func (rl *RateLimiter) Close() {
	rl.once.Do(func() {
		close(rl.stopCh)
	})
}

func (rl *RateLimiter) Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		key := clientIP(r)
		now := time.Now()

		rl.mu.Lock()
		b, exists := rl.buckets[key]
		if !exists || !now.Before(b.resetAt) {
			b = &rateBucket{
				count:   0,
				resetAt: now.Add(rl.ttl),
			}
			rl.buckets[key] = b
		}
		b.count++
		count := b.count
		resetAt := b.resetAt
		rl.mu.Unlock()

		remaining := max(rl.limit-count, 0)
		resetSec := max(int(math.Ceil(time.Until(resetAt).Seconds())), 1)

		w.Header().Set("X-RateLimit-Limit", strconv.Itoa(rl.limit))
		w.Header().Set("X-RateLimit-Remaining", strconv.Itoa(remaining))
		w.Header().Set("X-RateLimit-Reset", strconv.Itoa(resetSec))

		if count > rl.limit {
			w.Header().Set("Retry-After", strconv.Itoa(resetSec))
			httputil.WriteError(w, r, rl.log, httputil.TooManyRequests("ThrottlerException: Too Many Requests"))
			return
		}

		next.ServeHTTP(w, r)
	})
}

func clientIP(r *http.Request) string {
	if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
		first, _, _ := strings.Cut(xff, ",")
		if trimmed := strings.TrimSpace(first); trimmed != "" {
			return trimmed
		}
	}
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err == nil && host != "" {
		return host
	}
	return r.RemoteAddr
}
