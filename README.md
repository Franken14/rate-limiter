# Distributed Rate Limiter

A distributed rate-limiting service built with Go and Redis. This project implements a sliding window algorithm to ensure accurate request throttling across multiple service instances.

## Overview
In a distributed environment, local in-memory rate limiting is insufficient because traffic is spread across multiple nodes. This project addresses this by using a centralized Redis store to synchronize state. It leverages Lua scripting to execute operations atomically, ensuring consistent enforcement even during concurrent request spikes.

## Features
- **Distributed Consistency**: Synchronizes rate limit state across all instances using Redis.
- **Atomic Operations**: Uses Lua scripts to prevent race conditions during concurrent access.
- **Fail-Open Design**: Includes an in-memory fallback mechanism (Token Bucket) to allow traffic at a reduced rate if Redis becomes unavailable.

## Tech Stack
- **Language**: Go
- **Datastore**: Redis (Sorted Sets)
- **Logic**: Lua Scripting

## Architecture

### Sliding Window Algorithm
The system uses a sliding window log to track requests. This offers higher precision compared to fixed window counters, smoothing out traffic bursts.

### Request Flow
1. **Interception**: The Go server intercepts incoming requests and identifies the client.
2. **Redis Check**: The server executes a Lua script in Redis to:
   - Remove expired timestamps.
   - Count valid requests in the current window.
   - Add the current timestamp if the limit hasn't been reached.
3. **Fallback Strategy**: If the Redis connection fails or times out, the system automatically switches to a local in-memory Token Bucket limiter. This ensures availability (fail-open) while still providing basic protection against abuse.

## Setup
1. **Prerequisites**:
   - Go 1.25+
   - Redis server
2. **Configuration**:
   - The application connects to Redis at `localhost:6379` by default.
   - Set the `REDIS_ADDR` environment variable to override the address (e.g., `export REDIS_ADDR=localhost:6379`).
3. **Run**:
   ```bash
   go mod download
   go build -o api cmd/api/main.go
   ./api
   ```