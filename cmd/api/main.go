package main

import (
	"fmt"
	"log"
	"net/http"
	"os"
	"time"

	"github.com/Franken14/rate-limiter/internal/limiter"
	"github.com/Franken14/rate-limiter/internal/middleware"
	"github.com/redis/go-redis/v9"
)

func main() {
	addr := "localhost:6379"
	if envAddr := os.Getenv("REDIS_ADDR"); envAddr != "" {
		addr = envAddr
	}

	rdb := redis.NewClient(&redis.Options{
		Addr: addr,
	})

	// We allow 5 requests every 10 seconds for limiter. Fallback: 5 req/sec.
	l := limiter.NewLimiter(rdb, 5, 10*time.Second, 5)

	// Create the rate limit middleware
	rateLimiterMiddleware := middleware.RateLimit(l)

	// Define the wrapper handler
	finalHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, "Request successful!")
	})

	// Wrap the handler with the middleware
	http.Handle("/", rateLimiterMiddleware(finalHandler))

	fmt.Println("Server starting on :8080...")
	log.Fatal(http.ListenAndServe(":8080", nil))
}
