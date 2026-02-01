package limiter

import (
	"context"
	_ "embed"
	"strconv"
	"sync"
	"time"

	"github.com/redis/go-redis/v9"
)

//go:embed script.lua
var luaScript string

// Limiter represents a sliding window rate limiter using Redis
type Limiter struct {
	client        *redis.Client
	script        *redis.Script
	limit         int
	window        time.Duration
	fallbackLimit int
	tokenBucket   *TokenBucket
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

	return &Limiter{
		client:        client,
		script:        redisScript,
		limit:         limit,
		window:        window,
		fallbackLimit: fallbackLimit,
		tokenBucket:   NewTokenBucket(fallbackLimit, fallbackLimit), // burst = rate for simplicity
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
	// 1. Create a unique key for the user in Redis
	key := "ratelimit:" + identifier

	// 2. Prepare the arguments for Lua (Convert Go types to something Redis understands)
	now := time.Now().UnixNano() / int64(time.Millisecond) // Current time in ms
	windowMS := l.window.Milliseconds()                    // Window in ms
	maxLimit := l.limit

	// 3. Run the script!
	// Generate a unique member logic to prevent overwrites within the same ms
	// We use the full nanosecond timestamp + random suffix or just the pointer address if safe?
	// Easiest is just RandomInt or UUID-like.
	// Since we don't have a UUID lib, we can use now + pseudo-random.
	// But `now` is ms.
	member := strconv.FormatInt(now, 10) + "-" + strconv.FormatInt(time.Now().UnixNano(), 10)

	// Returns: [allowed, limit, remaining, reset]
	result, err := l.script.Run(ctx, l.client, []string{key}, now, windowMS, maxLimit, member).Slice()
	if err != nil {
		// Log the error if needed, but fail open
		// Ideally log failure safely

		// Fallback to Token Bucket
		allowed := l.tokenBucket.Allow()

		// Approximate result for fallback
		remaining := 0
		if allowed {
			remaining = 1 // At least one left? Or just hide it.
		}

		return &RateLimitResult{
			Allowed:   allowed,
			Limit:     l.fallbackLimit,
			Remaining: remaining,
			Reset:     0, // Unknown
		}, nil
	}

	// 4. Parse the result
	// Note: go-redis returns Lua numbers as int64
	allowedInt, _ := result[0].(int64)
	limitInt, _ := result[1].(int64)
	remainingInt, _ := result[2].(int64)
	resetInt, _ := result[3].(int64)

	return &RateLimitResult{
		Allowed:   allowedInt == 1,
		Limit:     int(limitInt),
		Remaining: int(remainingInt),
		Reset:     resetInt,
	}, nil
}
