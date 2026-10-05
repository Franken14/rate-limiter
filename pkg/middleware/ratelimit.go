package middleware

import (
	"fmt"
	"net"
	"net/http"
	"strconv"

	"github.com/Franken14/rate-limiter/internal/limiter"
)

// RateLimit returns a middleware that rate limits requests using the provided Limiter.
func RateLimit(l *limiter.Limiter) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ctx := r.Context()

			// Extract IP
			ip := r.RemoteAddr
			host, _, err := net.SplitHostPort(r.RemoteAddr)
			if err == nil {
				ip = host
			}

			// Check Rate Limit
			result, err := l.Allow(ctx, ip)
			if err != nil {
				// Internal error (shouldn't happen with fail-open logic, but handled gracefully)
				http.Error(w, "Internal Server Error", http.StatusInternalServerError)
				return
			}

			// Set Headers
			w.Header().Set("X-RateLimit-Limit", strconv.Itoa(result.Limit))
			w.Header().Set("X-RateLimit-Remaining", strconv.Itoa(result.Remaining))
			w.Header().Set("X-RateLimit-Reset", strconv.FormatInt(result.Reset/1000, 10))

			// Check if Allowed
			if !result.Allowed {
				w.WriteHeader(http.StatusTooManyRequests)
				fmt.Fprint(w, "Rate limit exceeded. Try again later.")
				return
			}

			// Proceed
			next.ServeHTTP(w, r)
		})
	}
}
