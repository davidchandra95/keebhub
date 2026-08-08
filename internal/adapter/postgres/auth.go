package postgres

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/davidchandra95/keebhub/internal/app"
	"github.com/davidchandra95/keebhub/internal/domain"
	generateddb "github.com/davidchandra95/keebhub/internal/generated/db"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
)

const maximumHandleAttempts = 1000

type AuthStore struct {
	pool *pgxpool.Pool
}

func NewAuthStore(pool *pgxpool.Pool) *AuthStore {
	return &AuthStore{pool: pool}
}

func (s *AuthStore) Login(ctx context.Context, params app.LoginParams) (domain.User, error) {
	for attempt := 1; attempt <= maximumHandleAttempts; attempt++ {
		user, err := s.loginAttempt(ctx, params, domain.HandleCandidate(params.HandleBase, attempt))
		if err == nil {
			return user, nil
		}
		if isRetryableUserConflict(err) {
			continue
		}
		return domain.User{}, err
	}
	return domain.User{}, fmt.Errorf("allocate unique handle after %d attempts", maximumHandleAttempts)
}

func (s *AuthStore) loginAttempt(ctx context.Context, params app.LoginParams, handle string) (domain.User, error) {
	tx, err := s.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return domain.User{}, fmt.Errorf("begin authentication transaction: %w", err)
	}
	defer func() {
		_ = tx.Rollback(ctx)
	}()

	queries := generateddb.New(tx)
	databaseUser, err := queries.GetUserByDiscordID(ctx, params.Identity.ID)
	switch {
	case err == nil:
		if databaseUser.Status == domain.UserStatusDisabled {
			return domain.User{}, domain.ErrUserDisabled
		}
		databaseUser, err = queries.UpdateDiscordIdentity(ctx, generateddb.UpdateDiscordIdentityParams{
			ID:              databaseUser.ID,
			DiscordUsername: params.Identity.Username,
			DisplayName:     params.Identity.DisplayName,
			AvatarUrl:       params.Identity.AvatarURL,
			UpdatedAt:       timestamp(params.CreatedAt),
		})
		if err != nil {
			return domain.User{}, fmt.Errorf("update Discord identity: %w", err)
		}
	case errors.Is(err, pgx.ErrNoRows):
		databaseUser, err = queries.CreateUser(ctx, generateddb.CreateUserParams{
			DiscordID:       params.Identity.ID,
			DiscordUsername: params.Identity.Username,
			DisplayName:     params.Identity.DisplayName,
			AvatarUrl:       params.Identity.AvatarURL,
			Handle:          handle,
		})
		if err != nil {
			return domain.User{}, fmt.Errorf("create user: %w", err)
		}
	default:
		return domain.User{}, fmt.Errorf("find Discord user: %w", err)
	}

	if params.PreviousTokenHash != nil {
		if _, err := queries.DeleteSessionByHash(ctx, params.PreviousTokenHash[:]); err != nil {
			return domain.User{}, fmt.Errorf("replace previous session: %w", err)
		}
	}
	if err := queries.CreateSession(ctx, generateddb.CreateSessionParams{
		UserID:    databaseUser.ID,
		TokenHash: params.TokenHash[:],
		ExpiresAt: timestamp(params.ExpiresAt),
		CreatedAt: timestamp(params.CreatedAt),
	}); err != nil {
		return domain.User{}, fmt.Errorf("create session: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return domain.User{}, fmt.Errorf("commit authentication transaction: %w", err)
	}
	return mapUser(databaseUser), nil
}

func (s *AuthStore) Authenticate(ctx context.Context, tokenHash [32]byte, now time.Time) (domain.User, error) {
	databaseUser, err := generateddb.New(s.pool).TouchAndGetSessionUser(ctx, generateddb.TouchAndGetSessionUserParams{
		TokenHash:  tokenHash[:],
		LastSeenAt: timestamp(now),
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.User{}, domain.ErrInvalidSession
	}
	if err != nil {
		return domain.User{}, fmt.Errorf("authenticate session: %w", err)
	}
	if databaseUser.Status == domain.UserStatusDisabled {
		return domain.User{}, domain.ErrUserDisabled
	}
	return mapUser(databaseUser), nil
}

func (s *AuthStore) DeleteSession(ctx context.Context, tokenHash [32]byte) error {
	if _, err := generateddb.New(s.pool).DeleteSessionByHash(ctx, tokenHash[:]); err != nil {
		return fmt.Errorf("delete session: %w", err)
	}
	return nil
}

func mapUser(user generateddb.User) domain.User {
	return domain.User{
		ID:              user.ID,
		DiscordID:       user.DiscordID,
		DiscordUsername: user.DiscordUsername,
		DisplayName:     user.DisplayName,
		AvatarURL:       user.AvatarUrl,
		Handle:          user.Handle,
		Location:        user.Location,
		Bio:             user.Bio,
		Status:          user.Status,
		CreatedAt:       user.CreatedAt.Time,
		UpdatedAt:       user.UpdatedAt.Time,
	}
}

func timestamp(value time.Time) pgtype.Timestamptz {
	return pgtype.Timestamptz{Time: value, Valid: true}
}

func isRetryableUserConflict(err error) bool {
	var postgresError *pgconn.PgError
	if !errors.As(err, &postgresError) || postgresError.Code != "23505" {
		return false
	}
	return postgresError.ConstraintName == "users_handle_lower_key" || postgresError.ConstraintName == "users_discord_id_key"
}
