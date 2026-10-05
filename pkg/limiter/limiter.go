package limiter

import (
	"context"
	_ "embed"
	"sync"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"github.com/redis/go-redis/v9"
	"github.com/sony/gobreaker"
)

//go:embed script.lua
var luaScript string

// Metrics
var (
	requestsTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "rate_limit_requests_total",
			Help: "Total number of rate limit requests",
		},
		[]string{"status", "mechanism"}, // status: allowed/blocked, mechanism: redis/fallback
	)
	requestLatency = promauto.NewHistogram(
		prometheus.HistogramOpts{
			Name: "rate_limit_latency_seconds",
			Help: "Latency of rate limit checks",
		},
	)
)

// Limiter represents a token bucket rate limiter using Redis
type Limiter struct {
	client        redis.UniversalClient
	script        *redis.Script
	rate          float64
	capacity      int
	fallbackLimit   int
	fallbackBuckets sync.Map
	cb              *gobreaker.CircuitBreaker
}

// TokenBucket is a thread-safe token bucket for fail-open logic
type TokenBucket struct {
	mu         sync.Mutex
	capacity   int
	tokens     float64
	rate       float64 // tokens per second
	lastRefill time.Time
}

func NewTokenBucket(rate int, capacity int) *TokenBucket {
	// For fallback, we assume rate is per second if derived from simple check
	return &TokenBucket{
		capacity:   capacity,
		tokens:     float64(capacity),
		rate:       float64(rate),
		lastRefill: time.Now(),
	}
}

func (tb *TokenBucket) Allow() bool {
	tb.mu.Lock()
	defer tb.mu.Unlock()

	now := time.Now()
	elapsed := now.Sub(tb.lastRefill).Seconds()
	tb.lastRefill = now

	// Refill tokens
	tb.tokens += elapsed * tb.rate
	if tb.tokens > float64(tb.capacity) {
		tb.tokens = float64(tb.capacity)
	}

	if tb.tokens >= 1.0 {
		tb.tokens--
		return true
	}
	return false
}

func NewLimiter(client redis.UniversalClient, limit int, window time.Duration, fallbackLimit int) *Limiter {
	// Pre-load the script into a Go-Redis script object
	redisScript := redis.NewScript(luaScript)

	// Calculate rate (tokens/sec) and capacity (burst)
	// If window is 1s, rate = limit.
	// If window is 60s, rate = limit / 60.
	rate := float64(limit) / window.Seconds()
	capacity := limit

	// Circuit Breaker Settings
	st := gobreaker.Settings{
		Name:        "RedisLimiter",
		MaxRequests: 0,               // unlimited requests in half-open state
		Interval:    0,               // cyclic clearing disabled
		Timeout:     5 * time.Second, // Open state duration
		ReadyToTrip: func(counts gobreaker.Counts) bool {
			// Trip if we fail 3 times and >60% of requests are failing
			return counts.Requests >= 3 && float64(counts.TotalFailures)/float64(counts.Requests) >= 0.6
		},
	}

	return &Limiter{
		client:        client,
		script:        redisScript,
		rate:          rate,
		capacity:      capacity,
		fallbackLimit: fallbackLimit,
		cb:            gobreaker.NewCircuitBreaker(st),
	}
}

// RateLimitResult contains the status of the rate limit check
type RateLimitResult struct {
	Allowed   bool
	Limit     int
	Remaining int
	Reset     int64 // Seconds until full refill
}

func (l *Limiter) Allow(ctx context.Context, identifier string) (*RateLimitResult, error) {
	return l.allow(ctx, identifier, l.rate, l.capacity, l.fallbackLimit)
}

func (l *Limiter) AllowDynamic(ctx context.Context, identifier string, limit int, window time.Duration) (*RateLimitResult, error) {
	rate := float64(limit) / window.Seconds()
	return l.allow(ctx, identifier, rate, limit, limit) // Use the dynamic limit as fallback limit too
}

func (l *Limiter) allow(ctx context.Context, identifier string, rate float64, capacity int, fallbackLimit int) (*RateLimitResult, error) {
	start := time.Now()
	defer func() {
		requestLatency.Observe(time.Since(start).Seconds())
	}()

	key := "ratelimit:" + identifier
	now := time.Now().UnixMicro()

	operation := func() (interface{}, error) {
		res, err := l.script.Run(ctx, l.client, []string{key}, now, rate, capacity, 1).Slice()
		if err != nil {
			return nil, err
		}

		allowedInt, _ := res[0].(int64)
		limitInt, _ := res[1].(int64)
		remainingInt, _ := res[2].(int64)
		resetInt, _ := res[3].(int64)

		return &RateLimitResult{
			Allowed:   allowedInt == 1,
			Limit:     int(limitInt),
			Remaining: int(remainingInt),
			Reset:     resetInt,
		}, nil
	}

	result, err := l.cb.Execute(operation)

	if err != nil {
		bucketIface, _ := l.fallbackBuckets.LoadOrStore(identifier, NewTokenBucket(fallbackLimit, fallbackLimit))
		bucket := bucketIface.(*TokenBucket)
		allowed := bucket.Allow()

		status := "blocked"
		if allowed {
			status = "allowed"
		}
		requestsTotal.WithLabelValues(status, "fallback").Inc()

		remaining := 0
		if allowed {
			remaining = 1
		}

		return &RateLimitResult{
			Allowed:   allowed,
			Limit:     fallbackLimit,
			Remaining: remaining,
			Reset:     0,
		}, nil
	}

	rateResult := result.(*RateLimitResult)

	status := "blocked"
	if rateResult.Allowed {
		status = "allowed"
	}
	requestsTotal.WithLabelValues(status, "redis").Inc()

	return rateResult, nil
}
