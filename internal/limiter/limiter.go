package limiter

import (
	"context"
	_ "embed"
	"time"

	"github.com/redis/go-redis/v9"
)

var luaScript string

// Limiter represents a sliding window rate limiter using Redis
type Limiter struct {
	client *redis.Client
	limit  int
	window time.Duration
}

func NewLimiter(client *redis.Client, limit int, window time.Duration) *Limiter {
	return &Limiter{
		client: client,
		limit:  limit,
		window: window,
	}
}

func (l *Limiter) Allow(ctx context.Context, identifier string) (bool, error) {
	// We will fill this logic in next
	return true, nil
}
