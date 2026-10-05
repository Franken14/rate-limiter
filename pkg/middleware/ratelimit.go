package middleware

import (
	"fmt"
	"net"
	"net/http"
	"strconv"
	"time"

	"github.com/Franken14/rate-limiter/pkg/limiter"
)

// Extractor is a function that extracts the client identifier, and optionally returns a dynamic limit and window.
// If limit is 0, the limiter's default limit is used.
type Extractor func(r *http.Request) (identifier string, limit int, window time.Duration)

// DefaultIPExtractor extracts the IP address and uses default limits.
func DefaultIPExtractor(r *http.Request) (string, int, time.Duration) {
	ip := r.RemoteAddr
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err == nil {
		ip = host
	}
	return ip, 0, 0
}

// RateLimit returns a middleware that rate limits requests using the provided Limiter.
// It accepts an optional Extractor. If none is provided, DefaultIPExtractor is used.
func RateLimit(l *limiter.Limiter, extractors ...Extractor) func(http.Handler) http.Handler {
	ext := DefaultIPExtractor
	if len(extractors) > 0 {
		ext = extractors[0]
	}

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ctx := r.Context()
			identifier, customLimit, customWindow := ext(r)

			var result *limiter.RateLimitResult
			var err error

			if customLimit > 0 && customWindow > 0 {
				result, err = l.AllowDynamic(ctx, identifier, customLimit, customWindow)
			} else {
				result, err = l.Allow(ctx, identifier)
			}

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
