# Engineering Journal: Challenges & Design Decisions

This document chronicles the engineering journey of building this distributed rate limiter. It explicitly highlights the constraints faced, tradeoffs made, and alternatives rejected. Use this to demonstrate "Engineering Depth" in your interviews.

---

## 🏗️ 1. Core Architecture: Why Redis?
### The Choice
I chose **Redis** as the centralized state store.
### The Challenge
In a distributed system with multiple API instances (pods), a local in-memory counter `map[string]int` is useless. User A might hit Pod 1 (Count: 1) and then Pod 2 (Count: 1), effectively doubling their limit.
### Alternatives Considered
1.  **Sticky Sessions (Load Balancer)**:
    *   *Idea*: Force User A to always hit Pod 1.
    *   *Rejected*: Causes "Hot Spotting" if one user spams the system. Limits autoscaling flexibility.
2.  **Gossip Protocol**:
    *   *Idea*: Pods talk to each other to sync counts.
    *   *Rejected*: Too much complexity and network chatter for a simple rate limiter. Even eventual consistency is hard here.
3.  **Database (Postgres/MySQL)**:
    *   *Idea*: Store counts in a SQL table.
    *   *Rejected*: Too slow. Disk I/O latency (10ms+) adds overhead to every request. Redis is in-memory (<1ms).

---

## 🧮 2. Algorithm: Sliding Window Log vs. Others
### The Choice
I implemented **Sliding Window Log** (Sorted Sets).
### The Challenge
My requirement was strict accuracy (e.g., "Max 5 requests in *any* 10-second period").
### The Tradeoff
This algorithm stores *every* timestamp, so it consumes O(N) memory per user.
*   *Observation*: If a user is allowed 10,000 req/sec, I'm storing 10,000 timestamps. That's expensive.
### Alternatives Considered
1.  **Fixed Window Counter**:
    *   *Pros*: O(1) memory (just one integer).
    *   *Cons*: The "Edge Case" problem. If I allow 100 req/min, a user can send 100 requests at 10:00:59 and 100 at 10:01:01. That's 200 requests in 2 seconds, effectively bypassing the limit.
2.  **Token Bucket**:
    *   *Pros*: Great for allowing "Bursts" while maintaining an average rate. O(1) memory.
    *   *Why I didn't use it for Redis*: Implementing a distributed Token Bucket in Redis requires careful locking or Lua. Sliding Window was more intuitive for my "strict window" requirement, though Token Bucket is arguably more robust for high-scale systems.

---

## 🏎️ 3. Concurrency: The Race Condition
### The Challenge
I encountered a classic "Check-Then-Act" race condition.
*   Flow: `GET usage` (returns 9) -> Check `9 < 10` (True) -> `INCR usage` (becomes 10).
*   Concurrent Issue: Two requests run this flow at the exact same nanosecond. Both see `9`. Both `INCR`. Both pass. The usage becomes 11 (Limit Exceeded).
### The Solution
**Lua Scripting**.
*   Redis runs Lua scripts *atomically*. It blocks other commands until the script finishes. This effectively serializes the logic for that specific key without complex distributed locks.

---

## 🛡️ 4. Resilience: The "Fail-Open" Decision
### The Challenge
What happens if Redis crashes?
*   *Option A (Fail-Closed)*: Return `500 Internal Server Error`. The API goes down with Redis.
*   *Option B (Fail-Open)*: Allow all traffic. The API stays up, but we lose protection.
### My Choice
**Fail-Open with In-Memory Fallback**.
*   I didn't just "allow all." I fell back to a local **Token Bucket**.
*   *Tradeoff*: Distributed enforcement is lost (each pod has its own limit), but the system remains available *and* somewhat protected.
"I prioritized **Availability** over **Consistency** (CAP theorem) during a partition event. It's better to let a few extra requests through than to bring down the payment processing system just because the rate limiter cache is down."

---

## 🧑‍💻 5. User Experience: Informative Headers
### The Challenge
A rate limiter that silently blocks requests (returning just `429 Too Many Requests`) is frustrating. Developers don't know *why* they were blocked or *when* they can retry.
### The Solution
I added standard HTTP headers to every response (even successful ones):
*   `X-RateLimit-Limit`: Defines the contract (e.g., "You get 5 requests").
*   `X-RateLimit-Remaining`: Acts as a fuel gauge ("You have 2 left").
*   `X-RateLimit-Reset`: The exact time to retry.
### Why this matters
This turns the rate limiter from a "black box" into a cooperative system. Clients can write smarter code that sleeps exactly until `Reset` time, preventing unnecessary load on my server from blind retries.

