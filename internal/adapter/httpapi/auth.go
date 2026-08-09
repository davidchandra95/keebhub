package httpapi

import (
	"context"
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
	"unicode"

	"github.com/davidchandra95/keebhub/internal/app"
	"github.com/davidchandra95/keebhub/internal/domain"
	"github.com/labstack/echo/v5"
	"go.uber.org/zap"
)

const (
	oauthStateLifetime    = 10 * time.Minute
	sessionCookieLifetime = 30 * 24 * time.Hour
	oauthStateBytes       = 32
	maximumReturnTo       = 2048
	currentUserKey        = "keebhub.current_user"
)

type Authenticator interface {
	AuthorizationURL(state string) (string, error)
	LoginWithDiscord(ctx context.Context, code, previousRawToken string) (app.LoginResult, error)
	Authenticate(ctx context.Context, rawToken string) (domain.User, error)
	Logout(ctx context.Context, rawToken string) error
}

type authHandlers struct {
	auth              Authenticator
	logger            *zap.Logger
	random            io.Reader
	now               func() time.Time
	sessionCookieName string
	oauthCookieName   string
	secureCookies     bool
}

type oauthStateCookie struct {
	State     string `json:"state"`
	ReturnTo  string `json:"return_to"`
	ExpiresAt int64  `json:"expires_at"`
}

type currentUserResponse struct {
	User userResponse `json:"user"`
}

type userResponse struct {
	ID              string  `json:"id"`
	Handle          string  `json:"handle"`
	DiscordUsername string  `json:"discord_username"`
	DisplayName     string  `json:"display_name"`
	AvatarURL       *string `json:"avatar_url"`
	Location        *string `json:"location"`
	Bio             *string `json:"bio"`
	CreatedAt       string  `json:"created_at"`
}

func newAuthHandlers(cfg Config, secureCookies bool) authHandlers {
	random := cfg.Random
	if random == nil {
		random = rand.Reader
	}
	now := cfg.Now
	if now == nil {
		now = time.Now
	}
	cookieName := cfg.SessionCookieName
	if cookieName == "" {
		cookieName = "keebhub_session"
	}
	logger := cfg.Logger
	if logger == nil {
		logger = zap.NewNop()
	}
	return authHandlers{
		auth:              cfg.Auth,
		logger:            logger,
		random:            random,
		now:               now,
		sessionCookieName: cookieName,
		oauthCookieName:   cookieName + "_oauth",
		secureCookies:     secureCookies,
	}
}

func (h authHandlers) startDiscord(c *echo.Context) error {
	returnTo, err := validateReturnTo(c.QueryParam("return_to"))
	if err != nil {
		return &Error{
			Status:  http.StatusBadRequest,
			Code:    "invalid_return_to",
			Message: "The return address is not allowed.",
			Fields:  map[string]string{"return_to": "must be an internal application route"},
		}
	}

	state, err := h.randomValue(oauthStateBytes)
	if err != nil {
		return (&Error{
			Status:  http.StatusInternalServerError,
			Code:    "internal_error",
			Message: "An unexpected server error occurred.",
		}).Wrap(fmt.Errorf("generate OAuth state: %w", err))
	}
	if h.auth == nil {
		return c.Redirect(http.StatusFound, loginErrorURL("authentication_unavailable", RequestID(c), returnTo))
	}
	authorizationURL, err := h.auth.AuthorizationURL(state)
	if err != nil {
		h.logOAuthFailure(c, "start", err)
		return c.Redirect(http.StatusFound, loginErrorURL("authentication_unavailable", RequestID(c), returnTo))
	}

	now := h.now().UTC()
	cookieValue, err := encodeOAuthState(oauthStateCookie{
		State:     state,
		ReturnTo:  returnTo,
		ExpiresAt: now.Add(oauthStateLifetime).Unix(),
	})
	if err != nil {
		return (&Error{
			Status:  http.StatusInternalServerError,
			Code:    "internal_error",
			Message: "An unexpected server error occurred.",
		}).Wrap(fmt.Errorf("encode OAuth state: %w", err))
	}
	h.setOAuthCookie(c, cookieValue, now.Add(oauthStateLifetime))
	h.logger.Info("OAuth started", zap.String("request_id", RequestID(c)), zap.String("oauth_stage", "start"))
	return c.Redirect(http.StatusFound, authorizationURL)
}

