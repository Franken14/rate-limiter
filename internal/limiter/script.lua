local key = KEYS[1]
local now = tonumber(ARGV[1])
local window = tonumber(ARGV[2])
local limit = tonumber(ARGV[3])

-- 2. Calculate the start of our current window
local window_start = now - window

-- 3. Remove all timestamps older than the window_start
redis.call('ZREMRANGEBYSCORE', key, 0, window_start)

-- 4. Count how many requests are left in the set
local current_usage = redis.call('ZCARD', key)

-- 5. Calculate stats
local remaining = limit - current_usage
local reset = 0

local oldest_entry = redis.call('ZRANGE', key, 0, 0, 'WITHSCORES')
if #oldest_entry > 0 then
    local oldest_score = tonumber(oldest_entry[2])
    reset = oldest_score + window
else
    reset = now + window
end

if current_usage < limit then
    redis.call('ZADD', key, now, now)
    redis.call('EXPIRE', key, window)
    
    remaining = remaining - 1
    return {1, limit, remaining, reset}
else
    return {0, limit, remaining, reset}
end