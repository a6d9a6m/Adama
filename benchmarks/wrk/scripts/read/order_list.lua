local target = os.getenv("TARGET") or "gateway"
local user_id = os.getenv("USER_ID") or "88"
local page = os.getenv("PAGE") or "1"
local page_size = os.getenv("PAGE_SIZE") or "10"
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
    local path = string.format("%s/order/list?page=%s&page_size=%s", path_prefix(), page, page_size)
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
