package engine

import (
	"errors"
	"fmt"
	"math"
	"strconv"
	"strings"
	"time"
	"unicode"
)

const (
	nowPrefix       = "now:"
	millisPrefix    = "millis:"
	maxMillisDigits = 13 // a Unix time in milliseconds has 13 digits
)

// builtins are template values computed per request instead of read from params:
//
//	{{ now:yyyyMMddHHmmss }}   the current time, in the server's local timezone
//	{{ millis:6 }}             the last 6 digits of the Unix time in milliseconds
//
// Layout tokens: yyyy MM dd HH mm ss.
type builtins struct {
	now func() time.Time // injectable for tests
}

func newBuiltins() builtins {
	return builtins{now: time.Now}
}

func isBuiltin(key string) bool {
	return strings.HasPrefix(key, nowPrefix) || strings.HasPrefix(key, millisPrefix)
}

// validateBuiltin checks a built-in key at startup, so typos never reach a request.
func validateBuiltin(key string) error {
	if layout, ok := strings.CutPrefix(key, nowPrefix); ok {
		_, err := goLayout(layout)
		return err
	}
	if digits, ok := strings.CutPrefix(key, millisPrefix); ok {
		_, err := millisDigits(digits)
		return err
	}
	return fmt.Errorf("unknown built-in %q", key)
}

// resolve renders a built-in for the instant at. One instant per request keeps
// every built-in in a single template consistent with the others.
func (b builtins) resolve(key string, at time.Time) (string, bool) {
	if layout, ok := strings.CutPrefix(key, nowPrefix); ok {
		goFmt, err := goLayout(layout)
		if err != nil {
			return "", false
		}
		return at.Format(goFmt), true
	}
	if digits, ok := strings.CutPrefix(key, millisPrefix); ok {
		n, err := millisDigits(digits)
		if err != nil {
			return "", false
		}
		return fmt.Sprintf("%0*d", n, at.UnixMilli()%int64(math.Pow10(n))), true
	}
	return "", false
}

var layoutTokens = strings.NewReplacer("yyyy", "2006", "MM", "01", "dd", "02", "HH", "15", "mm", "04", "ss", "05")

func goLayout(layout string) (string, error) {
	if layout == "" {
		return "", errors.New("now: needs a layout, e.g. now:yyyyMMddHHmmss")
	}
	out := layoutTokens.Replace(layout)
	if strings.ContainsFunc(out, unicode.IsLetter) {
		return "", fmt.Errorf("layout %q: only yyyy MM dd HH mm ss are supported", layout)
	}
	return out, nil
}

func millisDigits(s string) (int, error) {
	n, err := strconv.Atoi(s)
	if err != nil || n < 1 || n > maxMillisDigits {
		return 0, fmt.Errorf("millis: needs a digit count from 1 to %d, got %q", maxMillisDigits, s)
	}
	return n, nil
}
