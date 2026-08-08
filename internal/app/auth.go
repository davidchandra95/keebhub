package app

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"time"

	"github.com/davidchandra95/keebhub/internal/domain"
)

const (
	sessionTokenBytes = 32
	sessionLifetime   = 30 * 24 * time.Hour
)

type DiscordOAuth interface {
	AuthorizationURL(state string) (string, error)
	IdentityFromCode(ctx context.Context, code string) (domain.DiscordIdentity, error)
}

type AuthStore interface {
	Login(ctx context.Context, params LoginParams) (domain.User, error)
	Authenticate(ctx context.Context, tokenHash [sha256.Size]byte, now time.Time) (domain.User, error)
	DeleteSession(ctx context.Context, tokenHash [sha256.Size]byte) error
}

type LoginParams struct {
	Identity          domain.DiscordIdentity
	HandleBase        string
	TokenHash         [sha256.Size]byte
	PreviousTokenHash *[sha256.Size]byte
	CreatedAt         time.Time
	ExpiresAt         time.Time
}

type LoginResult struct {
	User      domain.User
	RawToken  string
	ExpiresAt time.Time
}

type AuthService struct {
	oauth  DiscordOAuth
	store  AuthStore
	random io.Reader
	now    func() time.Time
}

func NewAuthService(oauth DiscordOAuth, store AuthStore) *AuthService {
	return &AuthService{
		oauth:  oauth,
		store:  store,
		random: rand.Reader,
		now:    time.Now,
	}
}

func (s *AuthService) AuthorizationURL(state string) (string, error) {
	if s.oauth == nil {
		return "", domain.ErrAuthenticationUnavailable
	}
	return s.oauth.AuthorizationURL(state)
}

func (s *AuthService) LoginWithDiscord(ctx context.Context, code, previousRawToken string) (LoginResult, error) {
	if s.oauth == nil {
		return LoginResult{}, domain.ErrAuthenticationUnavailable
	}
	if s.store == nil {
		return LoginResult{}, fmt.Errorf("auth store is not configured")
	}

	identity, err := s.oauth.IdentityFromCode(ctx, code)
	if err != nil {
		return LoginResult{}, err
	}
	if err := identity.Validate(); err != nil {
		return LoginResult{}, fmt.Errorf("validate Discord identity: %w", err)
	}

	rawToken, tokenHash, err := s.newSessionToken()
	if err != nil {
		return LoginResult{}, fmt.Errorf("generate session token: %w", err)
	}

	var previousHash *[sha256.Size]byte
	if previousRawToken != "" {
		if parsed, parseErr := SessionTokenHash(previousRawToken); parseErr == nil {
			previousHash = &parsed
		}
	}

	now := s.now().UTC()
	expiresAt := now.Add(sessionLifetime)
	user, err := s.store.Login(ctx, LoginParams{
		Identity:          identity,
		HandleBase:        domain.NormalizeHandle(identity.Username),
		TokenHash:         tokenHash,
		PreviousTokenHash: previousHash,
		CreatedAt:         now,
		ExpiresAt:         expiresAt,
	})
	if err != nil {
		return LoginResult{}, err
	}

	return LoginResult{User: user, RawToken: rawToken, ExpiresAt: expiresAt}, nil
}

func (s *AuthService) Authenticate(ctx context.Context, rawToken string) (domain.User, error) {
	if s.store == nil {
		return domain.User{}, fmt.Errorf("auth store is not configured")
	}
	hash, err := SessionTokenHash(rawToken)
	if err != nil {
		return domain.User{}, err
	}
	return s.store.Authenticate(ctx, hash, s.now().UTC())
}

func (s *AuthService) Logout(ctx context.Context, rawToken string) error {
	if rawToken == "" || s.store == nil {
		return nil
	}
	hash, err := SessionTokenHash(rawToken)
	if err != nil {
		if errors.Is(err, domain.ErrInvalidSession) {
			return nil
		}
		return err
	}
	return s.store.DeleteSession(ctx, hash)
}

func SessionTokenHash(rawToken string) ([sha256.Size]byte, error) {
	decoded, err := base64.RawURLEncoding.DecodeString(rawToken)
	if err != nil || len(decoded) != sessionTokenBytes {
		return [sha256.Size]byte{}, domain.ErrInvalidSession
	}
	return sha256.Sum256(decoded), nil
}

func (s *AuthService) newSessionToken() (string, [sha256.Size]byte, error) {
	random := s.random
	if random == nil {
		random = rand.Reader
	}
	value := make([]byte, sessionTokenBytes)
	if _, err := io.ReadFull(random, value); err != nil {
		return "", [sha256.Size]byte{}, err
	}
	return base64.RawURLEncoding.EncodeToString(value), sha256.Sum256(value), nil
}
