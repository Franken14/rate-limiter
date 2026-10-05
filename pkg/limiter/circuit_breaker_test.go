package limiter

import (
	"context"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/assert"
)

func TestCircuitBreakerFailOpen(t *testing.T) {
	// 1. Setup MiniRedis
	mr, err := miniredis.Run()
	if err != nil {
		t.Fatal(err)
	}
	defer mr.Close()

	rdb := redis.NewClient(&redis.Options{
		Addr: mr.Addr(),
	})

	// 2. Initialize Limiter
	// Limit: 10, Window: 1s, Fallback: 2
	l := NewLimiter(rdb, 10, 1*time.Second, 2)

	// 3. Verify Happy Path (Redis UP)
	res, err := l.Allow(context.Background(), "user1")
	assert.NoError(t, err)
	assert.True(t, res.Allowed)
	assert.Equal(t, 10, res.Limit) // Redis limit

	// 4. Simulate Redis Failure
	// Close miniredis to cause connection errors
	mr.Close()

	// 5. Trip the Circuit Breaker
	// Our CB settings: Trip after 3 requests with >60% failure ratio
	// Make 5 requests to ensure it trips. These will consume the fallback tokens!
	for i := 0; i < 5; i++ {
		l.Allow(context.Background(), "user1")
	}

	// Refill the bucket
	// Fallback limit is 2 req/sec, so 1100ms should give us back 2 tokens.
	time.Sleep(1100 * time.Millisecond)

	// 6. Verify Fail-Open (Circuit is now Open)
	// The Limiter should now use the Fallback TokenBucket
	// Fallback capacity is 2.

	// Req 1: Allowed (Token Bucket)
	res, err = l.Allow(context.Background(), "user1")
	assert.NoError(t, err)
	assert.True(t, res.Allowed)
	assert.Equal(t, 2, res.Limit) // Fallback limit

	// Req 2: Allowed
	res, _ = l.Allow(context.Background(), "user1")
	assert.True(t, res.Allowed)

	// Req 3: Blocked (Token Bucket Empty)
	res, _ = l.Allow(context.Background(), "user1")
	assert.False(t, res.Allowed)
}
