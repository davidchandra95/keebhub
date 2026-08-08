package httpapi_test

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/davidchandra95/keebhub/internal/adapter/httpapi"
	"github.com/davidchandra95/keebhub/internal/app"
	"github.com/davidchandra95/keebhub/internal/domain"
	"go.uber.org/zap"
)

type fakePinger struct {
	err error
}

type fakeAuthenticator struct {
	authorizationState string
	authorizationErr   error
	loginResult        app.LoginResult
	loginErr           error
	loginCalls         int
	authenticateUser   domain.User
	authenticateErr    error
	logoutToken        string
}

func (a *fakeAuthenticator) AuthorizationURL(state string) (string, error) {
	a.authorizationState = state
	return "https://discord.example/authorize?state=" + url.QueryEscape(state), a.authorizationErr
}

func (a *fakeAuthenticator) LoginWithDiscord(_ context.Context, _, _ string) (app.LoginResult, error) {
	a.loginCalls++
	return a.loginResult, a.loginErr
}

func (a *fakeAuthenticator) Authenticate(context.Context, string) (domain.User, error) {
	return a.authenticateUser, a.authenticateErr
}

func (a *fakeAuthenticator) Logout(_ context.Context, rawToken string) error {
	a.logoutToken = rawToken
	return nil
}

func (p fakePinger) Ping(context.Context) error {
	return p.err
}

func TestHealthDoesNotDependOnDatabase(t *testing.T) {
	t.Parallel()

	handler := newHandler(t, fakePinger{err: errors.New("database unavailable")})
	response := performRequest(handler, http.MethodGet, "/healthz", "", nil)

	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", response.Code, http.StatusOK)
	}
	assertJSONStatus(t, response.Body.String(), "ok")
}

func TestReadinessReflectsDatabase(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		pingError  error
		wantStatus int
		wantCode   string
	}{
		{name: "ready", wantStatus: http.StatusOK},
		{name: "database unavailable", pingError: errors.New("down"), wantStatus: http.StatusServiceUnavailable, wantCode: "service_unavailable"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			handler := newHandler(t, fakePinger{err: tt.pingError})
			response := performRequest(handler, http.MethodGet, "/readyz", "", nil)
			if response.Code != tt.wantStatus {
				t.Fatalf("status = %d, want %d: %s", response.Code, tt.wantStatus, response.Body.String())
			}
			if tt.wantCode != "" {
				assertErrorCode(t, response.Body.Bytes(), tt.wantCode)
			}
		})
	}
}

