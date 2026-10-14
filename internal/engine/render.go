package engine

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

var placeholder = regexp.MustCompile(`\{\{\s*(.+?)\s*\}\}`)

// placeholders lists the keys used in a template, e.g. {{ a }} {{b}} -> [a b].
func placeholders(tpl string) []string {
	var keys []string
	for _, m := range placeholder.FindAllStringSubmatch(tpl, -1) {
		keys = append(keys, m[1])
	}
	return keys
}

// render replaces every {{ key }} in tpl with lookup(key).
func render(tpl string, lookup func(key string) (string, bool)) (string, error) {
	var missing string
	out := placeholder.ReplaceAllStringFunc(tpl, func(m string) string {
		key := placeholder.FindStringSubmatch(m)[1]
		v, ok := lookup(key)
		if !ok && missing == "" {
			missing = key
		}
		return v
	})
	if missing != "" {
		return "", fmt.Errorf("unresolved placeholder %q", missing)
	}
	return out, nil
}

// mapStrings returns a copy of v (scalar, list or object) with fn applied to every string in it.
func mapStrings(v any, fn func(string) (string, error)) (any, error) {
	return mapLeaves(v, func(s string) (any, error) { return fn(s) })
}

// mapLeaves is mapStrings where fn may turn a string into any JSON value.
func mapLeaves(v any, fn func(string) (any, error)) (any, error) {
	switch t := v.(type) {
	case string:
		return fn(t)
	case []any:
		out := make([]any, len(t))
		for i, e := range t {
			r, err := mapLeaves(e, fn)
			if err != nil {
				return nil, err
			}
			out[i] = r
		}
		return out, nil
	case map[string]any:
		out := make(map[string]any, len(t))
		for k, e := range t {
			r, err := mapLeaves(e, fn)
			if err != nil {
				return nil, err
			}
			out[k] = r
		}
		return out, nil
	default:
		return v, nil
	}
}

// lookupPath resolves a minimal JSONPath ($.a.b, $.items[0].id) against a decoded JSON document.
func lookupPath(doc any, path string) (any, bool) {
	rest, ok := strings.CutPrefix(path, "$.")
	if !ok {
		return nil, false
	}
	cur := doc
	for _, seg := range strings.Split(rest, ".") {
		name, idx, indexed := seg, 0, false
		if i := strings.IndexByte(seg, '['); i >= 0 && strings.HasSuffix(seg, "]") {
			n, err := strconv.Atoi(seg[i+1 : len(seg)-1])
			if err != nil {
				return nil, false
			}
			name, idx, indexed = seg[:i], n, true
		}

		obj, ok := cur.(map[string]any)
		if !ok {
			return nil, false
		}
		if cur, ok = obj[name]; !ok {
			return nil, false
		}
		if indexed {
			arr, ok := cur.([]any)
			if !ok || idx < 0 || idx >= len(arr) {
				return nil, false
			}
			cur = arr[idx]
		}
	}
	return cur, true
}

// stringify renders a decoded JSON scalar for comparisons and messages.
func stringify(v any) string {
	switch t := v.(type) {
	case nil:
		return ""
	case string:
		return t
	default:
		return fmt.Sprint(t)
	}
}
