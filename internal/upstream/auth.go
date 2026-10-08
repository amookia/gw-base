package upstream

import (
	"fmt"
	"net/http"

	"gw-base/internal/config"
)

// Authenticator decorates an outgoing upstream request with credentials.
type Authenticator func(r *http.Request) error

// NewAuthenticator resolves secrets once at startup, so a missing env var
// fails fast instead of on the first request. A nil config means no auth.
func NewAuthenticator(cfg *config.Auth) (Authenticator, error) {
	if cfg == nil {
		return nil, nil
	}
	switch cfg.Type {
	case "basic":
		user, err := cfg.Username.Resolve()
		if err != nil {
			return nil, fmt.Errorf("auth.username: %w", err)
		}
		pass, err := cfg.Password.Resolve()
		if err != nil {
			return nil, fmt.Errorf("auth.password: %w", err)
		}
		return func(r *http.Request) error {
			r.SetBasicAuth(user, pass)
			return nil
		}, nil
	default:
		return nil, fmt.Errorf("unsupported auth type %q", cfg.Type)
	}
}
