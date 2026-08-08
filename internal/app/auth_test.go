package app

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"testing"
	"time"

	"github.com/davidchandra95/keebhub/internal/domain"
)

type fakeDiscordOAuth struct {
	identity domain.DiscordIdentity
	err      error
}

func (f fakeDiscordOAuth) AuthorizationURL(state string) (string, error) {
	return "https://discord.example/authorize?state=" + state, f.err
}

func (f fakeDiscordOAuth) IdentityFromCode(context.Context, string) (domain.DiscordIdentity, error) {
	return f.identity, f.err
}

type recordingAuthStore struct {
	loginParams LoginParams
	user        domain.User
	authUser    domain.User
	authErr     error
	deletedHash [sha256.Size]byte
}

func (s *recordingAuthStore) Login(_ context.Context, params LoginParams) (domain.User, error) {
	s.loginParams = params
	return s.user, nil
}

func (s *recordingAuthStore) Authenticate(_ context.Context, hash [sha256.Size]byte, _ time.Time) (domain.User, error) {
	s.deletedHash = hash
	return s.authUser, s.authErr
}

func (s *recordingAuthStore) DeleteSession(_ context.Context, hash [sha256.Size]byte) error {
	s.deletedHash = hash
	return nil
}

func TestLoginWithDiscordCreatesOpaqueSessionAndReplacesPreviousToken(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 8, 8, 12, 0, 0, 0, time.UTC)
	store := &recordingAuthStore{user: domain.User{ID: 42, Handle: "gunawan-keyboard"}}
	service := NewAuthService(fakeDiscordOAuth{identity: domain.DiscordIdentity{
		ID: "123456789", Username: "Gunawan.Keyboard", DisplayName: "Gunawan",
	}}, store)
	service.random = bytes.NewReader(bytes.Repeat([]byte{0x2a}, sessionTokenBytes))
	service.now = func() time.Time { return now }
	previous := base64.RawURLEncoding.EncodeToString(bytes.Repeat([]byte{0x11}, sessionTokenBytes))

	result, err := service.LoginWithDiscord(context.Background(), "oauth-code", previous)
	if err != nil {
		t.Fatalf("LoginWithDiscord() error = %v", err)
	}
	decoded, err := base64.RawURLEncoding.DecodeString(result.RawToken)
	if err != nil || len(decoded) != sessionTokenBytes {
		t.Fatalf("raw token decoded length = %d, error = %v", len(decoded), err)
	}
	if store.loginParams.TokenHash != sha256.Sum256(decoded) {
		t.Error("store did not receive the SHA-256 token hash")
	}
	wantPrevious := sha256.Sum256(bytes.Repeat([]byte{0x11}, sessionTokenBytes))
	if store.loginParams.PreviousTokenHash == nil || *store.loginParams.PreviousTokenHash != wantPrevious {
		t.Error("previous browser session hash was not passed for replacement")
	}
	if store.loginParams.HandleBase != "gunawan-keyboard" {
		t.Errorf("handle base = %q", store.loginParams.HandleBase)
	}
	if !result.ExpiresAt.Equal(now.Add(30 * 24 * time.Hour)) {
		t.Errorf("expiry = %s", result.ExpiresAt)
	}
}

func TestSessionTokenHashRejectsMalformedTokens(t *testing.T) {
	t.Parallel()

	for _, raw := range []string{"", "not-base64!", base64.RawURLEncoding.EncodeToString(make([]byte, 31)), base64.RawURLEncoding.EncodeToString(make([]byte, 33))} {
		if _, err := SessionTokenHash(raw); !errors.Is(err, domain.ErrInvalidSession) {
			t.Errorf("SessionTokenHash(%q) error = %v", raw, err)
		}
	}
}

func TestLogoutIgnoresMalformedToken(t *testing.T) {
	t.Parallel()

	store := &recordingAuthStore{}
	service := NewAuthService(nil, store)
	if err := service.Logout(context.Background(), "malformed"); err != nil {
		t.Fatalf("Logout() error = %v", err)
	}
	if store.deletedHash != ([sha256.Size]byte{}) {
		t.Error("malformed token reached the store")
	}
}
