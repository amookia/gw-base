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
	"gw-base/internal/logging"
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

	logger, err := logging.New(cfg.Logging, cfg.Service)
	if err != nil {
		return err
	}

	eng, err := engine.New(cfg, logger)
	if err != nil {
		return err
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	return eng.Run(ctx)
}
