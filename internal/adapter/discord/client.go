// Package discord implements the external Discord OAuth2 adapter.
package discord

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/davidchandra95/keebhub/internal/domain"
)

const (
	defaultAuthorizationEndpoint = "https://discord.com/oauth2/authorize"
	defaultTokenEndpoint         = "https://discord.com/api/v10/oauth2/token"
	defaultUserEndpoint          = "https://discord.com/api/v10/users/@me"
	maximumResponseBytes         = 1 << 20
)

type Config struct {
	ClientID              string
	ClientSecret          string
	RedirectURI           string
	HTTPClient            *http.Client
	AuthorizationEndpoint string
	TokenEndpoint         string
	UserEndpoint          string
}

type Client struct {
	clientID              string
	clientSecret          string
	redirectURI           string
	httpClient            *http.Client
	authorizationEndpoint string
	tokenEndpoint         string
	userEndpoint          string
}

func New(cfg Config) (*Client, error) {
	if strings.TrimSpace(cfg.ClientID) == "" || strings.TrimSpace(cfg.ClientSecret) == "" || strings.TrimSpace(cfg.RedirectURI) == "" {
		return nil, fmt.Errorf("discord OAuth credentials and redirect URI are required")
	}

	client := cfg.HTTPClient
	if client == nil {
		client = &http.Client{Timeout: 10 * time.Second}
	}
	return &Client{
		clientID:              cfg.ClientID,
		clientSecret:          cfg.ClientSecret,
		redirectURI:           cfg.RedirectURI,
		httpClient:            client,
		authorizationEndpoint: valueOrDefault(cfg.AuthorizationEndpoint, defaultAuthorizationEndpoint),
		tokenEndpoint:         valueOrDefault(cfg.TokenEndpoint, defaultTokenEndpoint),
		userEndpoint:          valueOrDefault(cfg.UserEndpoint, defaultUserEndpoint),
	}, nil
}

func (c *Client) AuthorizationURL(state string) (string, error) {
	if state == "" {
		return "", fmt.Errorf("OAuth state is empty")
	}
	endpoint, err := url.Parse(c.authorizationEndpoint)
	if err != nil {
		return "", fmt.Errorf("parse Discord authorization endpoint: %w", err)
	}
	query := endpoint.Query()
	query.Set("client_id", c.clientID)
	query.Set("redirect_uri", c.redirectURI)
	query.Set("response_type", "code")
	query.Set("scope", "identify")
	query.Set("state", state)
	endpoint.RawQuery = query.Encode()
	return endpoint.String(), nil
}

func (c *Client) IdentityFromCode(ctx context.Context, code string) (domain.DiscordIdentity, error) {
	if strings.TrimSpace(code) == "" {
		return domain.DiscordIdentity{}, fmt.Errorf("%w: authorization code is empty", domain.ErrDiscordUnavailable)
	}
	accessToken, err := c.exchangeCode(ctx, code)
	if err != nil {
		return domain.DiscordIdentity{}, err
	}
	return c.currentUser(ctx, accessToken)
}

func (c *Client) exchangeCode(ctx context.Context, code string) (string, error) {
	form := url.Values{
		"grant_type":   {"authorization_code"},
		"code":         {code},
		"redirect_uri": {c.redirectURI},
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, c.tokenEndpoint, strings.NewReader(form.Encode()))
	if err != nil {
		return "", fmt.Errorf("%w: create token request: %v", domain.ErrDiscordUnavailable, err)
	}
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	request.Header.Set("Accept", "application/json")
	request.Header.Set("User-Agent", "KeebHub/0.1")
	request.SetBasicAuth(c.clientID, c.clientSecret)

	response, err := c.httpClient.Do(request)
	if err != nil {
		return "", fmt.Errorf("%w: exchange authorization code: %v", domain.ErrDiscordUnavailable, err)
	}
	defer func() {
		_ = response.Body.Close()
	}()
	if response.StatusCode != http.StatusOK {
		return "", fmt.Errorf("%w: token endpoint returned status %d", domain.ErrDiscordUnavailable, response.StatusCode)
	}

	var payload struct {
		AccessToken string `json:"access_token"`
		TokenType   string `json:"token_type"`
	}
	if err := decodeResponse(response.Body, &payload); err != nil {
		return "", fmt.Errorf("%w: decode token response: %v", domain.ErrDiscordUnavailable, err)
	}
	if payload.AccessToken == "" || !strings.EqualFold(payload.TokenType, "Bearer") {
		return "", fmt.Errorf("%w: token response is incomplete", domain.ErrDiscordUnavailable)
	}
	return payload.AccessToken, nil
}

