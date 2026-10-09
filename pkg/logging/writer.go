package logging

import (
	"fmt"
	"io"
	"os"
	"path/filepath"

	"gopkg.in/natefinch/lumberjack.v2"
)

// OpenWriter returns where log records go: stdout and/or a rotating file.
// With neither configured it falls back to stdout, so logs are never lost silently.
func OpenWriter(opts Options) (io.Writer, error) {
	var writers []io.Writer
	if opts.Console {
		writers = append(writers, os.Stdout)
	}
	if f := opts.File; f.Name != "" {
		dir := f.Dir
		if dir == "" {
			dir = "."
		}
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return nil, fmt.Errorf("creating log dir: %w", err)
		}
		writers = append(writers, &lumberjack.Logger{
			Filename:   filepath.Join(dir, f.Name),
			MaxSize:    f.MaxSizeMB,
			MaxBackups: f.MaxBackups,
			MaxAge:     f.MaxAgeDays,
			Compress:   f.Compress,
		})
	}

	switch len(writers) {
	case 0:
		return os.Stdout, nil
	case 1:
		return writers[0], nil
	default:
		return io.MultiWriter(writers...), nil
	}
}
