package limiter

import (
	"context"
	_ "embed"
	"time"

	"github.com/redis/go-redis/v9"
)

//go:embed script.lua
var luaScript string

// Limiter represents a sliding window rate limiter using Redis
type Limiter struct {
	client *redis.Client
	script *redis.Script // New field
	limit  int
	window time.Duration
}

func NewLimiter(client *redis.Client, limit int, window time.Duration) *Limiter {
	// Pre-load the script into a Go-Redis script object
	redisScript := redis.NewScript(luaScript)

	return &Limiter{
		client: client,
		script: redisScript,
		limit:  limit,
		window: window,
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
	// Returns: [allowed, limit, remaining, reset]
	result, err := l.script.Run(ctx, l.client, []string{key}, now, windowMS, maxLimit).Slice()
	if err != nil {
		return nil, err
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
