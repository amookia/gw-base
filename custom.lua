local b64 = 'ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789+/'

function base64_encode(data)
  if not data then return nil end
  data = tostring(data)

  local result = {}
  local len = #data
  local i = 1

  while i <= len do
    local a = string.byte(data, i) or 0
    local b = string.byte(data, i + 1) or 0
    local c = string.byte(data, i + 2) or 0

    local n = a * 65536 + b * 256 + c

    local c1 = math.floor(n / 262144) % 64 + 1
    local c2 = math.floor(n / 4096)  % 64 + 1
    local c3 = math.floor(n / 64)    % 64 + 1
    local c4 = n % 64 + 1

    result[#result + 1] = string.sub(b64, c1, c1)
    result[#result + 1] = string.sub(b64, c2, c2)
    result[#result + 1] = (i + 1 <= len) and string.sub(b64, c3, c3) or '='
    result[#result + 1] = (i + 2 <= len) and string.sub(b64, c4, c4) or '='

    i = i + 3
  end

  return table.concat(result)
end

-- authenticate is called for every outgoing request
function authenticate(req)
    local token = cache_get("access_token")   
    local expires = tonumber(cache_get("expires_at") or "0")

    print(getenv("PGSB_USERNAME"))
    if not token or os.time() > expires - 30 then
        headers = {
    		["Authorization"] = "Basic " .. base64_encode(getenv("PGSB_USERNAME") .. ":" .. getenv("PGSB_PASSWORD"))
	}
	
        local body = "username=Melal&password=8uLdgENj-MLsVU6&grant_type=password"
        --local resp, status = http_post("http://127.0.0.1:8787/astangsb/post/PostToken", body, headers)
    	local resp, status = http_post("http://" .. getenv("INQUIRY_URL") .. "/astangsb/post/PostToken", body, headers)
        print(resp)

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

    set_header(req, "Token", token)
    set_header(req, "Authorization", "Basic bWVsYWxfcGdzYjpNZUxMQEw4N0A5")
    return true
end
