package engine

import (
	"encoding/json"
	"strconv"

	"gw-base/internal/config"
)

// wholePlaceholder returns the key when s is exactly one {{ key }} and nothing else.
func wholePlaceholder(s string) (string, bool) {
	loc := placeholder.FindStringSubmatchIndex(s)
	if loc == nil || loc[0] != 0 || loc[1] != len(s) {
		return "", false
	}
	return s[loc[2]:loc[3]], true
}

// typedValue turns a param's text (already validated when the request was
// read) into its declared JSON type. A param that was not sent becomes null.
func typedValue(typ, text string) (any, error) {
	if text == "" {
		return nil, nil
	}
	switch typ {
	case config.TypeBoolean:
		return strconv.ParseBool(text)
	case config.TypeInteger:
		return strconv.ParseInt(text, 10, 64)
	case config.TypeNumber:
		return json.Number(text), nil // keeps the exact digits the client sent
	}
	return text, nil
}
