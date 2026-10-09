-- authenticate is called for every outgoing request
function authenticate(req)
    local token = cache_get("access_token")   
    local expires = tonumber(cache_get("expires_at") or "0")

    if not token or os.time() > expires - 30 then
        -- refresh
        local body = "grant_type=refresh_token&refresh_token=" .. (cache_get("refresh_token") or "")
        local resp, status = http_post("http://127.0.0.1:8888/oauth/token", body)

        if status ~= 200 then
            return "token refresh failed: " .. tostring(status)
        end

        local access = string.match(resp, '"access_token"%s*:%s*"([^"]+)"')
        local refresh = string.match(resp, '"refresh_token"%s*:%s*"([^"]+)"')
        local expires_in = tonumber(string.match(resp, '"expires_in"%s*:%s*(%d+)') or "3600")

        if not access then
            return "could not parse access_token"
        end

        cache_set("access_token", access)
        if refresh then cache_set("refresh_token", refresh) end
        cache_set("expires_at", tostring(os.time() + expires_in))
        token = access
    end

    set_header(req, "Authorization", "Bearer " .. token)
    return true
end
