package main

import (
	"fmt"
	"log"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/Franken14/rate-limiter/internal/limiter"
	"github.com/Franken14/rate-limiter/internal/middleware"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/redis/go-redis/v9"
)

func main() {
	var rdb redis.UniversalClient

	if clusterAddrs := os.Getenv("REDIS_CLUSTER_ADDRS"); clusterAddrs != "" {
		// Redis Cluster
		addrs := strings.Split(clusterAddrs, ",")
		rdb = redis.NewClusterClient(&redis.ClusterOptions{
			Addrs: addrs,
		})
		fmt.Println("Connected to Redis Cluster at:", addrs)
	} else {
		// Single Node
		addr := "localhost:6379"
		if envAddr := os.Getenv("REDIS_ADDR"); envAddr != "" {
			addr = envAddr
		}
		rdb = redis.NewClient(&redis.Options{
			Addr: addr,
		})
		fmt.Println("Connected to Single Redis at:", addr)
	}

	// We allow 5 requests every 10 seconds for limiter. Fallback: 5 req/sec.
	limit := getEnvAsInt("RATE_LIMIT", 5)
	windowSec := getEnvAsInt("RATE_LIMIT_WINDOW_SEC", 10)
	burst := getEnvAsInt("RATE_LIMIT_BURST", 5)

	l := limiter.NewLimiter(rdb, limit, time.Duration(windowSec)*time.Second, burst)

	// Create the rate limit middleware
	rateLimiterMiddleware := middleware.RateLimit(l)

	// Define the wrapper handler
	finalHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, "Request successful!")
	})

	// Wrap the handler with the middleware
	http.Handle("/", rateLimiterMiddleware(finalHandler))

	// Prometheus Metrics Endpoint
	http.Handle("/metrics", promhttp.Handler())

	fmt.Println("Server starting on :8080...")
	log.Fatal(http.ListenAndServe(":8080", nil))
}

func getEnvAsInt(key string, defaultVal int) int {
	if value, exists := os.LookupEnv(key); exists {
		if intValue, err := strconv.Atoi(value); err == nil {
			return intValue
		}
	}
	return defaultVal
}
