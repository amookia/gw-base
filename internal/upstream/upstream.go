package upstream

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	auth2 "gw-base/internal/upstream/auth"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"time"

	"gw-base/internal/config"
)

const maxResponseBytes = 10 << 20

type Upstream struct {
	method string
	url    string
	format string // json | form
	auth   auth2.Authenticator
	logger *slog.Logger
	client *http.Client
}

func New(method string, cfg config.Upstream, logger *slog.Logger) (*Upstream, error) {
	target, err := cfg.URL()
	if err != nil {
		return nil, err
	}
	auth, err := auth2.NewAuthenticator(cfg.Auth)
	if err != nil {
		return nil, err
	}
	return &Upstream{
		method: method,
		url:    target,
		format: cfg.Format,
		auth:   auth,
		logger: logger,
		// Own transport per upstream: the connection pool is reused across calls.
		client: &http.Client{
			Timeout:   cfg.Timeout,
			Transport: http.DefaultTransport.(*http.Transport).Clone(),
		},
	}, nil
}

// encode serializes fields as a JSON object or a urlencoded form, per upstream.format.
func (u *Upstream) encode(fields map[string]any) (body []byte, contentType string, err error) {
	if len(fields) == 0 {
		return nil, "", nil
	}
	if u.format == "form" {
		form := make(url.Values, len(fields))
		for k, v := range fields {
			switch t := v.(type) {
			case nil:
				form.Set(k, "")
			case map[string]any, []any:
				return nil, "", fmt.Errorf("field %q: form bodies cannot be nested", k)
			default:
				form.Set(k, fmt.Sprint(t))
			}
		}
		return []byte(form.Encode()), "application/x-www-form-urlencoded", nil
	}
	body, err = json.Marshal(fields)
	return body, "application/json", err
}

// Call sends fields (may be empty) and returns the response body and status code.
func (u *Upstream) Call(ctx context.Context, fields map[string]any) ([]byte, int, error) {
	body, contentType, err := u.encode(fields)
	if err != nil {
		return nil, 0, fmt.Errorf("encoding body: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, u.method, u.url, bytes.NewReader(body))
	if err != nil {
		return nil, 0, fmt.Errorf("building request: %w", err)
	}
	req.Header.Set("Accept", "application/json")
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
		"method", u.method, "url", u.url, "status", resp.StatusCode, "duration_ms", time.Since(start).Milliseconds())
	return data, resp.StatusCode, nil
}
