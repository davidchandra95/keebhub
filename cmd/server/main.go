package main

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/davidchandra95/keebhub/internal/adapter/discord"
	"github.com/davidchandra95/keebhub/internal/adapter/httpapi"
	postgresadapter "github.com/davidchandra95/keebhub/internal/adapter/postgres"
	"github.com/davidchandra95/keebhub/internal/app"
	"github.com/davidchandra95/keebhub/internal/platform/config"
	"github.com/davidchandra95/keebhub/internal/platform/database"
	"github.com/davidchandra95/keebhub/internal/platform/logging"
	platformserver "github.com/davidchandra95/keebhub/internal/platform/server"
	"go.uber.org/zap"
)

func main() {
	if err := run(); err != nil {
		log.Printf("KeebHub server failed: %v", err)
		os.Exit(1)
	}
}

func run() (returnErr error) {
	if err := loadDevelopmentEnv(); err != nil {
		return err
	}

	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("load configuration: %w", err)
	}

	logger, err := logging.New(cfg.Environment, cfg.LogLevel)
	if err != nil {
		return err
	}
	defer func() {
		returnErr = errors.Join(returnErr, syncLogger(logger))
	}()

	pool, err := database.NewPool(context.Background(), cfg)
	if err != nil {
		return err
	}
	defer pool.Close()

	var discordOAuth app.DiscordOAuth
	if cfg.DiscordConfigured() {
		discordOAuth, err = discord.New(discord.Config{
			ClientID:     cfg.DiscordClientID,
			ClientSecret: cfg.DiscordClientSecret,
			RedirectURI:  cfg.DiscordRedirectURI,
		})
		if err != nil {
			return fmt.Errorf("configure Discord OAuth: %w", err)
		}
	}
	auth := app.NewAuthService(discordOAuth, postgresadapter.NewAuthStore(pool))
	catalogStore := postgresadapter.NewCatalogStore(pool)
	catalog := app.NewCatalogService(catalogStore, catalogStore, time.Now)
	sellerStore := postgresadapter.NewSellerStore(pool)
	seller := app.NewSellerService(sellerStore, sellerStore, time.Now)

	handler := httpapi.New(httpapi.Config{
		AppBaseURL:        cfg.BaseURL,
		Auth:              auth,
		BodyLimit:         cfg.HTTPBodyLimit,
		Catalog:           catalog,
		Seller:            seller,
		Logger:            logger,
		Pinger:            pool,
		ReadinessTimeout:  cfg.ReadinessTimeout,
		SessionCookieName: cfg.SessionCookieName,
		StaticDir:         cfg.StaticDir,
	})

	httpServer := &http.Server{
		Addr:              cfg.HTTPAddr,
		Handler:           handler,
		ReadHeaderTimeout: 5 * time.Second,
		IdleTimeout:       60 * time.Second,
		MaxHeaderBytes:    1 << 20,
	}

	listener, err := net.Listen("tcp", cfg.HTTPAddr)
	if err != nil {
		return fmt.Errorf("listen on %s: %w", cfg.HTTPAddr, err)
	}
	logger.Info("KeebHub server listening", zap.String("address", listener.Addr().String()))

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	runner := platformserver.Runner{
		HTTPServer:      httpServer,
		Logger:          logger,
		ShutdownTimeout: cfg.ShutdownTimeout,
	}
	return runner.Run(ctx, listener)
}

func syncLogger(logger *zap.Logger) error {
	if err := logger.Sync(); err != nil && !errors.Is(err, syscall.EINVAL) && !errors.Is(err, os.ErrInvalid) {
		return fmt.Errorf("sync logger: %w", err)
	}
	return nil
}
