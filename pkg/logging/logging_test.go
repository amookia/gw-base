package logging

import (
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestDefaultBackendWritesJSONToFile(t *testing.T) {
	dir := t.TempDir()
	log, err := New(Options{Level: "info", File: File{Dir: dir, Name: "app.log"}})
	if err != nil {
		t.Fatal(err)
	}

	log.Debug("hidden")
	log.Info("hello", "k", "v")

	data, err := os.ReadFile(filepath.Join(dir, "app.log"))
	if err != nil {
		t.Fatal(err)
	}
	out := string(data)
	if !strings.Contains(out, `"msg":"hello"`) || !strings.Contains(out, `"k":"v"`) {
		t.Errorf("log file = %q", out)
	}
	if strings.Contains(out, "hidden") {
		t.Errorf("debug record leaked past level info: %q", out)
	}
}

func TestInvalidOptions(t *testing.T) {
	for name, tc := range map[string]struct {
		opts Options
		want string
	}{
		"backend":  {Options{Backend: "nope"}, "unknown logger backend"},
		"level":    {Options{Level: "loud"}, "level"},
		"encoding": {Options{Encoding: "xml"}, "encoding must be json or text"},
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := New(tc.opts); err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Errorf("err = %v, want it to contain %q", err, tc.want)
			}
		})
	}
}

func TestRegisterCustomBackend(t *testing.T) {
	Register("test-backend", func(Options) (*slog.Logger, error) {
		return slog.New(slog.NewTextHandler(io.Discard, nil)), nil
	})
	if _, err := New(Options{Backend: "test-backend"}); err != nil {
		t.Fatal(err)
	}

	defer func() {
		if recover() == nil {
			t.Error("registering the same backend twice should panic")
		}
	}()
	Register("test-backend", newSlog)
}