---

## 🐛 6. The "Unique Member" Bug
### The Challenge
During testing `miniredis`, I found that using `ZADD key timestamp timestamp` failed when tests ran fast.
*   *Root Cause*: Redis Sorted Sets require unique members. If two requests arrived at the same millisecond `17000100`, the second `ZADD` effectively did nothing (it just updated the score of the existing member).
### The Fix
I updated the design to store `timestamp-uniqueID` as the member.
*   *Lesson*: Always scrutinize data structures for uniqueness requirements, especially when using time as a value.

---

## 🔮 7. Future Improvements (What I'd do next)
1.  **Redis Cluster Support**: Currently, I rely on a single Redis instance. For web-scale, I'd need sharding. I'd have to ensure my Lua script calls pass `KEYS` explicitly so the proxy knows which shard to hit.
2.  **Circuit Breaker**: If Redis is timing out, I shouldn't keep trying to Dial it for every request. A Circuit Breaker pattern would stop trying Redis for 30s after 5 failures, instantly switching to fallback.

---

## 🛠️ 8. Operational Excellence: Why Circuit Breaker & Prometheus?
### The Choice
I added **Circuit Breaker** (gobreaker) and **Prometheus metrics** late in the development cycle.
### The Challenge
*   **Circuit Breaker**: During load testing, when I stopped Redis, the API latency spiked because every request waited for a TCP timeout before failing over to the in-memory bucket.
    *   *Solution*: The Circuit Breaker wraps the Redis call. After 3 failures, it "trips" and immediately fails subsequent requests (falling back to memory) without attempting to reach Redis. This reduced 99th percentile latency from 200ms (timeout) to <1ms (bucket check) during outages.
*   **Observability**: I had no way to know if the rate limiter was actually working in production without grepping logs.
    *   *Solution*: I exposed a `/metrics` endpoint. Now I can track:
        *   `rate_limit_requests_total{status="blocked", mechanism="redis"}`
        *   `rate_limit_requests_total{status="allowed", mechanism="fallback"}`
    *   *Why*: This proves the system is "Observable." I can set alerts on "High Fallback Rate" (indicating Redis issues) or "High Block Rate" (indicating a potential DDoS).

---

## 📉 9. Verification: Stress Testing "Expectation vs Reality"
### The Expectation
I worried that my robust but complex logic (Middleware -> Gobreaker -> Redis -> Lua -> Response) would add too much overhead, potentially capping out at 1-2k request/sec.

### The Reality (Stress Test)
I ran a stress test with 100 concurrent workers against the local API.
*   **Result**: 32,300+ requests per second.
*   **Optimization**: This confirmed that my decision to use **Connection Pooling** (via `go-redis`) and **Lua Scripts** (reducing network round-trips) was correct. The bottleneck was likely the Docker network bridge, not the code itself.

---

## 🔄 10. Migration: From Sliding Window to Token Bucket
### The Scaling Wall
During a stress test with a simulated limit of 100,000 requests/minute, I observed infinite memory growth.
*   **Observation**: The Sliding Window Log implementation stores a timestamp for *every* request in the window.
*   **Impact**: For 100k requests, Redis stored ~16MB of data per user. This is unsustainable for a production system.

### The Pivot
I decided to migrate to the **Token Bucket** algorithm.
*   **Why**: Token Bucket only stores 2 integers (`tokens` and `last_refill`), regardless of how many requests are allowed.
*   **Result**: 
    -   Memory usage dropped from **16MB** to **~2MB** (base Redis overhead) per user key.
    -   Space complexity improved from **O(N)** to **O(1)**.
    -   Throughput improved slightly (+7%).

### The Tradeoff Revisit
In Section 2, I originally chose Sliding Window for "Action precision". I have now prioritized "Scalability" over strict millisecond-level window boundaries. The Token Bucket is "good enough" for almost all rate limiting use cases and prevents the system from crashing under load.
