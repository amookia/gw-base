package upstream

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
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
			if got["nin"] != fields["nin"] || got["name"] != fields["name"] {
				t.Errorf("body = %v, want %v", got, fields)
			}
		})
	}
}
