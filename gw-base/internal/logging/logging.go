package logging

import (
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"

	"gopkg.in/natefinch/lumberjack.v2"

	"gw-base/internal/config"
)

// New builds the service logger from the logging section of the YAML.
func New(cfg config.Logging, svc config.Service) (*slog.Logger, error) {
	var writers []io.Writer
	if cfg.Console {
		writers = append(writers, os.Stdout)
	}
	if cfg.FileName != "" {
		dir := cfg.FilePath
		if dir == "" {
			dir = "."
		}
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return nil, fmt.Errorf("creating log dir: %w", err)
		}
		writers = append(writers, &lumberjack.Logger{
			Filename:   filepath.Join(dir, cfg.FileName),
			MaxSize:    cfg.Rotating.MaxSize,
			MaxBackups: cfg.Rotating.MaxBackups,
			MaxAge:     cfg.Rotating.MaxAge,
			Compress:   cfg.Rotating.Compress,
		})
	}
	if len(writers) == 0 {
		writers = append(writers, os.Stdout) // never log into the void
	}

	level := slog.LevelInfo
	if cfg.Level != "" {
		if err := level.UnmarshalText([]byte(cfg.Level)); err != nil {
			return nil, fmt.Errorf("logging.level: %w", err)
		}
	}
	opts := &slog.HandlerOptions{Level: level}
	out := io.MultiWriter(writers...)

	var h slog.Handler
	switch cfg.Encoding {
	case "", "json":
		h = slog.NewJSONHandler(out, opts)
	case "text":
		h = slog.NewTextHandler(out, opts)
	default:
		return nil, fmt.Errorf("logging.encoding must be json or text, got %q", cfg.Encoding)
	}

	return slog.New(h).With("service", svc.Name, "version", svc.Version), nil
}
