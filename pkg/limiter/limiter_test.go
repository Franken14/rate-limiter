package limiter

import (
	"context"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newTestLimiter(t *testing.T) (*Limiter, *miniredis.Miniredis) {
	s := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{
		Addr: s.Addr(),
	})

	// 5 reqs / 1 sec, fallback 2 req/sec
	l := NewLimiter(rdb, 5, time.Second, 2)
	return l, s
}

func TestLimiter_RedisNormal(t *testing.T) {
	l, _ := newTestLimiter(t)
	ctx := context.Background()
	ip := "127.0.0.1"

	// 5 requests should pass
	for i := 0; i < 5; i++ {
		res, err := l.Allow(ctx, ip)
		require.NoError(t, err)
		assert.True(t, res.Allowed)
		assert.Equal(t, 5-1-i, res.Remaining)
	}

	// 6th should fail
	res, err := l.Allow(ctx, ip)
	require.NoError(t, err)
	assert.False(t, res.Allowed)
}

func TestLimiter_FailOpen(t *testing.T) {
	s := miniredis.RunT(t)
	// Create client but then close server to simulate failure
	rdb := redis.NewClient(&redis.Options{
		Addr:        s.Addr(),
		MaxRetries:  0,
		DialTimeout: 10 * time.Millisecond,
	})

	// Fallback: 2 req/sec
	l := NewLimiter(rdb, 5, time.Second, 2)

	// STOP REDIS
	s.Close()

	ctx := context.Background()
	ip := "127.0.0.1"

	// Should still work due to fallback (2 requests allowed)
	res, err := l.Allow(ctx, ip)
	require.NoError(t, err, "Should not return error even if Redis is down")
	assert.True(t, res.Allowed, "Should be allowed by fallback")
	assert.Equal(t, 2, res.Limit, "Should report fallback limit")

	res, err = l.Allow(ctx, ip)
	require.NoError(t, err)
	assert.True(t, res.Allowed)

	// 3rd should fail (fallback limit is 2)
	res, err = l.Allow(ctx, ip)
	require.NoError(t, err)
	assert.False(t, res.Allowed, "Should be blocked by fallback limit")
}
