# Distributed Rate Limiter (Go + Redis + Lua)

A high-performance, distributed rate-limiting service built with **Go** and **Redis**. This project implements a **Sliding Window Log** algorithm to ensure 100% accuracy in request throttling across multiple service instances.

## 🚀 The Engineering Challenge: Distributed Consistency
In a distributed microservices environment, local in-memory rate limiting fails because traffic is spread across multiple nodes. A client could bypass limits by hitting different server instances. 

**This project solves two critical challenges:**
1. **Consistency:** Using a centralized Redis store to synchronize state across all Go service instances.
2. **Race Conditions:** Utilizing **embedded Lua scripts** inside Redis to execute "Clean-Count-Update" operations atomically, preventing over-limit bursts during concurrent request spikes.

## 🛠 Tech Stack
- **Language:** Go 1.23+ (Standard Library + `go-redis`)
- **Datastore:** Redis 7.0+ (Sorted Sets for Sliding Window)
- **Logic:** Lua Scripting (for Atomic Transactions)
- **Env:** WSL2 / Ubuntu

## 📐 Architecture & System Design
The system uses the **Sliding Window Log** algorithm, which provides higher precision than the standard Fixed Window approach.

### Request Flow
1. **Middleware Interception:** The Go server intercepts an incoming request and identifies the user (via IP or API Key).
2. **Atomic Lua Execution:** The server calls a pre-loaded Lua script in Redis.
   - `ZREMRANGEBYSCORE`: Removes expired timestamps outside the current window.
   - `ZCARD`: Counts the remaining valid requests.
   - `ZADD`: Adds the current timestamp if the count is below the threshold.
3. **Decision:** The script returns a boolean. The Go server either allows the request or returns `HTTP 429 Too Many Requests`.



## ⚡ Performance Optimizations
- **Script Pre-loading:** The Lua script is loaded into Redis memory at startup. Subsequent calls use the **SHA-1 hash**, reducing network payload size and parsing overhead.
- **Connection Pooling:** Utilizes a persistent Redis connection pool to minimize TCP handshake latency.

## 🛠 Setup & Installation
1. **Prerequisites:**
   - Go 1.23+ installed in WSL2.
   - Redis server running (`sudo service redis-server start`).
2. **Run the Application:**
   ```bash
   go mod download
   go run main.go