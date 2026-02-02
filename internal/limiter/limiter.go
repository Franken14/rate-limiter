package limiter

import (
	"context"
	_ "embed"
	"strconv"
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

// Limiter represents a sliding window rate limiter using Redis
type Limiter struct {
	client        *redis.Client
	script        *redis.Script
	limit         int
	window        time.Duration
	fallbackLimit int
	tokenBucket   *TokenBucket
	cb            *gobreaker.CircuitBreaker
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

func NewLimiter(client *redis.Client, limit int, window time.Duration, fallbackLimit int) *Limiter {
	// Pre-load the script into a Go-Redis script object
	redisScript := redis.NewScript(luaScript)

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
		limit:         limit,
		window:        window,
		fallbackLimit: fallbackLimit,
		tokenBucket:   NewTokenBucket(fallbackLimit, fallbackLimit), // burst = rate for simplicity
		cb:            gobreaker.NewCircuitBreaker(st),
	}
}

// RateLimitResult contains the status of the rate limit check
type RateLimitResult struct {
	Allowed   bool
	Limit     int
	Remaining int
	Reset     int64 // Unix timestamp in milliseconds
}

func (l *Limiter) Allow(ctx context.Context, identifier string) (*RateLimitResult, error) {
	start := time.Now()
	defer func() {
		requestLatency.Observe(time.Since(start).Seconds())
	}()

	// 1. Create a unique key for the user in Redis
	key := "ratelimit:" + identifier

	// 2. Prepare the arguments for Lua
	now := time.Now().UnixNano() / int64(time.Millisecond) // Current time in ms
	windowMS := l.window.Milliseconds()                    // Window in ms
	maxLimit := l.limit

	// Generate a unique member ID for the Sorted Set
	member := strconv.FormatInt(now, 10) + "-" + strconv.FormatInt(time.Now().UnixNano(), 10)

	// 3. Define the critical operation for Circuit Breaker
	operation := func() (interface{}, error) {
		// Run the script
		// Returns: [allowed, limit, remaining, reset]
		res, err := l.script.Run(ctx, l.client, []string{key}, now, windowMS, maxLimit, member).Slice()
		if err != nil {
			return nil, err
		}

		// Parse the result
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

	// 4. Execute via Circuit Breaker
	result, err := l.cb.Execute(operation)

	if err != nil {
		// Circuit Breaker is Open OR Redis failed
		// Fallback to Token Bucket
		allowed := l.tokenBucket.Allow()

		// Record metric for fallback
		status := "blocked"
		if allowed {
			status = "allowed"
		}
		requestsTotal.WithLabelValues(status, "fallback").Inc()

		// Approximate result for fallback
		remaining := 0
		if allowed {
			remaining = 1
		}

		return &RateLimitResult{
			Allowed:   allowed,
			Limit:     l.fallbackLimit,
			Remaining: remaining,
			Reset:     0, // Unknown
		}, nil
	}

	// 5. Success
	rateResult := result.(*RateLimitResult)

	// Record metric for Redis success
	status := "blocked"
	if rateResult.Allowed {
		status = "allowed"
	}
	requestsTotal.WithLabelValues(status, "redis").Inc()

	return rateResult, nil
}
