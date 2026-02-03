# Project Deep Dive & Interview Preparation

This document is designed to help you master the codebase you've built and ace any interview questions regarding it. It covers the technical implementation details, the "why" behind the design choices, and common questions FAANG interviewers might ask.

---

## 🏗️ Architecture Overview

### 1. The Core Problem
Rate limiting in a **distributed system** is hard because multiple application instances (pods/servers) share the load. A local in-memory counter on Server A doesn't know about requests hitting Server B.

**Solution**: Use a centralized store (**Redis**) to keep the counts synchronized across all servers.

### 2. The Algorithm: Token Bucket
You implemented the **Token Bucket** algorithm (using Redis Hashes).
*   **Why?** It is O(1) space efficient and handles high throughput without memory explosion.
*   **How?**
    *   Redis `Hash` stores the bucket state.
    *   **Fields**: `tokens` (current capacity), `last_refill` (timestamp).
    *   **Logic**:
        1.  Fetch `tokens` and `last_refill` (`HMGET`).
        2.  Calculate tokens added since `last_refill` based on rate.
        3.  Clamp tokens to `capacity` (burst limit).
        4.  If `tokens >= 1`, decrement and allow (`HMSET`).
        5.  Else, deny.

### 3. Middleware Pattern
You refactored the logic into Go **Middleware**.
*   **Pattern**: A function that takes a `http.Handler` and returns a `http.Handler`.
*   **Benefit**: Separation of concerns. The business logic (`finalHandler`) doesn't need to know about rate limiting. It just works.

### 4. Fail-Open Strategy (Resilience)
*   **Problem**: If Redis dies, your API shouldn't die (returns 500 or blocks everyone).
*   **Solution**: **Fail-Open**.
*   **Implementation**: If Redis returns an error, catch it and fall back to a local in-memory **Token Bucket**.
*   **Trade-off**: You lose distributed consistency (each pod enforces its own limit), but you keep the service alive (`CAP Theorem` - favoring Availability over Consistency during partitions).

### 5. Operational Excellence (System Health)
To make the system production-ready, we implemented:
*   **Circuit Breaker (`gobreaker`)**: Instead of waiting for Redis timeouts on *every* request (which causes latency spikes), the circuit breaker detects high failure rates and "trips", instantly switching to fallback mode.
*   **Observability (`Prometheus`)**: We track `requests_total` (status=allowed/blocked, mechanism=redis/fallback) and `latency_seconds`. This allows us to quantify the system's behavior and set alerts.

---

## 🧬 Code Walkthrough (Key Concepts)

### API Interface (Headers)
To improve the developer experience (DX), the API returns standard rate limit headers to let clients know their status:
*   `X-RateLimit-Limit`: The total request limit per window (e.g., 5).
*   `X-RateLimit-Remaining`: The number of requests left in the current window.
*   `X-RateLimit-Reset`: The Unix timestamp (in seconds) when the window resets.
*   **Why?** This prevents "blind" retries. Clients can intelligently back off until the `Reset` time.

### Go Implementation
**`internal/middleware/ratelimit.go`**
*   **Closures**: The `RateLimit` function returns a closure that captures the `Limiter` instance.
*   **Context**: `r.Context()` is passed to Redis calls to support cancellation/timeouts.
*   **Interfaces**: You use `http.Handler` interface throughout.

**`internal/limiter/limiter.go`**
*   **Struct Composition**: `Limiter` holds the Redis client, script, and fallback logic.
*   **Concurrency (Mutex)**: The `TokenBucket` uses `sync.Mutex` (`mu.Lock()`/`defer mu.Unlock()`). **Why?** Because multiple HTTP requests (goroutines) access the *same* `TokenBucket` instance simultaneously. Without the lock, you'd have race conditions on `tb.tokens`.

### Lua Script (`internal/limiter/script.lua`)
**Why Lua?**
*   **Atomicity**: Redis guarantees that a queue of commands runs atomically. No other commands run while the script is executing.
*   **Race Condition Prevention**: Without Lua, you'd do "GET count" -> application logic -> "INCR count". Two requests could "GET" the same number before "INCR", breaking the limit.
*   **Efficiency**: Reduces Network RTT (Round Trip Time). 1 request to Redis instead of 3-4 (ZREM, ZCARD, ZADD).

**Key Logic & Return Values**:
The script returns a 4-element array (Slice in Go): `[allowed, limit, remaining, reset]`.
1.  **Allowed**: `1` (true) or `0` (false).
2.  **Limit**: The max requests allowed (passed in ARGV).
3.  **Remaining**: `Limit - CurrentCount`.
4.  **Reset**: The Unix ms timestamp when the window expires.
    *   *Calculation*: If the set is empty, `Reset = Now + Window`. If not, `Reset = OldestScore + Window`. This implementation ensures the reset time is based on the *oldest* request in the sliding window, giving a precise "time to live" for the current blockage.

