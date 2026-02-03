package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/Franken14/rate-limiter/internal/limiter"
	"github.com/redis/go-redis/v9"
)

func main() {
	// Flags for configuration
	redisAddr := flag.String("redis", "localhost:6379", "Redis address")
	concurrency := flag.Int("concurrency", 50, "Number of concurrent workers")
	duration := flag.Duration("duration", 10*time.Second, "Test duration")
	rateLimit := flag.Int("limit", 1000, "Rate limit")
	// Use a larger window to allow building up a large ZSET
	window := flag.Duration("window", 60*time.Second, "Rate limit window")
	flag.Parse()

	// Setup Redis
	rdb := redis.NewClient(&redis.Options{
		Addr: *redisAddr,
	})
	defer rdb.Close()

	// Ping check
	ctx := context.Background()
	if err := rdb.Ping(ctx).Err(); err != nil {
		log.Fatalf("Failed to connect to Redis: %v", err)
	}

	// Flush DB to ensure clean state
	rdb.FlushDB(ctx)

	// Initialize Limiter
	l := limiter.NewLimiter(rdb, *rateLimit, *window, 100)

	log.Printf("Starting benchmark with: concurrency=%d, limit=%d, window=%v, duration=%v", *concurrency, *rateLimit, *window, *duration)

	var (
		successCount int64
		blockedCount int64
		errorCount   int64
		totalLatency int64 // in microseconds
	)

	benchCtx, cancel := context.WithTimeout(context.Background(), *duration)
	defer cancel()

	var wg sync.WaitGroup
	start := time.Now()

	for i := 0; i < *concurrency; i++ {
		wg.Add(1)
		go func(workerID int) {
			defer wg.Done()
			// Use a single key for maximum stress on the sorted set
			identifier := "benchmark_user"

			for {
				select {
				case <-benchCtx.Done():
					return
				default:
					reqStart := time.Now()
					res, err := l.Allow(context.Background(), identifier)
					latency := time.Since(reqStart).Microseconds()
					atomic.AddInt64(&totalLatency, latency)

					if err != nil {
						atomic.AddInt64(&errorCount, 1)
					} else if res.Allowed {
						atomic.AddInt64(&successCount, 1)
					} else {
						atomic.AddInt64(&blockedCount, 1)
					}
				}
			}
		}(i)
	}

	wg.Wait()
	elapsed := time.Since(start).Seconds()

	totalReqs := successCount + blockedCount + errorCount
	var rps float64
	if elapsed > 0 {
		rps = float64(totalReqs) / elapsed
	}
	var avgLatency float64
	if totalReqs > 0 {
		avgLatency = float64(totalLatency) / float64(totalReqs)
	}

	fmt.Println("\n--- Benchmark Results ---")
	fmt.Printf("Total Requests: %d\n", totalReqs)
	fmt.Printf("Elapsed Time: %.2fs\n", elapsed)
	fmt.Printf("RPS: %.2f\n", rps)
	fmt.Printf("Avg Latency: %.2f µs\n", avgLatency)
	fmt.Printf("Allowed: %d\n", successCount)
	fmt.Printf("Blocked: %d\n", blockedCount)
	fmt.Printf("Errors: %d\n", errorCount)

	// Check Memory usage
	memInfo, err := rdb.Info(ctx, "memory").Result()
	if err == nil {
		fmt.Println("\n--- Redis Memory Info ---")
		lines := strings.Split(memInfo, "\r\n")
		for _, line := range lines {
			if strings.HasPrefix(line, "used_memory_human") || strings.HasPrefix(line, "used_memory_peak_human") {
				fmt.Println(line)
			}
		}
	}
}