func (h authHandlers) discordCallback(c *echo.Context) error {
	h.clearOAuthCookie(c)

	stateCookie, err := c.Cookie(h.oauthCookieName)
	if err != nil {
		h.logOAuthFailure(c, "state", domain.ErrInvalidSession)
		return c.Redirect(http.StatusFound, loginErrorURL("oauth_state_invalid", RequestID(c), "/"))
	}
	state, err := decodeOAuthState(stateCookie.Value, h.now().UTC())
	if err != nil || !matchingOAuthState(state.State, c.QueryParam("state")) {
		h.logOAuthFailure(c, "state", domain.ErrInvalidSession)
		return c.Redirect(http.StatusFound, loginErrorURL("oauth_state_invalid", RequestID(c), "/"))
	}

	if providerError := c.QueryParam("error"); providerError != "" {
		code := "authentication_failed"
		if providerError == "access_denied" {
			code = "authorization_denied"
		}
		h.logOAuthFailure(c, "authorization", fmt.Errorf("discord returned %s", providerError))
		return c.Redirect(http.StatusFound, loginErrorURL(code, RequestID(c), state.ReturnTo))
	}

	code := c.QueryParam("code")
	if strings.TrimSpace(code) == "" {
		h.logOAuthFailure(c, "authorization", errors.New("authorization code is missing"))
		return c.Redirect(http.StatusFound, loginErrorURL("authentication_failed", RequestID(c), state.ReturnTo))
	}
	if h.auth == nil {
		return c.Redirect(http.StatusFound, loginErrorURL("authentication_unavailable", RequestID(c), state.ReturnTo))
	}

	previousToken := ""
	if cookie, cookieErr := c.Cookie(h.sessionCookieName); cookieErr == nil {
		previousToken = cookie.Value
	}
	result, err := h.auth.LoginWithDiscord(c.Request().Context(), code, previousToken)
	if err != nil {
		errorCode := callbackErrorCode(err)
		h.logOAuthFailure(c, "login", err)
		return c.Redirect(http.StatusFound, loginErrorURL(errorCode, RequestID(c), state.ReturnTo))
	}

	h.setSessionCookie(c, result.RawToken, result.ExpiresAt)
	h.logger.Info("OAuth callback succeeded",
		zap.String("request_id", RequestID(c)),
		zap.String("oauth_stage", "callback"),
		zap.Int64("user_id", result.User.ID),
	)
	return c.Redirect(http.StatusFound, state.ReturnTo)
}

func (h authHandlers) logout(c *echo.Context) error {
	rawToken := ""
	if cookie, err := c.Cookie(h.sessionCookieName); err == nil {
		rawToken = cookie.Value
	}
	if h.auth != nil {
		if err := h.auth.Logout(c.Request().Context(), rawToken); err != nil {
			return (&Error{
				Status:  http.StatusInternalServerError,
				Code:    "internal_error",
				Message: "An unexpected server error occurred.",
			}).Wrap(err)
		}
	}
	h.clearSessionCookie(c)
	fields := []zap.Field{zap.String("request_id", RequestID(c))}
	if user, ok := CurrentUser(c); ok {
		fields = append(fields, zap.Int64("user_id", user.ID))
	}
	h.logger.Info("User logged out", fields...)
	return c.NoContent(http.StatusNoContent)
}

func (h authHandlers) me(c *echo.Context) error {
	user, ok := CurrentUser(c)
	if !ok {
		return echo.ErrUnauthorized
	}
	return c.JSON(http.StatusOK, currentUserResponse{User: userResponseFromDomain(user)})
}

func userResponseFromDomain(user domain.User) userResponse {
	return userResponse{
		ID: fmt.Sprintf("%d", user.ID), Handle: user.Handle, DiscordUsername: user.DiscordUsername,
		DisplayName: user.DisplayName, AvatarURL: user.AvatarURL, Location: user.Location, Bio: user.Bio,
		CreatedAt: formatTimestamp(user.CreatedAt),
	}
}

func (h authHandlers) sessionMiddleware() echo.MiddlewareFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c *echo.Context) error {
			if h.auth == nil {
				return next(c)
			}
			cookie, err := c.Cookie(h.sessionCookieName)
			if err != nil || cookie.Value == "" {
				return next(c)
			}
			user, err := h.auth.Authenticate(c.Request().Context(), cookie.Value)
			if err == nil {
				c.Set(currentUserKey, user)
				return next(c)
			}
			if errors.Is(err, domain.ErrInvalidSession) || errors.Is(err, domain.ErrUserDisabled) {
				h.clearSessionCookie(c)
				h.logger.Info("Session rejected", zap.String("request_id", RequestID(c)))
				return next(c)
			}
			return (&Error{
				Status:  http.StatusInternalServerError,
				Code:    "internal_error",
				Message: "An unexpected server error occurred.",
			}).Wrap(err)
		}
	}
}

func CurrentUser(c *echo.Context) (domain.User, bool) {
	user, ok := c.Get(currentUserKey).(domain.User)
	return user, ok
}

func (h authHandlers) randomValue(size int) (string, error) {
	value := make([]byte, size)
	if _, err := io.ReadFull(h.random, value); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(value), nil
}

