-- Token Bucket Algorithm
-- key: ratelimit:{id} (Hash)
-- fields: tokens, last_refill (microseconds)

local key = KEYS[1]
local now = tonumber(ARGV[1])        -- Current time in microseconds/millisecond
local rate = tonumber(ARGV[2])       -- Tokens per second (or unit time)
local capacity = tonumber(ARGV[3])   -- Max tokens (burst)
local requested = tonumber(ARGV[4])  -- Tokens to consume (default 1)

-- 1. Fetch current state
local state = redis.call('HMGET', key, 'tokens', 'last_refill')
local tokens = tonumber(state[1])
local last_refill = tonumber(state[2])

-- 2. Initialize if missing
if tokens == nil then
    tokens = capacity
    last_refill = now
end

-- 3. Refill tokens
if last_refill < now then
    local delta = (now - last_refill) / 1000000 -- Convert microseconds to seconds
    local filled = delta * rate
    tokens = math.min(capacity, tokens + filled)
    last_refill = now
end

-- 4. Check limit
local allowed = 0
local remaining = math.floor(tokens)
local reset = 0 -- Token bucket doesn't have a distinct "reset" time like fixed window, but we can approximate time to full

if tokens >= requested then
    allowed = 1
    tokens = tokens - requested
    remaining = math.floor(tokens)
end

-- 5. Calculate time to full refill (for informational purposes)
if rate > 0 then
    local needed = capacity - tokens
    reset = math.ceil(needed / rate) -- Seconds until full
end

-- 6. Save state
redis.call('HMSET', key, 'tokens', tokens, 'last_refill', last_refill)
-- Expire the key if idle for a long time (e.g., time to full + 1 hour) to save memory
redis.call('EXPIRE', key, 86400) -- Default 24h expiration

return {allowed, capacity, remaining, reset}