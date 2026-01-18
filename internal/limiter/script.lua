-- 1. Define variables from arguments
local key = KEYS[1]
local now = tonumber(ARGV[1])
local window = tonumber(ARGV[2])
local limit = tonumber(ARGV[3])

-- 2. Calculate the start of our current window
local window_start = now - window

-- 3. Remove all timestamps older than the window_start
-- ZREMRANGEBYSCORE key min max
redis.call('ZREMRANGEBYSCORE', key, 0, window_start)

-- 4. Count how many requests are left in the set
local current_usage = redis.call('ZCARD', key)

-- 5. Check if we are under the limit
if current_usage < limit then
    -- Add the current request timestamp
    -- ZADD key score member
    redis.call('ZADD', key, now, now)
    
    -- Set the key to expire so Redis cleans up idle users
    -- This saves memory for users who don't return
    redis.call('EXPIRE', key, window)
    
    return 1 -- Allowed
else
    return 0 -- Blocked
end