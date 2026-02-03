# Challenges in Code: The "Struggle" Stories

This document catalogs the specific errors, wrong assumptions, and bugs encountered during development. Use these stories to answer "Tell me about a time you faced a difficult bug" or "What was the hardest part of this project?"

---

## 🏗️ Challenge 1: The "Shared State" Test Failure
### The Context
I implemented a **Circuit Breaker** to fallback to an in-memory Token Bucket when Redis fails. I wrote a unit test `TestCircuitBreakerFailOpen` to verify this.

### The Bug
My test logic was:
1.  Simulate Redis failure.
2.  Make 5 requests to trip the circuit breaker.
3.  Make another request and assert `Allowed = true` (expecting the fallback bucket to handle it).

**Result**: The test failed with `Allowed = false`.

### The Incorrect Assumption
I assumed the "Circuit Tripping" requests (step 2) were just "failures" that were discarded.

### The Realization
I realized that my code attempts the fallback token bucket *every time* Redis fails.
*   During step 2 (tripping the circuit), those 5 requests failed Redis but *consumed* tokens from the fallback bucket!
*   By the time I reached step 3 (verification), the fallback bucket was empty.

### The Fix
I added a `time.Sleep` before the verification step to allow the Token Bucket to refill its tokens.
```go
// Refill the bucket
time.Sleep(1100 * time.Millisecond) // Wait for tokens to regenerate
```
**Lesson**: When testing stateful systems (like token buckets), always account for the side effects of your "setup" steps.

---

## 🐛 Challenge 2: The Redis "Unique Member" Race Condition
### The Context
I used a Redis Sorted Set (`ZADD`) to store request timestamps.
`ZADD key score member` where `score` = timestamp, `member` = timestamp.

### The Bug
In my local integration tests, I sent 10 requests in a loop. I expected 10 entries in Redis. I only got 1.

### The Incorrect Assumption
I assumed `timestamp` (in milliseconds) was unique enough for a literal generic key.

### The Realization
Computers are fast. All 10 requests happened in the same millisecond.
*   Request 1: `ZADD key 1000 "1000"`
*   Request 2: `ZADD key 1000 "1000"` (Redis sees this as an update to the existing member, not a new entry).

### The Fix
I changed the member to be `timestamp-uniqueID` (e.g., `timestamp-nanoTime` or `timestamp-random`).
**Lesson**: `Set` data structures require uniqueness. Time is not a unique identifier in high-concurrency environments.

---

## 🛡️ Challenge 3: Atomic "Check-Then-Act" with Redis
### The Context
I needed to check if `count < limit`, and if so, increment it.

### The "Naive" Code (Mental Draft)
```go
count := redis.Get(key)
if count < limit {
    redis.Incr(key)
    return true
}
return false
```

### The Flaw
This is a classic race condition. Two requests can both read `count = 4` at the same time. Both enter the `if` block. Both increment. Count becomes 6 (Limit exceeded).

### The Solution (Discovery)
I researched "distributed locking" but found it too heavy/slow for a rate limiter.
I found **Lua scripting** in Redis.
*   "Scripts execute atomically in Redis. No other command can run while a script is executing."
*   This allowed me to bundle the `Check` and the `Act` into a single atomic operation without managing external locks.

---

## 📉 Challenge 4: Memory Explosion with Sliding Window
### The Context
My Sliding Window Log stores *every* request timestamp to be precise.

### The Constraint
I realized: If a user has a limit of 1,000,000 requests/hour, I am storing 1 million entries in Redis for that user.
`1M * 20 bytes ≈ 20MB` per user. 
For 1000 users, that's 20GB of RAM!

### The "Fix" (Tradeoff)
I didn't "fix" it in code, but I documented the **Tradeoff**.
*   For low-limit APIs (e.g., failed login attempts, strict payment limits), this algorithm is perfect.
*   For high-volume APIs (DDoS protection), I noted that I would switch to **Fixed Window** or **Token Bucket** (O(1) memory) despite their lower precision.
**Lesson**: There is no "perfect" algorithm. Engineering is about choosing the right tradeoff for the specific use case.
