package auth

import (
	"fmt"
	"gw-base/internal/config"
	"net/http"
	"os"

	lua "github.com/yuin/gopher-lua"
)

func newLuaAuthenticator(cfg *config.Auth) (Authenticator, error) {
	script, err := loadScript(cfg)
	if err != nil {
		return nil, err
	}

	cache := newMemoryCache(cfg.Cache)

	return func(r *http.Request) error {
		L := lua.NewState()
		defer L.Close()

		registerLuaHelpers(L, cache)

		if err := L.DoString(script); err != nil {
			return fmt.Errorf("lua script load: %w", err)
		}

		fn := L.GetGlobal("authenticate")
		if fn.Type() != lua.LTFunction {
			return fmt.Errorf("lua script must define function authenticate(req)")
		}

		ud := L.NewUserData()
		ud.Value = r

		if err := L.CallByParam(lua.P{
			Fn:      fn,
			NRet:    1,
			Protect: true,
		}, ud); err != nil {
			return fmt.Errorf("lua authenticate: %w", err)
		}

		return interpretLuaResult(L)
	}, nil
}

func loadScript(cfg *config.Auth) (string, error) {
	if cfg.ScriptIsFile {
		b, err := os.ReadFile(cfg.Script)
		if err != nil {
			return "", fmt.Errorf("read lua script: %w", err)
		}
		return string(b), nil
	}
	if cfg.Script == "" {
		return "", fmt.Errorf("lua script is empty")
	}
	return cfg.Script, nil
}

func interpretLuaResult(L *lua.LState) error {
	ret := L.Get(-1)
	L.Pop(1)

	switch ret.Type() {
	case lua.LTNil, lua.LTBool:
		if ret == lua.LFalse {
			return fmt.Errorf("lua authenticate returned false")
		}
		return nil
	case lua.LTString:
		return fmt.Errorf("lua authenticate: %s", ret.String())
	default:
		return fmt.Errorf("lua authenticate returned unexpected type %s", ret.Type())
	}
}
