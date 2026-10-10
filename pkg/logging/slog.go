package logging

import (
	"fmt"
	"log/slog"
)

// ParseLevel maps a level name to a slog.Level; "" means info.
func ParseLevel(name string) (slog.Level, error) {
	level := slog.LevelInfo
	if name == "" {
		return level, nil
	}
	if err := level.UnmarshalText([]byte(name)); err != nil {
		return 0, fmt.Errorf("level: %w", err)
	}
	return level, nil
}

// newSlog is the default backend: the standard library's slog.
func newSlog(opts Options) (*slog.Logger, error) {
	level, err := ParseLevel(opts.Level)
	if err != nil {
		return nil, err
	}
	out, err := OpenWriter(opts)
	if err != nil {
		return nil, err
	}

	handlerOpts := &slog.HandlerOptions{Level: level}
	switch opts.Encoding {
	case "", "json":
		return slog.New(slog.NewJSONHandler(out, handlerOpts)), nil
	case "text":
		return slog.New(slog.NewTextHandler(out, handlerOpts)), nil
	default:
		return nil, fmt.Errorf("encoding must be json or text, got %q", opts.Encoding)
	}
}
