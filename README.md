# Distributed Rate Limiter
> *A high-precision, distributed rate limiting service capable of handling **32,000+ RPS** with <9ms latency.*

[![Go](https://img.shields.io/badge/Language-Go-blue.svg)](https://golang.org/)
[![Redis](https://img.shields.io/badge/Database-Redis-red.svg)](https://redis.io/)
[![Prometheus](https://img.shields.io/badge/Observability-Prometheus-orange.svg)](https://prometheus.io/)

## Overview
Building a rate limiter for a single server is straightforward. Building one that works accurately across a distributed cluster—while surviving network partitions and cascading failures—is a different challenge.

I implemented this **Distributed Token Bucket** algorithm using Redis to ensure scalability and reliability. I designed it with a "Fail-Open" philosophy, ensuring the API never goes down just because the rate limiter service is unreachable.

## Key Features
*   **Scalable Throttling**: I used the Token Bucket algorithm (via Redis Hashes) to achieve O(1) memory and time complexity, regardless of the limit size.
*   **Operational Resilience**:
    *   **Circuit Breaker**: Detects Redis outages or latency spikes and "trips" instantly to protect the system.
    *   **Fail-Open Fallback**: Gracefully degrades to a local in-memory Token Bucket when Redis is unavailable.
*   **Observability**: Fully instrumented with Prometheus metrics (`requests_total`, `latency`) to visualize system health in real-time.
*   **Concurrency Safe**: Lua scripting ensures **atomicity** for all Check-Then-Act operations, preventing race conditions under high load.

## Architecture
The system consists of a Go middleware layer that intercepts requests and coordinates with a centralized Redis cluster.

| Component | Responsibility | Performance |
|-----------|----------------|-------------|
| **Middleware** | Intercepts requests, coordinates context/timeouts | <1ms overhead |
| **Circuit Breaker** | Monitors Redis health; trips on 3 failures | - |
| **Redis (Lua)** | Executes atomic token bucket logic | O(1) |
| **Fallback** | Local memory token bucket (when Redis is down) | O(1) |

> **Performance**: In my local stress tests, this system handled **32,300 req/sec** with p99 latency of 9ms.

## Required Reading (Engineering Depth)
I wrote these documents to demonstrate the engineering rigor behind the project:

- [**Engineering Journal**](./ENGINEERING_JOURNAL.md): A chronicle of the trade-offs I made (e.g., Consistency vs Availability), alternatives I rejected, and "War Stories" about bugs I encountered.

## Getting Started
The easiest way to run the service is with Docker Compose.

### Quick Start
```bash
# 1. Clone the repo
git clone https://github.com/Franken14/rate-limiter.git

# 2. Start the stack (API + Redis)
docker-compose up --build
```
The API is now running at `http://localhost:8080`.

### Verify it works
Hit the API. You will see the standard Rate Limit headers:
```bash
curl -v http://localhost:8080
```
**Response Headers**:
```http
X-RateLimit-Limit: 10
X-RateLimit-Remaining: 9
X-RateLimit-Reset: 1709401234
```

### Check Metrics
Visit [http://localhost:8080/metrics](http://localhost:8080/metrics) to see Prometheus metrics.
```text
rate_limit_requests_total{mechanism="redis",status="allowed"} 1
```

## Configuration
Environment variables control the behavior:

| Variable | Default | Description |
|----------|---------|-------------|
| `REDIS_ADDR` | `localhost:6379` | Address of the Redis instance |
| `RATE_LIMIT` | `5` | Max requests per window |
| `RATE_LIMIT_WINDOW_SEC` | `10` | Frequency window size (seconds) |
| `RATE_LIMIT_BURST` | `5` | Fallback bucket capacity |

## Testing & Verification
### Unit Tests
```bash
go test ./...
```
### Stress Test
Requires [`hey`](https://github.com/rakyll/hey).
```bash
# Run with high limits
export RATE_LIMIT=100000
go run cmd/api/main.go &
hey -n 20000 -c 100 http://localhost:8080/
```

## License
MIT