package upstream

import (
	"bytes"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"time"
)

// Headers that carry credentials never reach the logs.
var sensitiveHeaders = map[string]bool{
	"Authorization":       true,
	"Proxy-Authorization": true,
	"Cookie":              true,
	"Set-Cookie":          true,
}

// debugTransport logs the full upstream exchange when the logger is at debug
// level. At any other level it is a plain pass-through.
type debugTransport struct {
	next   http.RoundTripper
	logger *slog.Logger
}

func (t debugTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	ctx := req.Context()
	if !t.logger.Enabled(ctx, slog.LevelDebug) {
		return t.next.RoundTrip(req)
	}

	t.logger.DebugContext(ctx, "upstream request",
		"method", req.Method, "url", req.URL.String(),
		"headers", redact(req.Header), "body", requestBody(req))

	start := time.Now()
	resp, err := t.next.RoundTrip(req)
	if err != nil {
		t.logger.DebugContext(ctx, "upstream request failed",
			"method", req.Method, "url", req.URL.String(),
			"duration_ms", time.Since(start).Milliseconds(), "error", err)
		return nil, err
	}

	body, readErr := peekBody(resp)
	attrs := []any{
		"method", req.Method, "url", req.URL.String(), "status", resp.StatusCode,
		"headers", redact(resp.Header), "body", string(body),
		"duration_ms", time.Since(start).Milliseconds(),
	}
	if readErr != nil {
		attrs = append(attrs, "body_error", readErr)
	}
	t.logger.DebugContext(ctx, "upstream response", attrs...)
	return resp, nil
}

// requestBody reads the outgoing body without consuming it.
func requestBody(req *http.Request) string {
	if req.GetBody == nil {
		return ""
	}
	rc, err := req.GetBody()
	if err != nil {
		return ""
	}
	defer rc.Close()
	data, _ := io.ReadAll(rc)
	return string(data)
}

// peekBody reads the response body (up to maxResponseBytes) and puts it back
// so the caller can still read it.
func peekBody(resp *http.Response) ([]byte, error) {
	data, err := io.ReadAll(io.LimitReader(resp.Body, maxResponseBytes))
	resp.Body = readCloser{Reader: io.MultiReader(bytes.NewReader(data), resp.Body), Closer: resp.Body}
	return data, err
}

type readCloser struct {
	io.Reader
	io.Closer
}

func redact(h http.Header) map[string]string {
	out := make(map[string]string, len(h))
	for name, values := range h {
		if sensitiveHeaders[http.CanonicalHeaderKey(name)] {
			out[name] = "[redacted]"
			continue
		}
		out[name] = strings.Join(values, ", ")
	}
	return out
}
