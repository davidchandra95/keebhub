// Package httpapi provides the Echo HTTP adapter and transport-level policy.
package httpapi

import (
	"context"
	"io"
	"net/http"
	"net/url"
	"time"

	"github.com/labstack/echo/v5"
	"go.uber.org/zap"
)

// Pinger is the database behavior consumed by the readiness handler.
type Pinger interface {
	Ping(context.Context) error
}

// Config contains injected HTTP adapter dependencies and transport policy.
type Config struct {
	AppBaseURL        string
	Auth              Authenticator
	BodyLimit         int64
	Catalog           CatalogService
	Seller            SellerService
	Logger            *zap.Logger
	Now               func() time.Time
	Pinger            Pinger
	Random            io.Reader
	ReadinessTimeout  time.Duration
	SessionCookieName string
	StaticDir         string
}

type healthHandlers struct {
	pinger           Pinger
	readinessTimeout time.Duration
}

// New builds the complete HTTP handler without opening sockets.
func New(cfg Config) http.Handler {
	logger := cfg.Logger
	if logger == nil {
		logger = zap.NewNop()
	}
	static := newStaticHandler(cfg.StaticDir)
	baseURL, _ := url.Parse(cfg.AppBaseURL)
	auth := newAuthHandlers(cfg, baseURL != nil && baseURL.Scheme == "https")
	catalog := newCatalogHandlers(cfg)
	seller := newSellerHandlers(cfg)

	e := echo.NewWithConfig(echo.Config{
		HTTPErrorHandler: errorHandler(logger),
		Router: echo.NewRouter(echo.RouterConfig{
			NotFoundHandler: static.serve,
		}),
	})

	e.Use(
		requestIDMiddleware(),
		securityHeadersMiddleware(),
		bodyLimitMiddleware(cfg.BodyLimit),
		sameOriginMiddleware(cfg.AppBaseURL),
		auth.sessionMiddleware(),
		accessLogMiddleware(logger),
		recoveryMiddleware(logger),
	)

	health := healthHandlers{
		pinger:           cfg.Pinger,
		readinessTimeout: cfg.ReadinessTimeout,
	}
	e.GET("/healthz", health.health)
	e.GET("/readyz", health.ready)
	e.GET("/auth/discord", auth.startDiscord)
	e.GET("/auth/discord/callback", auth.discordCallback)
	e.POST("/auth/logout", auth.logout)
	e.GET("/api/v1/me", auth.me)
	e.PATCH("/api/v1/me", seller.updateProfile)
	e.GET("/api/v1/categories", catalog.listCategories)
	e.GET("/api/v1/listings", catalog.searchListings)
	e.POST("/api/v1/listings", catalog.createListing)
	e.PATCH("/api/v1/listings/:listing_id", catalog.updateListing)
	e.POST("/api/v1/listings/:listing_id/status", catalog.changeListingStatus)
	e.GET("/api/v1/me/listings", catalog.listOwnedListings)
	e.GET("/api/v1/listings/:listing_id", catalog.getListing)
	e.GET("/api/v1/users/:handle/listings", seller.listSellerListings)
	e.GET("/api/v1/users/:handle", seller.getSellerProfile)

	return e
}

func (h healthHandlers) health(c *echo.Context) error {
	return c.JSON(http.StatusOK, map[string]string{"status": "ok"})
}

func (h healthHandlers) ready(c *echo.Context) error {
	if h.pinger == nil {
		return echo.ErrServiceUnavailable
	}

	ctx, cancel := context.WithTimeout(c.Request().Context(), h.readinessTimeout)
	defer cancel()
	if err := h.pinger.Ping(ctx); err != nil {
		return (&Error{
			Status:  http.StatusServiceUnavailable,
			Code:    "service_unavailable",
			Message: "The service is temporarily unavailable.",
		}).Wrap(err)
	}
	return c.JSON(http.StatusOK, map[string]string{"status": "ready"})
}

// Wrap returns a copy of the HTTP error with an internal cause.
func (e *Error) Wrap(err error) error {
	copy := *e
	copy.Err = err
	return &copy
}
