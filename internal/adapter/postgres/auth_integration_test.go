package postgres_test

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	postgresadapter "github.com/davidchandra95/keebhub/internal/adapter/postgres"
	"github.com/davidchandra95/keebhub/internal/app"
	"github.com/davidchandra95/keebhub/internal/domain"
	"github.com/davidchandra95/keebhub/internal/testutil/testdatabase"
)

type staticOAuth struct {
	identity domain.DiscordIdentity
}

func (o staticOAuth) AuthorizationURL(state string) (string, error) {
	return "https://discord.example/authorize?state=" + state, nil
}

func (o staticOAuth) IdentityFromCode(context.Context, string) (domain.DiscordIdentity, error) {
	return o.identity, nil
}

func TestAuthStoreLifecycle(t *testing.T) {
	database := testdatabase.Open(t)
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	store := postgresadapter.NewAuthStore(database.Pool)

	firstService := app.NewAuthService(staticOAuth{identity: domain.DiscordIdentity{
		ID: "100000000000000001", Username: "Gunawan.Keyboard", DisplayName: "Gunawan",
	}}, store)
	first, err := firstService.LoginWithDiscord(ctx, "first-code", "")
	if err != nil {
		t.Fatalf("first login: %v", err)
	}
	firstHash, err := app.SessionTokenHash(first.RawToken)
	if err != nil {
		t.Fatalf("hash first token: %v", err)
	}
	var storedHash []byte
	var sessionCount int
	if err := database.Pool.QueryRow(ctx, `SELECT token_hash FROM sessions WHERE user_id = $1`, first.User.ID).Scan(&storedHash); err != nil {
		t.Fatalf("read stored hash: %v", err)
	}
	if string(storedHash) != string(firstHash[:]) || string(storedHash) == first.RawToken {
		t.Error("database did not store only the SHA-256 session hash")
	}
	authenticated, err := firstService.Authenticate(ctx, first.RawToken)
	if err != nil || authenticated.ID != first.User.ID {
		t.Fatalf("authenticate first session: user=%+v error=%v", authenticated, err)
	}

	secondService := app.NewAuthService(staticOAuth{identity: domain.DiscordIdentity{
		ID: "100000000000000001", Username: "renamed.user", DisplayName: "Renamed User",
	}}, store)
	second, err := secondService.LoginWithDiscord(ctx, "second-code", first.RawToken)
	if err != nil {
		t.Fatalf("second login: %v", err)
	}
	if second.User.Handle != "gunawan-keyboard" || second.User.DiscordUsername != "renamed.user" || second.User.DisplayName != "Renamed User" {
		t.Errorf("updated user = %+v", second.User)
	}
	if _, err := secondService.Authenticate(ctx, first.RawToken); !errors.Is(err, domain.ErrInvalidSession) {
		t.Errorf("replaced session error = %v", err)
	}
	if err := database.Pool.QueryRow(ctx, `SELECT count(*) FROM sessions WHERE user_id = $1`, second.User.ID).Scan(&sessionCount); err != nil || sessionCount != 1 {
		t.Fatalf("session count = %d, error = %v", sessionCount, err)
	}

	collisionService := app.NewAuthService(staticOAuth{identity: domain.DiscordIdentity{
		ID: "100000000000000002", Username: "Gunawan.Keyboard", DisplayName: "Other User",
	}}, store)
	collision, err := collisionService.LoginWithDiscord(ctx, "collision-code", "")
	if err != nil {
		t.Fatalf("collision login: %v", err)
	}
	if collision.User.Handle != "gunawan-keyboard-2" {
		t.Errorf("collision handle = %q", collision.User.Handle)
	}

	if _, err := database.Pool.Exec(ctx, `UPDATE users SET status = 'disabled' WHERE id = $1`, second.User.ID); err != nil {
		t.Fatalf("disable user: %v", err)
	}
	if _, err := secondService.Authenticate(ctx, second.RawToken); !errors.Is(err, domain.ErrUserDisabled) {
		t.Errorf("disabled session error = %v", err)
	}
	if _, err := secondService.LoginWithDiscord(ctx, "disabled-code", ""); !errors.Is(err, domain.ErrUserDisabled) {
		t.Errorf("disabled login error = %v", err)
	}

	if err := collisionService.Logout(ctx, collision.RawToken); err != nil {
		t.Fatalf("logout: %v", err)
	}
	if err := collisionService.Logout(ctx, collision.RawToken); err != nil {
		t.Fatalf("repeated logout: %v", err)
	}
	if _, err := collisionService.Authenticate(ctx, collision.RawToken); !errors.Is(err, domain.ErrInvalidSession) {
		t.Errorf("logged-out session error = %v", err)
	}

	invalidHash := []byte("too-short")
	if _, err := database.Pool.Exec(ctx, `INSERT INTO sessions (user_id, token_hash, expires_at) VALUES ($1, $2, now() + interval '1 day')`, collision.User.ID, invalidHash); err == nil {
		t.Error("database accepted a session hash that was not 32 bytes")
	}
}

func TestConcurrentLoginsAllocateUniqueHandles(t *testing.T) {
	database := testdatabase.Open(t)
	store := postgresadapter.NewAuthStore(database.Pool)
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	identities := []domain.DiscordIdentity{
		{ID: "200000000000000001", Username: "same.name", DisplayName: "First"},
		{ID: "200000000000000002", Username: "same.name", DisplayName: "Second"},
	}
	results := make(chan app.LoginResult, len(identities))
	errorsChannel := make(chan error, len(identities))
	var wait sync.WaitGroup
	for _, identity := range identities {
		identity := identity
		wait.Add(1)
		go func() {
			defer wait.Done()
			service := app.NewAuthService(staticOAuth{identity: identity}, store)
			result, err := service.LoginWithDiscord(ctx, "code", "")
			if err != nil {
				errorsChannel <- err
				return
			}
			results <- result
		}()
	}
	wait.Wait()
	close(results)
	close(errorsChannel)
	for err := range errorsChannel {
		t.Fatalf("concurrent login: %v", err)
	}

	handles := map[string]bool{}
	for result := range results {
		handles[result.User.Handle] = true
	}
	if !handles["same-name"] || !handles["same-name-2"] || len(handles) != 2 {
		t.Errorf("handles = %v", handles)
	}
}

func TestExpiredSessionIsRejected(t *testing.T) {
	database := testdatabase.Open(t)
	ctx := context.Background()
	store := postgresadapter.NewAuthStore(database.Pool)
	service := app.NewAuthService(staticOAuth{identity: domain.DiscordIdentity{
		ID: "300000000000000001", Username: "expired.user", DisplayName: "Expired",
	}}, store)
	login, err := service.LoginWithDiscord(ctx, "code", "")
	if err != nil {
		t.Fatalf("login: %v", err)
	}
	if _, err := database.Pool.Exec(ctx, `UPDATE sessions SET created_at = now() - interval '2 hours', expires_at = now() - interval '1 hour' WHERE user_id = $1`, login.User.ID); err != nil {
		t.Fatalf("expire session: %v", err)
	}
	if _, err := service.Authenticate(ctx, login.RawToken); !errors.Is(err, domain.ErrInvalidSession) {
		t.Errorf("expired session error = %v", err)
	}
}
