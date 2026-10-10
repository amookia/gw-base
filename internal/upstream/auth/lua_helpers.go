package auth

import (
	"io"
	"net/http"
	"strings"

	lua "github.com/yuin/gopher-lua"
)

func registerLuaHelpers(L *lua.LState, cache AuthCache) {
	L.OpenLibs()

	L.SetGlobal("cache_get", L.NewFunction(luaCacheGet(cache)))
	L.SetGlobal("cache_set", L.NewFunction(luaCacheSet(cache)))
	L.SetGlobal("set_header", L.NewFunction(luaSetHeader))
	L.SetGlobal("http_post", L.NewFunction(luaHTTPPost))
	L.SetGlobal("http_post_json", L.NewFunction(luaHTTPPostJSON))
}

func luaCacheGet(cache AuthCache) lua.LGFunction {
	return func(L *lua.LState) int {
		key := L.CheckString(1)
		if v, ok := cache.Get(key); ok {
			L.Push(lua.LString(v))
		} else {
			L.Push(lua.LNil)
		}
		return 1
	}
}

func luaCacheSet(cache AuthCache) lua.LGFunction {
	return func(L *lua.LState) int {
		cache.Set(L.CheckString(1), L.CheckString(2))
		return 0
	}
}

func luaSetHeader(L *lua.LState) int {
	ud := L.CheckUserData(1)
	req, ok := ud.Value.(*http.Request)
	if !ok {
		L.ArgError(1, "http.Request expected")
	}
	req.Header.Set(L.CheckString(2), L.CheckString(3))
	return 0
}

func luaHTTPPost(L *lua.LState) int {
	urlStr := L.CheckString(1)
	body := L.OptString(2, "")

	resp, err := http.Post(urlStr, "application/x-www-form-urlencoded", strings.NewReader(body))
	return pushHTTPResponse(L, resp, err)
}

func luaHTTPPostJSON(L *lua.LState) int {
	urlStr := L.CheckString(1)
	body := L.OptString(2, "{}")

	req, err := http.NewRequest(http.MethodPost, urlStr, strings.NewReader(body))
	if err != nil {
		return pushError(L, err)
	}
	req.Header.Set("Content-Type", "application/json")

	if L.GetTop() >= 3 && L.Get(3).Type() == lua.LTTable {
		L.Get(3).(*lua.LTable).ForEach(func(k, v lua.LValue) {
			if ks, ok := k.(lua.LString); ok {
				if vs, ok := v.(lua.LString); ok {
					req.Header.Set(string(ks), string(vs))
				}
			}
		})
	}

	resp, err := http.DefaultClient.Do(req)
	return pushHTTPResponse(L, resp, err)
}

func pushHTTPResponse(L *lua.LState, resp *http.Response, err error) int {
	if err != nil {
		return pushError(L, err)
	}
	defer resp.Body.Close()

	b, err := io.ReadAll(resp.Body)
	if err != nil {
		return pushError(L, err)
	}

	L.Push(lua.LString(string(b)))
	L.Push(lua.LNumber(resp.StatusCode))
	return 2
}

func pushError(L *lua.LState, err error) int {
	L.Push(lua.LNil)
	L.Push(lua.LString(err.Error()))
	return 2
}
