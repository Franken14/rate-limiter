package main

import (
	"fmt"
	"log"
	"net"
	"net/http"
	"strconv"
	"time"

	"github.com/Franken14/rate-limiter/internal/limiter"
	"github.com/redis/go-redis/v9"
)

func main() {
	// 1. Initialize Redis Client
	rdb := redis.NewClient(&redis.Options{
		Addr: "localhost:6379",
	})

	// 2. Initialize our Limiter
	// We allow 5 requests every 10 seconds
	l := limiter.NewLimiter(rdb, 5, 10*time.Second)

	// 3. Define the HTTP Handler
	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()

		// Use the IP address as the identifier
		ip := r.RemoteAddr
		host, _, err := net.SplitHostPort(r.RemoteAddr)
		if err == nil {
			ip = host
		}

		result, err := l.Allow(ctx, ip)
		if err != nil {
			http.Error(w, "Internal Server Error", http.StatusInternalServerError)
			return
		}

		w.Header().Set("X-RateLimit-Limit", strconv.Itoa(result.Limit))
		w.Header().Set("X-RateLimit-Remaining", strconv.Itoa(result.Remaining))
		w.Header().Set("X-RateLimit-Reset", strconv.FormatInt(result.Reset/1000, 10))

		if !result.Allowed {
			w.WriteHeader(http.StatusTooManyRequests)
			fmt.Fprint(w, "Rate limit exceeded. Try again later.")
			return
		}

		fmt.Fprint(w, "Request successful!")
	})

	// 4. Start the Server
	fmt.Println("Server starting on :8080...")
	log.Fatal(http.ListenAndServe(":8080", nil))
}
