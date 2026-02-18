local target = os.getenv("TARGET") or "gateway"
local user_id = os.getenv("USER_ID") or "88"
local goods_id = os.getenv("GOODS_ID") or "1"
local amount = os.getenv("AMOUNT") or "1"
local non2xx = 0
local token_parse_failures = 0
local current_token = nil
local phase = "token"

local function path_prefix()
    if target == "service" then
        return ""
    end
    return "/api/v1"
end

request = function()
    if phase == "token" or current_token == nil then
        local headers = {
            ["X-User-Id"] = user_id
        }
        local path = string.format("%s/adama/goods/%s", path_prefix(), goods_id)
        return wrk.format("GET", path, headers)
    end

    local headers = {
        ["Content-Type"] = "application/json",
        ["X-User-Id"] = user_id,
        ["X-Seckill-Token"] = current_token
    }
    local body = string.format('{"gid":%s,"amount":%s}', goods_id, amount)
    local path = path_prefix() .. "/adama/order"
    return wrk.format("POST", path, headers, body)
end

response = function(status, headers, body)
    if status >= 300 then
        non2xx = non2xx + 1
    end

    if phase == "token" then
        local token = string.match(body, '"seckill_token"%s*:%s*"([^"]+)"')
        if token ~= nil and token ~= "" then
            current_token = token
            phase = "order"
        else
            token_parse_failures = token_parse_failures + 1
            current_token = nil
            phase = "token"
        end
        return
    end

    current_token = nil
    phase = "token"
end

done = function(summary, latency, requests)
    io.write(string.format("\nNon-2xx responses: %d\n", non2xx))
    io.write(string.format("Token parse failures: %d\n", token_parse_failures))
end