func (c *Client) currentUser(ctx context.Context, accessToken string) (domain.DiscordIdentity, error) {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, c.userEndpoint, nil)
	if err != nil {
		return domain.DiscordIdentity{}, fmt.Errorf("%w: create current-user request: %v", domain.ErrDiscordUnavailable, err)
	}
	request.Header.Set("Authorization", "Bearer "+accessToken)
	request.Header.Set("Accept", "application/json")
	request.Header.Set("User-Agent", "KeebHub/0.1")

	response, err := c.httpClient.Do(request)
	if err != nil {
		return domain.DiscordIdentity{}, fmt.Errorf("%w: fetch current Discord user: %v", domain.ErrDiscordUnavailable, err)
	}
	defer func() {
		_ = response.Body.Close()
	}()
	if response.StatusCode != http.StatusOK {
		return domain.DiscordIdentity{}, fmt.Errorf("%w: current-user endpoint returned status %d", domain.ErrDiscordUnavailable, response.StatusCode)
	}

	var payload struct {
		ID         string  `json:"id"`
		Username   string  `json:"username"`
		GlobalName *string `json:"global_name"`
		Avatar     *string `json:"avatar"`
	}
	if err := decodeResponse(response.Body, &payload); err != nil {
		return domain.DiscordIdentity{}, fmt.Errorf("%w: decode current-user response: %v", domain.ErrDiscordUnavailable, err)
	}

	displayName := payload.Username
	if payload.GlobalName != nil && strings.TrimSpace(*payload.GlobalName) != "" {
		displayName = *payload.GlobalName
	}
	identity := domain.DiscordIdentity{
		ID:          payload.ID,
		Username:    payload.Username,
		DisplayName: displayName,
		AvatarURL:   avatarURL(payload.ID, payload.Avatar),
	}
	if err := identity.Validate(); err != nil {
		return domain.DiscordIdentity{}, fmt.Errorf("%w: invalid current-user response: %v", domain.ErrDiscordUnavailable, err)
	}
	return identity, nil
}

func avatarURL(userID string, avatarHash *string) *string {
	if avatarHash == nil || *avatarHash == "" || !safeDiscordPathValue(userID) || !safeDiscordPathValue(*avatarHash) {
		return nil
	}
	value := fmt.Sprintf("https://cdn.discordapp.com/avatars/%s/%s.png?size=256", userID, *avatarHash)
	return &value
}

func safeDiscordPathValue(value string) bool {
	if value == "" {
		return false
	}
	for _, character := range value {
		if (character < 'a' || character > 'z') && (character < 'A' || character > 'Z') && (character < '0' || character > '9') && character != '_' {
			return false
		}
	}
	return true
}

func decodeResponse(reader io.Reader, target any) error {
	decoder := json.NewDecoder(io.LimitReader(reader, maximumResponseBytes+1))
	if err := decoder.Decode(target); err != nil {
		return err
	}
	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		if err == nil {
			return fmt.Errorf("response contains multiple JSON values")
		}
		return err
	}
	return nil
}

func valueOrDefault(value, fallback string) string {
	if value == "" {
		return fallback
	}
	return value
}
