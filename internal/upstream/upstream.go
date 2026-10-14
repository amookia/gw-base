package upstream

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"gw-base/internal/config"
	auth2 "gw-base/internal/upstream/auth"
)

const maxResponseBytes = 10 << 20

type Upstream struct {
	method string
	base   string // base URL without a trailing slash
	logURL string // base + the configured path template: safe to log, holds no request values
	format string // json | form | multipart
	header map[string]string
	auth   auth2.Authenticator
	logger *slog.Logger
	client *http.Client
}

func New(cfg config.Upstream, logger *slog.Logger) (*Upstream, error) {
	base, err := cfg.Base()
	if err != nil {
		return nil, err
	}
	method := cfg.Method
	if method == "" {
		method = http.MethodPost
	}
	auth, err := auth2.NewAuthenticator(cfg.Auth)
	if err != nil {
		return nil, err
	}
	return &Upstream{
		method: method,
		base:   base,
		logURL: joinURL(base, cfg.Path),
		format: cfg.Format,
		header: cfg.Headers,
		auth:   auth,
		logger: logger,
		// Own transport per upstream: the connection pool is reused across calls.
		client: &http.Client{
			Timeout:   cfg.Timeout,
			Transport: debugTransport{next: http.DefaultTransport.(*http.Transport).Clone(), logger: logger},
		},
	}, nil
}

func joinURL(base, path string) string {
	return base + "/" + strings.TrimLeft(path, "/")
}

// Call sends fields (may be empty) to base + path and returns the response body
// and status code. path is already rendered and escaped by the caller.
func (u *Upstream) Call(ctx context.Context, path string, fields map[string]any) ([]byte, int, error) {
	body, contentType, err := encode(u.format, fields)
	if err != nil {
		return nil, 0, fmt.Errorf("encoding body: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, u.method, joinURL(u.base, path), bytes.NewReader(body))
	if err != nil {
		return nil, 0, fmt.Errorf("building request: %w", err)
	}
	//req.Header.Set("Accept", "application/json") // default; upstream.headers may override it
	req.Header.Set("Accept", "*/*")
	for name, value := range u.header {
		req.Header[name] = []string{value}
	}
	if contentType != "" {
		req.Header.Set("Content-Type", contentType)
	}
	if u.auth != nil {
		if err := u.auth(req); err != nil {
			return nil, 0, fmt.Errorf("authenticating request: %w", err)
		}
	}

	start := time.Now()
	resp, err := u.client.Do(req)
	if err != nil {
		return nil, 0, fmt.Errorf("calling upstream: %w", err)
	}
	defer resp.Body.Close()

	data, err := io.ReadAll(io.LimitReader(resp.Body, maxResponseBytes))
	if err != nil {
		return nil, resp.StatusCode, fmt.Errorf("reading response: %w", err)
	}

	u.logger.Info("upstream call complete",
		"method", u.method, "url", u.logURL, "status", resp.StatusCode, "duration_ms", time.Since(start).Milliseconds())
	return data, resp.StatusCode, nil
}
