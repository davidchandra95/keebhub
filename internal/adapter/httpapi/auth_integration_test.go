package httpapi_test

import (
	"context"
	"net/http"
	"net/url"
	"strings"
	"testing"

	"github.com/davidchandra95/keebhub/internal/adapter/httpapi"
	postgresadapter "github.com/davidchandra95/keebhub/internal/adapter/postgres"
	"github.com/davidchandra95/keebhub/internal/app"
	"github.com/davidchandra95/keebhub/internal/domain"
	"github.com/davidchandra95/keebhub/internal/testutil/testdatabase"
)

type integrationOAuth struct {
	identity domain.DiscordIdentity
}

func (o integrationOAuth) AuthorizationURL(state string) (string, error) {
	return "https://discord.example/authorize?state=" + url.QueryEscape(state), nil
}

func (o integrationOAuth) IdentityFromCode(context.Context, string) (domain.DiscordIdentity, error) {
	return o.identity, nil
}

func TestDiscordAuthenticationHTTPFlowWithPostgreSQL(t *testing.T) {
	database := testdatabase.Open(t)
	service := app.NewAuthService(integrationOAuth{identity: domain.DiscordIdentity{
		ID: "400000000000000001", Username: "integration.user", DisplayName: "Integration User",
	}}, postgresadapter.NewAuthStore(database.Pool))
	handler := newHandlerConfig(t, database.Pool, httpapi.Config{
		AppBaseURL:        "http://localhost:8080",
		Auth:              service,
		SessionCookieName: "keebhub_session",
	})

	start := performRequest(handler, http.MethodGet, "/auth/discord?return_to=%2Flistings%2F1001", "", nil)
	stateCookie := findCookie(t, start, "keebhub_session_oauth")
	authorizationURL, err := url.Parse(start.Header().Get("Location"))
	if err != nil {
		t.Fatalf("parse authorization redirect: %v", err)
	}
	state := authorizationURL.Query().Get("state")
	callback := performRequest(handler, http.MethodGet, "/auth/discord/callback?code=code&state="+url.QueryEscape(state), "", map[string]string{
		"Cookie": stateCookie.Name + "=" + stateCookie.Value,
	})
	if callback.Code != http.StatusFound || callback.Header().Get("Location") != "/listings/1001" {
		t.Fatalf("callback = %d %q", callback.Code, callback.Header().Get("Location"))
	}
	sessionCookie := findCookie(t, callback, "keebhub_session")

	me := performRequest(handler, http.MethodGet, "/api/v1/me", "", map[string]string{
		"Cookie": sessionCookie.Name + "=" + sessionCookie.Value,
	})
	if me.Code != http.StatusOK || !strings.Contains(me.Body.String(), `"handle":"integration-user"`) {
		t.Fatalf("me = %d %s", me.Code, me.Body.String())
	}

	var storedHash []byte
	if err := database.Pool.QueryRow(context.Background(), `SELECT token_hash FROM sessions`).Scan(&storedHash); err != nil {
		t.Fatalf("read stored session: %v", err)
	}
	if string(storedHash) == sessionCookie.Value || len(storedHash) != 32 {
		t.Errorf("stored session has length %d or matches raw token", len(storedHash))
	}

	logout := performRequest(handler, http.MethodPost, "/auth/logout", "", map[string]string{
		"Origin": "http://localhost:8080",
		"Cookie": sessionCookie.Name + "=" + sessionCookie.Value,
	})
	if logout.Code != http.StatusNoContent {
		t.Fatalf("logout = %d %s", logout.Code, logout.Body.String())
	}
	afterLogout := performRequest(handler, http.MethodGet, "/api/v1/me", "", map[string]string{
		"Cookie": sessionCookie.Name + "=" + sessionCookie.Value,
	})
	if afterLogout.Code != http.StatusUnauthorized {
		t.Fatalf("me after logout = %d", afterLogout.Code)
	}
}

func TestDisabledUserCannotRestoreHTTPAuthentication(t *testing.T) {
	database := testdatabase.Open(t)
	service := app.NewAuthService(integrationOAuth{identity: domain.DiscordIdentity{
		ID: "500000000000000001", Username: "disabled.user", DisplayName: "Disabled User",
	}}, postgresadapter.NewAuthStore(database.Pool))
	handler := newHandlerConfig(t, database.Pool, httpapi.Config{
		AppBaseURL:        "http://localhost:8080",
		Auth:              service,
		SessionCookieName: "keebhub_session",
	})

	login := completeIntegrationLogin(t, handler)
	if _, err := database.Pool.Exec(context.Background(), `UPDATE users SET status = 'disabled'`); err != nil {
		t.Fatalf("disable user: %v", err)
	}
	me := performRequest(handler, http.MethodGet, "/api/v1/me", "", map[string]string{
		"Cookie": login.Name + "=" + login.Value,
	})
	if me.Code != http.StatusUnauthorized {
		t.Fatalf("disabled me = %d", me.Code)
	}

	start := performRequest(handler, http.MethodGet, "/auth/discord", "", nil)
	stateCookie := findCookie(t, start, "keebhub_session_oauth")
	authorizationURL, _ := url.Parse(start.Header().Get("Location"))
	callback := performRequest(handler, http.MethodGet, "/auth/discord/callback?code=code&state="+url.QueryEscape(authorizationURL.Query().Get("state")), "", map[string]string{
		"Cookie": stateCookie.Name + "=" + stateCookie.Value,
	})
	if !strings.Contains(callback.Header().Get("Location"), "error=account_disabled") {
		t.Fatalf("disabled callback redirect = %q", callback.Header().Get("Location"))
	}
}

func completeIntegrationLogin(t *testing.T, handler http.Handler) *http.Cookie {
	t.Helper()
	start := performRequest(handler, http.MethodGet, "/auth/discord", "", nil)
	stateCookie := findCookie(t, start, "keebhub_session_oauth")
	authorizationURL, err := url.Parse(start.Header().Get("Location"))
	if err != nil {
		t.Fatalf("parse authorization redirect: %v", err)
	}
	callback := performRequest(handler, http.MethodGet, "/auth/discord/callback?code=code&state="+url.QueryEscape(authorizationURL.Query().Get("state")), "", map[string]string{
		"Cookie": stateCookie.Name + "=" + stateCookie.Value,
	})
	if callback.Code != http.StatusFound {
		t.Fatalf("callback = %d %s", callback.Code, callback.Body.String())
	}
	return findCookie(t, callback, "keebhub_session")
}
