package discord_test

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/davidchandra95/keebhub/internal/adapter/discord"
	"github.com/davidchandra95/keebhub/internal/domain"
)

func TestAuthorizationURLUsesIdentifyScope(t *testing.T) {
	t.Parallel()

	client := newClient(t, discord.Config{})
	got, err := client.AuthorizationURL("state-value")
	if err != nil {
		t.Fatalf("AuthorizationURL() error = %v", err)
	}
	parsed, err := url.Parse(got)
	if err != nil {
		t.Fatalf("parse URL: %v", err)
	}
	query := parsed.Query()
	if query.Get("client_id") != "client-id" || query.Get("scope") != "identify" || query.Get("response_type") != "code" || query.Get("state") != "state-value" {
		t.Errorf("authorization query = %v", query)
	}
}

func TestIdentityFromCodeExchangesCodeAndFetchesUser(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		switch request.URL.Path {
		case "/token":
			username, password, ok := request.BasicAuth()
			if !ok || username != "client-id" || password != "client-secret" {
				t.Errorf("basic auth = %q/%q/%v", username, password, ok)
			}
			if err := request.ParseForm(); err != nil {
				t.Errorf("parse form: %v", err)
			}
			if request.Form.Get("grant_type") != "authorization_code" || request.Form.Get("code") != "code-value" || request.Form.Get("redirect_uri") != "http://localhost/callback" {
				t.Errorf("token form = %v", request.Form)
			}
			response.Header().Set("Content-Type", "application/json")
			_, _ = response.Write([]byte(`{"access_token":"access-value","token_type":"Bearer"}`))
		case "/user":
			if request.Header.Get("Authorization") != "Bearer access-value" {
				t.Errorf("Authorization = %q", request.Header.Get("Authorization"))
			}
			response.Header().Set("Content-Type", "application/json")
			_, _ = response.Write([]byte(`{"id":"123456789","username":"gunawan.keyboard","global_name":"Gunawan","avatar":"avatar_hash"}`))
		default:
			http.NotFound(response, request)
		}
	}))
	t.Cleanup(server.Close)

	client := newClient(t, discord.Config{TokenEndpoint: server.URL + "/token", UserEndpoint: server.URL + "/user"})
	identity, err := client.IdentityFromCode(context.Background(), "code-value")
	if err != nil {
		t.Fatalf("IdentityFromCode() error = %v", err)
	}
	if identity.ID != "123456789" || identity.Username != "gunawan.keyboard" || identity.DisplayName != "Gunawan" {
		t.Errorf("identity = %+v", identity)
	}
	if identity.AvatarURL == nil || !strings.Contains(*identity.AvatarURL, "/123456789/avatar_hash.png") {
		t.Errorf("avatar URL = %v", identity.AvatarURL)
	}
}

func TestIdentityFromCodeMapsUpstreamFailures(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		tokenBody string
		tokenCode int
		userBody  string
		userCode  int
	}{
		{name: "token status", tokenCode: http.StatusBadGateway},
		{name: "malformed token", tokenCode: http.StatusOK, tokenBody: `{}`},
		{name: "user status", tokenCode: http.StatusOK, tokenBody: `{"access_token":"token","token_type":"Bearer"}`, userCode: http.StatusBadGateway},
		{name: "malformed user", tokenCode: http.StatusOK, tokenBody: `{"access_token":"token","token_type":"Bearer"}`, userCode: http.StatusOK, userBody: `{"id":"not-numeric","username":"name"}`},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
				if request.URL.Path == "/token" {
					response.WriteHeader(tt.tokenCode)
					_, _ = response.Write([]byte(tt.tokenBody))
					return
				}
				response.WriteHeader(tt.userCode)
				_, _ = response.Write([]byte(tt.userBody))
			}))
			t.Cleanup(server.Close)
			client := newClient(t, discord.Config{TokenEndpoint: server.URL + "/token", UserEndpoint: server.URL + "/user"})
			_, err := client.IdentityFromCode(context.Background(), "code")
			if !errors.Is(err, domain.ErrDiscordUnavailable) {
				t.Errorf("error = %v", err)
			}
		})
	}
}

func TestIdentityFromCodeHonorsHTTPTimeout(t *testing.T) {
	t.Parallel()

	client := newClient(t, discord.Config{
		HTTPClient: &http.Client{
			Timeout: time.Millisecond,
			Transport: roundTripFunc(func(request *http.Request) (*http.Response, error) {
				<-request.Context().Done()
				return nil, request.Context().Err()
			}),
		},
		TokenEndpoint: "https://discord.example/token",
	})
	_, err := client.IdentityFromCode(context.Background(), "code")
	if !errors.Is(err, domain.ErrDiscordUnavailable) {
		t.Fatalf("error = %v", err)
	}
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(request *http.Request) (*http.Response, error) {
	return f(request)
}

func newClient(t *testing.T, overrides discord.Config) *discord.Client {
	t.Helper()
	overrides.ClientID = "client-id"
	overrides.ClientSecret = "client-secret"
	overrides.RedirectURI = "http://localhost/callback"
	if overrides.AuthorizationEndpoint == "" {
		overrides.AuthorizationEndpoint = "https://discord.example/authorize"
	}
	client, err := discord.New(overrides)
	if err != nil {
		t.Fatalf("discord.New() error = %v", err)
	}
	return client
}
