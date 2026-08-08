package httpapi

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"net/http"
	"net/url"
	"runtime/debug"
	"strings"

	"github.com/labstack/echo/v5"
	"github.com/labstack/echo/v5/middleware"
	"go.uber.org/zap"
)

const requestIDContextKey = "keebhub.request_id"

// RequestID returns the generated request ID stored in the Echo context.
func RequestID(c *echo.Context) string {
	value, _ := c.Get(requestIDContextKey).(string)
	return value
}

func requestIDMiddleware() echo.MiddlewareFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c *echo.Context) error {
			requestID, err := newRequestID()
			if err != nil {
				return &Error{
					Status:  http.StatusInternalServerError,
					Code:    "internal_error",
					Message: "An unexpected server error occurred.",
					Err:     fmt.Errorf("generate request ID: %w", err),
				}
			}
			c.Set(requestIDContextKey, requestID)
			c.Response().Header().Set(echo.HeaderXRequestID, requestID)
			return next(c)
		}
	}
}

func newRequestID() (string, error) {
	var value [16]byte
	if _, err := rand.Read(value[:]); err != nil {
		return "", err
	}
	return hex.EncodeToString(value[:]), nil
}

func securityHeadersMiddleware() echo.MiddlewareFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c *echo.Context) error {
			header := c.Response().Header()
			header.Set("X-Content-Type-Options", "nosniff")
			header.Set("X-Frame-Options", "DENY")
			header.Set("Referrer-Policy", "strict-origin-when-cross-origin")
			header.Set("Permissions-Policy", "camera=(), microphone=(), geolocation=()")
			return next(c)
		}
	}
}

func bodyLimitMiddleware(limit int64) echo.MiddlewareFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c *echo.Context) error {
			request := c.Request()
			if request.ContentLength > limit {
				return echo.ErrStatusRequestEntityTooLarge
			}
			if request.Body != nil {
				request.Body = http.MaxBytesReader(c.Response(), request.Body, limit)
			}
			return next(c)
		}
	}
}

func sameOriginMiddleware(appBaseURL string) echo.MiddlewareFunc {
	expected, _ := url.Parse(appBaseURL)

	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c *echo.Context) error {
			request := c.Request()
			if isSafeMethod(request.Method) {
				return next(c)
			}

			presented := request.Header.Get("Origin")
			if presented == "" {
				presented = request.Referer()
			}
			if !sameOrigin(expected, presented) {
				return &Error{
					Status:  http.StatusForbidden,
					Code:    "origin_rejected",
					Message: "The request origin is not allowed.",
				}
			}
			return next(c)
		}
	}
}

func isSafeMethod(method string) bool {
	switch method {
	case http.MethodGet, http.MethodHead, http.MethodOptions:
		return true
	default:
		return false
	}
}

func sameOrigin(expected *url.URL, presented string) bool {
	if expected == nil || presented == "" {
		return false
	}
	parsed, err := url.Parse(presented)
	if err != nil {
		return false
	}
	return strings.EqualFold(parsed.Scheme, expected.Scheme) && strings.EqualFold(parsed.Host, expected.Host)
}

func recoveryMiddleware(logger *zap.Logger) echo.MiddlewareFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c *echo.Context) (err error) {
			defer func() {
				if recovered := recover(); recovered != nil {
					logger.Error("panic recovered",
						zap.String("request_id", RequestID(c)),
						zap.Any("panic", recovered),
						zap.ByteString("stack", debug.Stack()),
					)
					err = &Error{
						Status:  http.StatusInternalServerError,
						Code:    "internal_error",
						Message: "An unexpected server error occurred.",
						Err:     fmt.Errorf("panic: %v", recovered),
					}
				}
			}()
			return next(c)
		}
	}
}

func accessLogMiddleware(logger *zap.Logger) echo.MiddlewareFunc {
	return middleware.RequestLoggerWithConfig(middleware.RequestLoggerConfig{
		HandleError:     true,
		LogLatency:      true,
		LogMethod:       true,
		LogURIPath:      true,
		LogRoutePath:    true,
		LogRequestID:    true,
		LogStatus:       true,
		LogResponseSize: true,
		LogValuesFunc: func(_ *echo.Context, values middleware.RequestLoggerValues) error {
			fields := []zap.Field{
				zap.String("request_id", values.RequestID),
				zap.String("method", values.Method),
				zap.String("path", values.URIPath),
				zap.String("route", values.RoutePath),
				zap.Int("status", values.Status),
				zap.Int64("response_size", values.ResponseSize),
				zap.Duration("duration", values.Latency),
			}
			if values.Error != nil {
				fields = append(fields, zap.Error(values.Error))
			}

			switch {
			case values.Status >= http.StatusInternalServerError:
				logger.Error("HTTP request", fields...)
			case values.Status >= http.StatusBadRequest:
				logger.Warn("HTTP request", fields...)
			default:
				logger.Info("HTTP request", fields...)
			}
			return nil
		},
	})
}
