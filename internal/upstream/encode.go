package upstream

import (
	"bytes"
	"encoding/json"
	"fmt"
	"maps"
	"mime/multipart"
	"net/url"
	"slices"
)

// encode serializes fields per upstream.format and returns the body with its
// Content-Type. No fields means no body.
func encode(format string, fields map[string]any) (body []byte, contentType string, err error) {
	if len(fields) == 0 {
		return nil, "", nil
	}
	switch format {
	case "form":
		return encodeForm(fields)
	case "multipart":
		return encodeMultipart(fields)
	default:
		body, err = json.Marshal(fields)
		return body, "application/json", err
	}
}

// encodeForm sends application/x-www-form-urlencoded.
func encodeForm(fields map[string]any) ([]byte, string, error) {
	text, err := flatten(fields)
	if err != nil {
		return nil, "", err
	}
	form := make(url.Values, len(text))
	for k, v := range text {
		form.Set(k, v)
	}
	return []byte(form.Encode()), "application/x-www-form-urlencoded", nil
}

// encodeMultipart sends multipart/form-data, like curl --form. Fields go out
// in alphabetical order, since YAML maps carry no order.
func encodeMultipart(fields map[string]any) ([]byte, string, error) {
	text, err := flatten(fields)
	if err != nil {
		return nil, "", err
	}
	var buf bytes.Buffer
	w := multipart.NewWriter(&buf)
	for _, k := range slices.Sorted(maps.Keys(text)) {
		if err := w.WriteField(k, text[k]); err != nil {
			return nil, "", fmt.Errorf("field %q: %w", k, err)
		}
	}
	if err := w.Close(); err != nil {
		return nil, "", err
	}
	return buf.Bytes(), w.FormDataContentType(), nil
}

// flatten turns fields into text; form-style bodies cannot carry nested values.
func flatten(fields map[string]any) (map[string]string, error) {
	out := make(map[string]string, len(fields))
	for k, v := range fields {
		switch t := v.(type) {
		case nil:
			out[k] = ""
		case map[string]any, []any:
			return nil, fmt.Errorf("field %q: form bodies cannot be nested", k)
		default:
			out[k] = fmt.Sprint(t)
		}
	}
	return out, nil
}
