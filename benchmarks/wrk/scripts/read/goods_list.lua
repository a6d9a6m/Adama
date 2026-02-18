local target = os.getenv("TARGET") or "gateway"
local page = os.getenv("PAGE") or "1"
local page_size = os.getenv("PAGE_SIZE") or "10"
local keyword = os.getenv("KEYWORD") or ""
local non2xx = 0

local function path_prefix()
    if target == "service" then
        return ""
    end
    return "/api/v1"
end

request = function()
    local path = string.format("%s/goods/list?page=%s&page_size=%s", path_prefix(), page, page_size)
    if keyword ~= "" then
        path = path .. "&keyword=" .. keyword
    end
    return wrk.format("GET", path)
end

response = function(status, headers, body)
    if status >= 300 then
        non2xx = non2xx + 1
    end
end

done = function(summary, latency, requests)
    io.write(string.format("\nNon-2xx responses: %d\n", non2xx))
end