func (h authHandlers) setOAuthCookie(c *echo.Context, value string, expiresAt time.Time) {
	c.SetCookie(&http.Cookie{
		Name:     h.oauthCookieName,
		Value:    value,
		Path:     "/auth/discord/callback",
		Expires:  expiresAt,
		MaxAge:   int(oauthStateLifetime / time.Second),
		HttpOnly: true,
		Secure:   h.secureCookies,
		SameSite: http.SameSiteLaxMode,
	})
}

func (h authHandlers) clearOAuthCookie(c *echo.Context) {
	c.SetCookie(expiredCookie(h.oauthCookieName, "/auth/discord/callback", h.secureCookies))
}

func (h authHandlers) setSessionCookie(c *echo.Context, value string, expiresAt time.Time) {
	c.SetCookie(&http.Cookie{
		Name:     h.sessionCookieName,
		Value:    value,
		Path:     "/",
		Expires:  expiresAt,
		MaxAge:   int(sessionCookieLifetime / time.Second),
		HttpOnly: true,
		Secure:   h.secureCookies,
		SameSite: http.SameSiteLaxMode,
	})
}

func (h authHandlers) clearSessionCookie(c *echo.Context) {
	c.SetCookie(expiredCookie(h.sessionCookieName, "/", h.secureCookies))
}

func expiredCookie(name, path string, secure bool) *http.Cookie {
	return &http.Cookie{
		Name:     name,
		Path:     path,
		Expires:  time.Unix(1, 0).UTC(),
		MaxAge:   -1,
		HttpOnly: true,
		Secure:   secure,
		SameSite: http.SameSiteLaxMode,
	}
}

func encodeOAuthState(state oauthStateCookie) (string, error) {
	encoded, err := json.Marshal(state)
	if err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(encoded), nil
}

func decodeOAuthState(value string, now time.Time) (oauthStateCookie, error) {
	decoded, err := base64.RawURLEncoding.DecodeString(value)
	if err != nil {
		return oauthStateCookie{}, err
	}
	var state oauthStateCookie
	if err := json.Unmarshal(decoded, &state); err != nil {
		return oauthStateCookie{}, err
	}
	if !validRandomValue(state.State, oauthStateBytes) || state.ExpiresAt <= now.Unix() {
		return oauthStateCookie{}, errors.New("OAuth state is invalid or expired")
	}
	returnTo, err := validateReturnTo(state.ReturnTo)
	if err != nil || returnTo != state.ReturnTo {
		return oauthStateCookie{}, errors.New("OAuth return target is invalid")
	}
	return state, nil
}

func matchingOAuthState(expected, presented string) bool {
	if !validRandomValue(expected, oauthStateBytes) || !validRandomValue(presented, oauthStateBytes) {
		return false
	}
	return subtle.ConstantTimeCompare([]byte(expected), []byte(presented)) == 1
}

func validRandomValue(value string, expectedBytes int) bool {
	decoded, err := base64.RawURLEncoding.DecodeString(value)
	return err == nil && len(decoded) == expectedBytes
}

func validateReturnTo(value string) (string, error) {
	if value == "" {
		return "/", nil
	}
	if len(value) > maximumReturnTo || !strings.HasPrefix(value, "/") || strings.HasPrefix(value, "//") || strings.Contains(value, "\\") {
		return "", errors.New("return target is not an internal path")
	}
	for _, character := range value {
		if unicode.IsControl(character) {
			return "", errors.New("return target contains a control character")
		}
	}
	parsed, err := url.ParseRequestURI(value)
	if err != nil || parsed.IsAbs() || parsed.Host != "" || parsed.User != nil || strings.HasPrefix(parsed.Path, "//") {
		return "", errors.New("return target is malformed")
	}
	path := parsed.Path
	for _, reserved := range []string{"/api", "/auth", "/assets", "/healthz", "/readyz"} {
		if path == reserved || strings.HasPrefix(path, reserved+"/") {
			return "", errors.New("return target is a reserved backend path")
		}
	}
	return value, nil
}

func loginErrorURL(code, requestID, returnTo string) string {
	query := url.Values{"error": {code}, "request_id": {requestID}}
	if returnTo != "/" {
		query.Set("return_to", returnTo)
	}
	return "/login?" + query.Encode()
}

func callbackErrorCode(err error) string {
	switch {
	case errors.Is(err, domain.ErrUserDisabled):
		return "account_disabled"
	case errors.Is(err, domain.ErrDiscordUnavailable):
		return "discord_unavailable"
	case errors.Is(err, domain.ErrAuthenticationUnavailable):
		return "authentication_unavailable"
	default:
		return "authentication_failed"
	}
}

func (h authHandlers) logOAuthFailure(c *echo.Context, stage string, err error) {
	h.logger.Warn("OAuth failed",
		zap.String("request_id", RequestID(c)),
		zap.String("oauth_stage", stage),
		zap.Error(err),
	)
}
