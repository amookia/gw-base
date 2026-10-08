package engine

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"

	"gw-base/internal/config"
)

type Engine struct {
	cfg    *config.Config
	log    *slog.Logger
	router *gin.Engine
}

// New wires one route per endpoint declared in the YAML.
func New(cfg *config.Config, logger *slog.Logger) (*Engine, error) {
	gin.SetMode(gin.ReleaseMode)
	r := gin.New()
	r.Use(gin.Recovery(), accessLog(logger))
	r.GET("/healthz", func(c *gin.Context) { c.String(http.StatusOK, "ok") })

	for _, ep := range cfg.Endpoints {
		h, err := newHandler(ep, cfg.Server.MaxBodyBytes, logger)
		if err != nil {
			return nil, fmt.Errorf("endpoint %q: %w", ep.Name, err)
		}
		r.Handle(ep.Method, ep.Path, h.serve)
		logger.Info("route registered", "endpoint", ep.Name, "method", ep.Method, "path", ep.Path)
	}
	return &Engine{cfg: cfg, log: logger, router: r}, nil
}

// Run serves until ctx is cancelled, then shuts down gracefully.
func (e *Engine) Run(ctx context.Context) error {
	srv := &http.Server{
		Addr:         e.cfg.Server.Addr(),
		Handler:      e.router,
		ReadTimeout:  e.cfg.Server.ReadTimeout,
		WriteTimeout: e.cfg.Server.WriteTimeout,
	}

	errc := make(chan error, 1)
	go func() { errc <- srv.ListenAndServe() }()
	e.log.Info("listening", "addr", srv.Addr, "mode", e.cfg.Mode)

	select {
	case err := <-errc:
		return err
	case <-ctx.Done():
	}

	e.log.Info("shutting down")
	shutdownCtx, cancel := context.WithTimeout(context.Background(), e.cfg.Server.ShutdownTimeout)
	defer cancel()
	if err := srv.Shutdown(shutdownCtx); err != nil && !errors.Is(err, http.ErrServerClosed) {
		return err
	}
	return nil
}

func accessLog(log *slog.Logger) gin.HandlerFunc {
	return func(c *gin.Context) {
		start := time.Now()
		c.Next()
		// FullPath is the route pattern, so path params (possibly PII) never reach the logs.
		log.Info("request",
			"method", c.Request.Method, "route", c.FullPath(),
			"status", c.Writer.Status(), "duration_ms", time.Since(start).Milliseconds())
	}
}
