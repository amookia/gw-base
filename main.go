package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"gw-base/internal/config"
	"gw-base/internal/engine"
	"gw-base/pkg/logging"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}

func run() error {
	path := flag.String("config", "test.yaml", "path to the service YAML")
	flag.Parse()

	cfg, err := config.Load(*path)
	if err != nil {
		return fmt.Errorf("loading config: %w", err)
	}

	logger, err := logging.New(loggerOptions(cfg.Logging))
	if err != nil {
		return fmt.Errorf("creating logger: %w", err)
	}
	logger = logger.With("service", cfg.Service.Name, "version", cfg.Service.Version)

	eng, err := engine.New(cfg, logger)
	if err != nil {
		return err
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	return eng.Run(ctx)
}

// loggerOptions maps the YAML logging section onto pkg/logging, which stays
// independent of the application's config.
func loggerOptions(l config.Logging) logging.Options {
	return logging.Options{
		Backend:  l.Logger,
		Level:    l.Level,
		Encoding: l.Encoding,
		Console:  l.Console,
		File: logging.File{
			Dir:        l.FilePath,
			Name:       l.FileName,
			MaxSizeMB:  l.Rotating.MaxSize,
			MaxBackups: l.Rotating.MaxBackups,
			MaxAgeDays: l.Rotating.MaxAge,
			Compress:   l.Rotating.Compress,
		},
	}
}
