package engine

import (
	"fmt"
	"net/url"
	"os"
	"strings"

	"gw-base/internal/config"
)

func declaredParams(ep config.Endpoint) map[string]bool {
	declared := make(map[string]bool, len(ep.Params))
	for _, p := range ep.Params {
		declared[p.Name] = true
	}
	return declared
}

func isSet(env string) bool {
	_, set := os.LookupEnv(env)
	return set
}

// valueError is a request value that cannot be used in the upstream path.
// It is the client's fault, so it is answered with 400.
type valueError struct{ msg string }

func (e *valueError) Error() string { return e.msg }

// checkPath fails at startup on a bad upstream.path template.
// Allowed: declared params, {{env.NAME}} and the time built-ins.
func checkPath(ep config.Endpoint) error {
	declared := declaredParams(ep)
	for _, key := range placeholders(ep.Upstream.Path) {
		switch {
		case strings.HasPrefix(key, envPrefix):
			if name := strings.TrimPrefix(key, envPrefix); !isSet(name) {
				return fmt.Errorf("upstream.path: env var %q is not set", name)
			}
		case isClockBuiltin(key):
			if err := validateBuiltin(key); err != nil {
				return fmt.Errorf("upstream.path: %w", err)
			}
		case !declared[key]:
			return fmt.Errorf("upstream.path: unknown param %q", key)
		}
	}
	return nil
}

// buildPath renders upstream.path for one request. Request values are escaped
// as a single path segment, so a value can never add segments, a query string
// or "..". A param used in the path must have a value.
func (h *handler) buildPath(params map[string]string) (string, error) {
	at := h.clock.now()
	var bad error
	lookup := func(key string) (string, bool) {
		if name, ok := strings.CutPrefix(key, envPrefix); ok {
			return os.LookupEnv(name) // operator-controlled, inserted as written
		}
		if isClockBuiltin(key) {
			return h.clock.resolve(key, at)
		}
		v := params[key]
		switch {
		case v == "":
			bad = &valueError{msg: key + " is required"}
		case v == "." || v == "..":
			bad = &valueError{msg: key + " has an invalid value"}
		default:
			return url.PathEscape(v), true
		}
		return "", false
	}

	out, err := render(h.ep.Upstream.Path, lookup)
	if bad != nil {
		return "", bad
	}
	return out, err
}