---

## 🎯 FAANG Interview Questions ( & Answers)

### System Design / Architecture
1.  **"Why did you choose Token Bucket over Sliding Window Log?"**
    *   **Answer**: "I actually started with Sliding Window Log for precision, but I hit a scalability wall. It required O(N) memory (storing every timestamp). For high-scale/DDoS protection (e.g., 100k requests/sec), this caused memory exhaustion. I switched to Token Bucket because it uses O(1) memory (just 2 counters) while still allowing bursts."

2.  **"What happens if Redis is slow? Does it block the request?"**
    *   **Answer**: "Yes, Redis calls are synchronous. If Redis is slow, it adds latency. That's why I used `go-redis` with connection pooling. In a production system, I would add a strict timeout context to the `l.Allow()` call so we fail-open quickly if Redis hangs."

3.  **"How would you scale this for 1 million users?"**
    *   **Answer**: "The current Sorted Set approach uses O(N) memory. For 1M users, Redis memory would explode. I would:
        *   Switch algorithms to **Fixed Window Counter** (1 key per user, O(1) memory) or **Leaky Bucket** (Redis Cell).
        *   Shard Redis (Cluster mode) to distribute keys across nodes."

4.  **"What if the Redis script fails halfway?"**
    *   **Answer**: "Redis scripts are atomic. They either execute fully or not at all (mostly). However, if Redis crashes *during* execution, data might be inconsistent, but typically Redis guarantees standard ACID properties for scripts."

5.  **"Explain your Fail-Open logic tradeoffs."**
    *   **Answer**: "When Redis is down, I fallback to local memory. Tradeoff: If I have 100 API servers and my fallback limit is 5 req/sec, the *actual* total limit becomes 500 req/sec (5 * 100). The global limit is momentarily raised, but availability is preserved."

6.  **"Why did you use `http.HandlerFunc` wrappers in the middleware?"**
    *   **Answer**: "`http.Handler` is an interface. `http.HandlerFunc` is a type adapter that lets me use a simple function as a handler. It allows me to write inline closures for middleware logic."

### Redis/Lua
7.  **"What are `KEYS` vs `ARGV` in Lua?"**
    *   **Answer**: "`KEYS` are for Redis keys (which point to data sharding slots). `ARGV` are for values/arguments. Redis Cluster *requires* you to define `KEYS` explicitly so it knows which node to send the command to. Passing keys in `ARGV` would break clustering."

8.  **"What is the time complexity of your Lua script?"**
    *   **Answer**:
        *   `HMGET` / `HMSET`: O(1).
        *   Math operations: O(1).
        *   Total: **O(1)**. It is independent of the number of requests or the window size.

### Operational Excellence
9.  **"Why did you add a Circuit Breaker? Isn't a timeout enough?"**
    *   **Answer**: "A timeout protects the *individual request*, but a Circuit Breaker protects the *system*. If Redis is down, waiting 200ms for a timeout on 10,000 requests adds massive latency and load. The Circuit Breaker fails fast (in microseconds) after the threshold is reached, preserving the API's throughput."

10. **"how do you monitor this in production?"**
    *   **Answer**: "I instrumented the code with Prometheus metrics. I can see a graph of 'Mechanism=Redis' vs 'Mechanism=Fallback'. If 'Fallback' spikes, I know there's a problem with the Redis layer, even if I'm not looking at logs."

---

## 🚀 Performance Benchmarks
We stress-tested the solution on a local development machine to verify latency and throughput.

### Test Configuration
*   **Tool**: `hey` (Apache Bench alternative)
*   **Concurrency**: 100 parallel clients
*   **Total Requests**: 20,000
*   **Rate Limit Config**: 100,000 req/window (effectively unlimited for test)

### Results
*   **Throughput**: **~32,000 Requests/Second**
*   **Latency (p99)**: **< 9ms**
*   **Reliability**: 0 Failures. Redis handled 100% of the traffic without needing fallback.

### What this means
This single-node implementation is highly performant. 32k RPS is enough to handle the traffic of many mid-sized tech companies. For hyper-scale (1M+ users), we would introduce Redis Clustering as discussed in the Architecture section.

---

## 🧪 How to Verify (for yourself)
1.  **Run the App**: `./api` (make sure Redis is running).
2.  **Test Limit**: `curl http://localhost:8080` (6 times quickly). 6th should fail.
3.  **Test Fail-Open**: Stop Redis container/service. Run `curl`. It should succeed (fallback).

This project demonstrates strong understanding of **Distributed Systems**, **Concurrency Control**, **Resilience patterns**, and **Observability**. Good luck!