func TestAPINotFoundIsJSONWithGeneratedRequestID(t *testing.T) {
	t.Parallel()

	handler := newHandler(t, fakePinger{})
	response := performRequest(handler, http.MethodGet, "/api/v1/missing", "", map[string]string{
		"X-Request-ID": "untrusted-client-value",
	})

	if response.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", response.Code)
	}

	requestID := response.Header().Get("X-Request-ID")
	if requestID == "" || requestID == "untrusted-client-value" {
		t.Fatalf("generated request ID = %q", requestID)
	}

	var body struct {
		Error struct {
			Code      string `json:"code"`
			RequestID string `json:"request_id"`
		} `json:"error"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if body.Error.Code != "not_found" || body.Error.RequestID != requestID {
		t.Errorf("error = %+v, request header = %q", body.Error, requestID)
	}
}

func TestStaticFallbackDoesNotSwallowReservedOrAssetPaths(t *testing.T) {
	t.Parallel()

	handler := newHandler(t, fakePinger{})

	tests := []struct {
		name       string
		path       string
		accept     string
		wantStatus int
		wantBody   string
	}{
		{name: "client route", path: "/inbox/42", accept: "text/html", wantStatus: http.StatusOK, wantBody: "KeebHub shell"},
		{name: "API route", path: "/api/v1/unknown", accept: "text/html", wantStatus: http.StatusNotFound, wantBody: `"code":"not_found"`},
		{name: "auth route", path: "/auth/unknown", accept: "text/html", wantStatus: http.StatusNotFound, wantBody: `"code":"not_found"`},
		{name: "missing asset", path: "/assets/missing.js", accept: "text/html", wantStatus: http.StatusNotFound, wantBody: `"code":"not_found"`},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			response := performRequest(handler, http.MethodGet, tt.path, "", map[string]string{"Accept": tt.accept})
			if response.Code != tt.wantStatus {
				t.Fatalf("status = %d, want %d: %s", response.Code, tt.wantStatus, response.Body.String())
			}
			if !strings.Contains(response.Body.String(), tt.wantBody) {
				t.Errorf("body = %q, want substring %q", response.Body.String(), tt.wantBody)
			}
		})
	}
}

func TestUnsafeRequestRequiresSameOrigin(t *testing.T) {
	t.Parallel()

	handler := newHandler(t, fakePinger{})

	wrongOrigin := performRequest(handler, http.MethodPost, "/auth/logout", "", map[string]string{
		"Origin": "https://evil.example",
	})
	if wrongOrigin.Code != http.StatusForbidden {
		t.Fatalf("wrong-origin status = %d, want 403", wrongOrigin.Code)
	}

	matchingOrigin := performRequest(handler, http.MethodPost, "/auth/logout", "", map[string]string{
		"Origin": "http://localhost:8080",
	})
	if matchingOrigin.Code != http.StatusNoContent {
		t.Fatalf("matching-origin status = %d, want 204", matchingOrigin.Code)
	}
}

func TestUnsafeRequestAcceptsMatchingRefererWhenOriginIsMissing(t *testing.T) {
	t.Parallel()

	handler := newHandler(t, fakePinger{})
	response := performRequest(handler, http.MethodPost, "/auth/logout", "", map[string]string{
		"Referer": "http://localhost:8080/login",
	})
	if response.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want 204", response.Code)
	}
}

func TestDiscordStartSetsStateCookieAndRedirects(t *testing.T) {
	t.Parallel()

	auth := &fakeAuthenticator{}
	handler := newHandlerWithAuth(t, "https://keebhub.example", auth)
	response := performRequest(handler, http.MethodGet, "/auth/discord?return_to=%2Flistings%2F1001", "", nil)
	if response.Code != http.StatusFound {
		t.Fatalf("status = %d, want 302: %s", response.Code, response.Body.String())
	}
	if auth.authorizationState == "" || !strings.Contains(response.Header().Get("Location"), url.QueryEscape(auth.authorizationState)) {
		t.Errorf("redirect = %q, state = %q", response.Header().Get("Location"), auth.authorizationState)
	}
	cookie := findCookie(t, response, "keebhub_session_oauth")
	if !cookie.HttpOnly || !cookie.Secure || cookie.SameSite != http.SameSiteLaxMode || cookie.Path != "/auth/discord/callback" || cookie.MaxAge != 600 {
		t.Errorf("OAuth cookie = %+v", cookie)
	}
}

func TestDiscordStartRejectsExternalAndReservedReturnTargets(t *testing.T) {
	t.Parallel()

	handler := newHandlerWithAuth(t, "http://localhost:8080", &fakeAuthenticator{})
	for _, target := range []string{"https://evil.example", "//evil.example", "/auth/logout", "/api/v1/me", `/\\evil.example`} {
		response := performRequest(handler, http.MethodGet, "/auth/discord?return_to="+url.QueryEscape(target), "", nil)
		if response.Code != http.StatusBadRequest {
			t.Errorf("target %q status = %d, want 400", target, response.Code)
		}
	}
}

func TestDiscordCallbackCreatesSessionAndClearsState(t *testing.T) {
	t.Parallel()

	expiresAt := time.Date(2026, 9, 7, 12, 0, 0, 0, time.UTC)
	auth := &fakeAuthenticator{loginResult: app.LoginResult{
		User: domain.User{ID: 42, Handle: "gunawan"}, RawToken: "raw-session-token", ExpiresAt: expiresAt,
	}}
	handler := newHandlerWithAuth(t, "http://localhost:8080", auth)
	start := performRequest(handler, http.MethodGet, "/auth/discord?return_to=%2Flistings%2F1001", "", nil)
	stateCookie := findCookie(t, start, "keebhub_session_oauth")

	callback := performRequest(handler, http.MethodGet, "/auth/discord/callback?code=oauth-code&state="+url.QueryEscape(auth.authorizationState), "", map[string]string{
		"Cookie": stateCookie.Name + "=" + stateCookie.Value,
	})
	if callback.Code != http.StatusFound || callback.Header().Get("Location") != "/listings/1001" {
		t.Fatalf("callback = %d %q", callback.Code, callback.Header().Get("Location"))
	}
	if auth.loginCalls != 1 {
		t.Fatalf("login calls = %d, want 1", auth.loginCalls)
	}
	session := findCookie(t, callback, "keebhub_session")
	if session.Value != "raw-session-token" || !session.HttpOnly || session.Secure || session.Path != "/" || session.MaxAge != 30*24*60*60 {
		t.Errorf("session cookie = %+v", session)
	}
	cleared := findCookie(t, callback, "keebhub_session_oauth")
	if cleared.MaxAge != -1 {
		t.Errorf("OAuth clear cookie = %+v", cleared)
	}
}

func TestDiscordCallbackRejectsMismatchedStateBeforeLogin(t *testing.T) {
	t.Parallel()

	auth := &fakeAuthenticator{}
	handler := newHandlerWithAuth(t, "http://localhost:8080", auth)
	start := performRequest(handler, http.MethodGet, "/auth/discord", "", nil)
	stateCookie := findCookie(t, start, "keebhub_session_oauth")
	wrongState := strings.Repeat("A", 43)
	callback := performRequest(handler, http.MethodGet, "/auth/discord/callback?code=oauth-code&state="+wrongState, "", map[string]string{
		"Cookie": stateCookie.Name + "=" + stateCookie.Value,
	})
	if callback.Code != http.StatusFound || !strings.Contains(callback.Header().Get("Location"), "error=oauth_state_invalid") {
		t.Fatalf("callback = %d %q", callback.Code, callback.Header().Get("Location"))
	}
	if auth.loginCalls != 0 {
		t.Errorf("login calls = %d, want 0", auth.loginCalls)
	}
}

func TestDiscordCallbackMapsDisabledUserToLoginError(t *testing.T) {
	t.Parallel()

	auth := &fakeAuthenticator{loginErr: domain.ErrUserDisabled}
	handler := newHandlerWithAuth(t, "http://localhost:8080", auth)
	start := performRequest(handler, http.MethodGet, "/auth/discord", "", nil)
	stateCookie := findCookie(t, start, "keebhub_session_oauth")
	callback := performRequest(handler, http.MethodGet, "/auth/discord/callback?code=oauth-code&state="+url.QueryEscape(auth.authorizationState), "", map[string]string{
		"Cookie": stateCookie.Name + "=" + stateCookie.Value,
	})
	if !strings.Contains(callback.Header().Get("Location"), "error=account_disabled") {
		t.Fatalf("redirect = %q", callback.Header().Get("Location"))
	}
}

func TestCurrentUserRequiresActiveSession(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		auth       *fakeAuthenticator
		cookie     string
		wantStatus int
		wantBody   string
	}{
		{name: "missing", auth: &fakeAuthenticator{}, wantStatus: http.StatusUnauthorized},
		{name: "invalid", auth: &fakeAuthenticator{authenticateErr: domain.ErrInvalidSession}, cookie: "invalid", wantStatus: http.StatusUnauthorized},
		{name: "disabled", auth: &fakeAuthenticator{authenticateErr: domain.ErrUserDisabled}, cookie: "disabled", wantStatus: http.StatusUnauthorized},
		{name: "active", auth: &fakeAuthenticator{authenticateUser: domain.User{ID: 42, Handle: "gunawan", DiscordUsername: "gunawan", DisplayName: "Gunawan"}}, cookie: "valid", wantStatus: http.StatusOK, wantBody: `"id":"42"`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			headers := map[string]string{}
			if tt.cookie != "" {
				headers["Cookie"] = "keebhub_session=" + tt.cookie
			}
			response := performRequest(newHandlerWithAuth(t, "http://localhost:8080", tt.auth), http.MethodGet, "/api/v1/me", "", headers)
			if response.Code != tt.wantStatus {
				t.Fatalf("status = %d, want %d: %s", response.Code, tt.wantStatus, response.Body.String())
			}
			if tt.wantBody != "" && !strings.Contains(response.Body.String(), tt.wantBody) {
				t.Errorf("body = %s", response.Body.String())
			}
		})
	}
}

func TestLogoutClearsAndDeletesPresentedSession(t *testing.T) {
	t.Parallel()

	auth := &fakeAuthenticator{authenticateErr: domain.ErrInvalidSession}
	handler := newHandlerWithAuth(t, "http://localhost:8080", auth)
	response := performRequest(handler, http.MethodPost, "/auth/logout", "", map[string]string{
		"Origin": "http://localhost:8080",
		"Cookie": "keebhub_session=presented-token",
	})
	if response.Code != http.StatusNoContent || auth.logoutToken != "presented-token" {
		t.Fatalf("status = %d, logout token = %q", response.Code, auth.logoutToken)
	}
	if findCookie(t, response, "keebhub_session").MaxAge != -1 {
		t.Error("session cookie was not cleared")
	}
}

func TestDecodeJSONIsStrict(t *testing.T) {
	t.Parallel()

	type payload struct {
		Name string `json:"name"`
	}

	tests := []struct {
		name    string
		body    string
		want    payload
		wantErr bool
	}{
		{name: "valid", body: `{"name":"keeb"}`, want: payload{Name: "keeb"}},
		{name: "unknown field", body: `{"name":"keeb","extra":true}`, wantErr: true},
		{name: "multiple values", body: `{"name":"keeb"}{"name":"hub"}`, wantErr: true},
		{name: "empty", body: ``, wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			request := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(tt.body))
			var got payload
			err := httpapi.DecodeJSON(request, &got)
			if tt.wantErr {
				if err == nil {
					t.Fatal("DecodeJSON() error = nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("DecodeJSON() error = %v", err)
			}
			if got != tt.want {
				t.Errorf("payload = %+v, want %+v", got, tt.want)
			}
		})
	}
}

func newHandler(t *testing.T, pinger httpapi.Pinger) http.Handler {
	return newHandlerConfig(t, pinger, httpapi.Config{AppBaseURL: "http://localhost:8080"})
}

func newHandlerWithAuth(t *testing.T, baseURL string, auth httpapi.Authenticator) http.Handler {
	return newHandlerConfig(t, fakePinger{}, httpapi.Config{
		AppBaseURL:        baseURL,
		Auth:              auth,
		SessionCookieName: "keebhub_session",
		Now: func() time.Time {
			return time.Date(2026, 8, 8, 12, 0, 0, 0, time.UTC)
		},
	})
}

func newHandlerConfig(t *testing.T, pinger httpapi.Pinger, cfg httpapi.Config) http.Handler {
	t.Helper()

	staticDir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(staticDir, "assets"), 0o755); err != nil {
		t.Fatalf("create assets: %v", err)
	}
	if err := os.WriteFile(filepath.Join(staticDir, "index.html"), []byte("<html>KeebHub shell</html>"), 0o600); err != nil {
		t.Fatalf("write index: %v", err)
	}
	if err := os.WriteFile(filepath.Join(staticDir, "assets", "app.js"), []byte("console.log('ok')"), 0o600); err != nil {
		t.Fatalf("write asset: %v", err)
	}

	cfg.BodyLimit = 1 << 20
	cfg.Logger = zap.NewNop()
	cfg.Pinger = pinger
	cfg.ReadinessTimeout = time.Second
	cfg.StaticDir = staticDir
	return httpapi.New(cfg)
}

func performRequest(handler http.Handler, method, path, body string, headers map[string]string) *httptest.ResponseRecorder {
	request := httptest.NewRequest(method, path, strings.NewReader(body))
	for key, value := range headers {
		request.Header.Set(key, value)
	}
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	return response
}

func assertJSONStatus(t *testing.T, body, want string) {
	t.Helper()

	var response struct {
		Status string `json:"status"`
	}
	if err := json.Unmarshal([]byte(body), &response); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if response.Status != want {
		t.Errorf("status body = %q, want %q", response.Status, want)
	}
}

func findCookie(t *testing.T, response *httptest.ResponseRecorder, name string) *http.Cookie {
	t.Helper()
	for _, cookie := range response.Result().Cookies() {
		if cookie.Name == name {
			return cookie
		}
	}
	t.Fatalf("cookie %q not found in %v", name, response.Header().Values("Set-Cookie"))
	return nil
}

func assertErrorCode(t *testing.T, body []byte, want string) {
	t.Helper()

	var response struct {
		Error struct {
			Code string `json:"code"`
		} `json:"error"`
	}
	if err := json.Unmarshal(body, &response); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if response.Error.Code != want {
		t.Errorf("error code = %q, want %q", response.Error.Code, want)
	}
}
