package limiter

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestTokenBucket_Allow(t *testing.T) {
	// 10 tokens/sec, capacity 10
	tb := NewTokenBucket(10, 10)

	// Consume 10 tokens immediately
	for i := 0; i < 10; i++ {
		assert.True(t, tb.Allow(), "Token %d should be allowed", i+1)
	}

	// 11th token should be rejected
	assert.False(t, tb.Allow(), "11th token should be rejected immediately")

	// Wait for 1 token to refill (0.1s)
	// Add a small buffer to ensure time passes
	time.Sleep(110 * time.Millisecond)

	assert.True(t, tb.Allow(), "Should allow 1 token after refill")
	assert.False(t, tb.Allow(), "Should reject subsequent token immediately")
}

func TestTokenBucket_Burst(t *testing.T) {
	// 1 token/sec, capacity 5
	tb := NewTokenBucket(1, 5)

	// Consume 5 (burst usage)
	for i := 0; i < 5; i++ {
		assert.True(t, tb.Allow())
	}

	assert.False(t, tb.Allow())

	// Wait for 2.5 seconds -> should refill 2 tokens
	time.Sleep(2500 * time.Millisecond)

	assert.True(t, tb.Allow())
	assert.True(t, tb.Allow())
	assert.False(t, tb.Allow())
}
