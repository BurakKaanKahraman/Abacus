package middleware

import (
	"fmt"
	"net/http"
	"strconv"
	"sync"
	"time"

	"github.com/BurakKaanKahraman/abacus/backend/internal/domain"
	"github.com/BurakKaanKahraman/abacus/backend/internal/httpx"
	"golang.org/x/time/rate"
)

const (
	// visitorTTL is how long an idle client's bucket is kept. Evicting stale
	// buckets bounds the memory a hostile client can force the service to hold.
	visitorTTL = 10 * time.Minute
	// sweepInterval is how often expired buckets are collected.
	sweepInterval = time.Minute
)

// visitor is one client's token bucket plus its last-seen timestamp.
type visitor struct {
	limiter  *rate.Limiter
	lastSeen time.Time
}

// RateLimiter throttles requests per client IP with a token bucket.
//
// The bucket refills at limit/minute and holds `burst` tokens, so a client may
// spend a short burst and then settles into the sustained rate.
type RateLimiter struct {
	mu       sync.Mutex
	visitors map[string]*visitor

	limit      rate.Limit
	burst      int
	perMinute  int
	trustProxy bool

	now  func() time.Time
	stop chan struct{}
	once sync.Once
}

// NewRateLimiter starts a rate limiter and its background sweeper. Close must
// be called to release the sweeper goroutine.
func NewRateLimiter(perMinute, burst int, trustProxyHeaders bool) *RateLimiter {
	rl := &RateLimiter{
		visitors:   make(map[string]*visitor),
		limit:      rate.Limit(float64(perMinute) / 60.0),
		burst:      burst,
		perMinute:  perMinute,
		trustProxy: trustProxyHeaders,
		now:        time.Now,
		stop:       make(chan struct{}),
	}
	go rl.sweep()
	return rl
}

// Close stops the background sweeper. It is safe to call more than once.
func (rl *RateLimiter) Close() error {
	rl.once.Do(func() { close(rl.stop) })
	return nil
}

// Middleware rejects requests that exceed the configured rate with an RFC 7807
// document and a Retry-After hint.
func (rl *RateLimiter) Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		limiter := rl.limiterFor(ClientIP(r, rl.trustProxy))

		header := w.Header()
		header.Set("X-RateLimit-Limit", strconv.Itoa(rl.perMinute))

		if !limiter.Allow() {
			retryAfter := rl.retryAfter()
			header.Set("X-RateLimit-Remaining", "0")
			header.Set("Retry-After", strconv.Itoa(retryAfter))
			httpx.WriteProblem(w, r, domain.NewRateLimitError(fmt.Sprintf(
				"Rate limit exceeded. Maximum %d requests per minute allowed.", rl.perMinute)))
			return
		}

		header.Set("X-RateLimit-Remaining", strconv.Itoa(int(limiter.Tokens())))
		next.ServeHTTP(w, r)
	})
}

// limiterFor returns the bucket for a client, creating it on first contact.
func (rl *RateLimiter) limiterFor(key string) *rate.Limiter {
	rl.mu.Lock()
	defer rl.mu.Unlock()

	existing, ok := rl.visitors[key]
	if !ok {
		existing = &visitor{limiter: rate.NewLimiter(rl.limit, rl.burst)}
		rl.visitors[key] = existing
	}
	existing.lastSeen = rl.now()
	return existing.limiter
}

// retryAfter reports, in seconds, how long until the next token is available.
func (rl *RateLimiter) retryAfter() int {
	if rl.limit <= 0 {
		return 60
	}
	seconds := int(1.0/float64(rl.limit) + 0.5)
	if seconds < 1 {
		return 1
	}
	return seconds
}

// sweep evicts buckets that have not been touched within visitorTTL.
func (rl *RateLimiter) sweep() {
	ticker := time.NewTicker(sweepInterval)
	defer ticker.Stop()

	for {
		select {
		case <-rl.stop:
			return
		case <-ticker.C:
			cutoff := rl.now().Add(-visitorTTL)
			rl.mu.Lock()
			for key, v := range rl.visitors {
				if v.lastSeen.Before(cutoff) {
					delete(rl.visitors, key)
				}
			}
			rl.mu.Unlock()
		}
	}
}
