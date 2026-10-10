package engine

import (
	"encoding/json"
	"fmt"
	"strconv"

	"gw-base/internal/config"
)

var typeNames = map[string]string{
	config.TypeString:  "a string",
	config.TypeBoolean: "a boolean",
	config.TypeInteger: "an integer",
	config.TypeNumber:  "a number",
}

func typeError(p config.Param) error {
	return fmt.Errorf("%s must be %s", p.Name, typeNames[p.Type])
}

// fromJSON checks a JSON body value against the param's type and returns its
// canonical text, which is what templates and patterns see.
func fromJSON(p config.Param, v any) (string, error) {
	switch p.Type {
	case config.TypeString:
		if s, ok := v.(string); ok {
			return s, nil
		}
	case config.TypeBoolean:
		if b, ok := v.(bool); ok {
			return strconv.FormatBool(b), nil
		}
	case config.TypeInteger:
		if n, ok := v.(json.Number); ok {
			if _, err := n.Int64(); err == nil {
				return n.String(), nil
			}
		}
	case config.TypeNumber:
		if n, ok := v.(json.Number); ok {
			if _, err := n.Float64(); err == nil {
				return n.String(), nil
			}
		}
	}
	return "", typeError(p)
}

// fromText does the same for query, path and header values, which always
// arrive as text. An empty value means "not sent".
func fromText(p config.Param, s string) (string, error) {
	if s == "" {
		return "", nil
	}
	switch p.Type {
	case config.TypeBoolean:
		if s != "true" && s != "false" {
			return "", typeError(p)
		}
	case config.TypeInteger:
		if _, err := strconv.ParseInt(s, 10, 64); err != nil {
			return "", typeError(p)
		}
	case config.TypeNumber:
		if _, err := strconv.ParseFloat(s, 64); err != nil {
			return "", typeError(p)
		}
	}
	return s, nil
}
