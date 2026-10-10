package upstream

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"gw-base/internal/config"
)

func TestCallFormats(t *testing.T) {
	fields := map[string]any{"nin": "0012345678", "name": "a b&c"}

	for _, tc := range []struct {
		format, contentType string
		decode              func(t *testing.T, r *http.Request) map[string]string
	}{
		{"json", "application/json", func(t *testing.T, r *http.Request) map[string]string {
			got := map[string]string{}
			if err := json.NewDecoder(r.Body).Decode(&got); err != nil {
				t.Fatal(err)
			}
			return got
		}},
		{"form", "application/x-www-form-urlencoded", func(t *testing.T, r *http.Request) map[string]string {
			if err := r.ParseForm(); err != nil {
				t.Fatal(err)
			}
			return map[string]string{"nin": r.PostForm.Get("nin"), "name": r.PostForm.Get("name")}
		}},
	} {
		t.Run(tc.format, func(t *testing.T) {
			var gotType string
			var got map[string]string
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				gotType = r.Header.Get("Content-Type")
				got = tc.decode(t, r)
				_, _ = io.WriteString(w, `{}`)
			}))
			defer srv.Close()

			up, err := New(http.MethodPost, config.Upstream{
				BaseURL: srv.URL, Path: "/x", Format: tc.format, Timeout: time.Second,
			}, slog.New(slog.NewTextHandler(io.Discard, nil)))
			if err != nil {
				t.Fatal(err)
			}
			if _, _, err := up.Call(context.Background(), fields); err != nil {
				t.Fatal(err)
			}

			if gotType != tc.contentType {
				t.Errorf("content-type = %q, want %q", gotType, tc.contentType)
			}
			if got["nin"] != "0012345678" || got["name"] != "a b&c" {
				t.Errorf("body = %v, want %v", got, fields)
			}
		})
	}
}

func TestCallNestedJSON(t *testing.T) {
	var got map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewDecoder(r.Body).Decode(&got)
		_, _ = io.WriteString(w, `{}`)
	}))
	defer srv.Close()

	up, err := New(http.MethodPost, config.Upstream{BaseURL: srv.URL, Path: "/x", Format: "json", Timeout: time.Second},
		slog.New(slog.NewTextHandler(io.Discard, nil)))
	if err != nil {
		t.Fatal(err)
	}
	fields := map[string]any{
		"xx":    map[string]any{"x1": "x2"},
		"items": []any{map[string]any{"id": "1"}, map[string]any{"id": "2"}},
		"limit": 5,
	}
	if _, _, err := up.Call(context.Background(), fields); err != nil {
		t.Fatal(err)
	}
	want := `{"items":[{"id":"1"},{"id":"2"}],"limit":5,"xx":{"x1":"x2"}}`
	if b, _ := json.Marshal(got); string(b) != want {
		t.Errorf("body = %s, want %s", b, want)
	}

	// form bodies must stay flat
	form, _ := New(http.MethodPost, config.Upstream{BaseURL: srv.URL, Path: "/x", Format: "form", Timeout: time.Second},
		slog.New(slog.NewTextHandler(io.Discard, nil)))
	if _, _, err := form.Call(context.Background(), fields); err == nil {
		t.Error("expected an error for nested form body")
	}
}

func newLoggedUpstream(t *testing.T, level slog.Level) (*Upstream, *bytes.Buffer) {
	t.Helper()
	t.Setenv("GW_TEST_USER", "alice")
	t.Setenv("GW_TEST_PASS", "s3cret")

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("X-Reply", "yes")
		w.Header().Add("Set-Cookie", "sid=abc")
		_, _ = io.WriteString(w, `{"answer":42}`)
	}))
	t.Cleanup(srv.Close)

	var buf bytes.Buffer
	log := slog.New(slog.NewJSONHandler(&buf, &slog.HandlerOptions{Level: level}))
	up, err := New(http.MethodPost, config.Upstream{
		BaseURL: srv.URL, Path: "/x", Timeout: time.Second,
		Auth: &config.Auth{
			Type:     "basic",
			Username: config.Secret{Source: "env", Value: "GW_TEST_USER"},
			Password: config.Secret{Source: "env", Value: "GW_TEST_PASS"},
		},
	}, log)
	if err != nil {
		t.Fatal(err)
	}
	return up, &buf
}

func TestDebugLogsFullExchangeWithoutSecrets(t *testing.T) {
	up, logs := newLoggedUpstream(t, slog.LevelDebug)

	data, status, err := up.Call(context.Background(), map[string]any{"nin": "123"})
	if err != nil || status != 200 || string(data) != `{"answer":42}` {
		t.Fatalf("caller must still get the full answer: %q %d %v", data, status, err)
	}

	out := logs.String()
	for _, want := range []string{
		`"msg":"upstream request"`, `{\"nin\":\"123\"}`, `"msg":"upstream response"`,
		`{\"answer\":42}`, `"status":200`, `"X-Reply":"yes"`, `[redacted]`,
	} {
		if !strings.Contains(out, want) {
			t.Errorf("debug log is missing %s\n%s", want, out)
		}
	}
	for _, secret := range []string{"s3cret", base64.StdEncoding.EncodeToString([]byte("alice:s3cret")), "sid=abc"} {
		if strings.Contains(out, secret) {
			t.Errorf("debug log leaks %q\n%s", secret, out)
		}
	}
}

func TestInfoLevelLogsNoPayloads(t *testing.T) {
	up, logs := newLoggedUpstream(t, slog.LevelInfo)

	if _, _, err := up.Call(context.Background(), map[string]any{"nin": "123"}); err != nil {
		t.Fatal(err)
	}
	if out := logs.String(); strings.Contains(out, "upstream request") || strings.Contains(out, "nin") || strings.Contains(out, "answer") {
		t.Errorf("payloads must not be logged above debug level:\n%s", out)
	}
}
