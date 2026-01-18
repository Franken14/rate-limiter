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

func (l *Limiter) Allow(ctx context.Context, identifier string) (bool, error) {
	// 1. Create a unique key for the user in Redis
	key := "ratelimit:" + identifier

	// 2. Prepare the arguments for Lua (Convert Go types to something Redis understands)
	now := time.Now().UnixNano() / int64(time.Millisecond) // Current time in ms
	windowMS := l.window.Milliseconds()                    // Window in ms
	maxLimit := l.limit

	// 3. Run the script!
	// .Run() handles the "SHA-1 optimization" for you.
	// It sends the SHA hash; if Redis doesn't have it, it sends the full script.
	result, err := l.script.Run(ctx, l.client, []string{key}, now, windowMS, maxLimit).Int()
	if err != nil {
		return false, err
	}

	// 4. Return true if Lua returned 1, false if 0
	return result == 1, nil
}
