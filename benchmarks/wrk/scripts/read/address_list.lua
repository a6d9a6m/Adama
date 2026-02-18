local target = os.getenv("TARGET") or "gateway"
local user_id = os.getenv("USER_ID") or "88"
local non2xx = 0

local function path_prefix()
    if target == "service" then
        return ""
    end
    return "/api/v1"
end

request = function()
    local headers = {
        ["X-User-Id"] = user_id
    }
    local path = path_prefix() .. "/user/address/list"
    return wrk.format("GET", path, headers)
end

response = function(status, headers, body)
    if status >= 300 then
        non2xx = non2xx + 1
    end
end

done = function(summary, latency, requests)
    io.write(string.format("\nNon-2xx responses: %d\n", non2xx))
end
