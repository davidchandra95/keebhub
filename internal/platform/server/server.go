// Package server owns HTTP listener lifecycle and graceful shutdown.
package server

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"time"

	"go.uber.org/zap"
)

// Runner serves HTTP until the context ends or the listener fails.
type Runner struct {
	HTTPServer      *http.Server
	Logger          *zap.Logger
	ShutdownTimeout time.Duration
}

// Run blocks until shutdown completes or an unexpected serve error occurs.
func (r Runner) Run(ctx context.Context, listener net.Listener) error {
	logger := r.Logger
	if logger == nil {
		logger = zap.NewNop()
	}

	serveErrors := make(chan error, 1)
	go func() {
		serveErrors <- r.HTTPServer.Serve(listener)
	}()

	select {
	case err := <-serveErrors:
		if errors.Is(err, http.ErrServerClosed) {
			return nil
		}
		return fmt.Errorf("serve HTTP: %w", err)
	case <-ctx.Done():
		logger.Info("shutting down HTTP server")
	}

	shutdownCtx, cancel := context.WithTimeout(context.Background(), r.ShutdownTimeout)
	defer cancel()
	if err := r.HTTPServer.Shutdown(shutdownCtx); err != nil {
		return fmt.Errorf("shutdown HTTP server: %w", err)
	}

	if err := <-serveErrors; err != nil && !errors.Is(err, http.ErrServerClosed) {
		return fmt.Errorf("serve HTTP during shutdown: %w", err)
	}
	return nil
}
