package postgres

import (
	"context"
	"errors"
	"fmt"

	"github.com/davidchandra95/keebhub/internal/app"
	"github.com/davidchandra95/keebhub/internal/domain"
	generateddb "github.com/davidchandra95/keebhub/internal/generated/db"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// SellerStore is the PostgreSQL implementation of seller profile and catalog reads.
type SellerStore struct {
	pool *pgxpool.Pool
}

func NewSellerStore(pool *pgxpool.Pool) *SellerStore {
	return &SellerStore{pool: pool}
}

func (s *SellerStore) UpdateProfile(ctx context.Context, params app.UpdateProfileParams) (domain.User, error) {
	user, err := generateddb.New(s.pool).UpdateUserProfile(ctx, generateddb.UpdateUserProfileParams{
		UserID: params.UserID, SetLocation: params.SetLocation, Location: params.Location,
		SetBio: params.SetBio, Bio: params.Bio, UpdatedAt: timestamp(params.UpdatedAt),
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.User{}, domain.ErrUserDisabled
	}
	if err != nil {
		return domain.User{}, fmt.Errorf("update user profile: %w", err)
	}
	return mapUser(user), nil
}

func (s *SellerStore) GetSellerProfile(ctx context.Context, handle string) (domain.SellerProfile, error) {
	row, err := generateddb.New(s.pool).GetSellerProfileByHandle(ctx, handle)
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.SellerProfile{}, domain.ErrNotFound
	}
	if err != nil {
		return domain.SellerProfile{}, fmt.Errorf("get seller profile: %w", err)
	}
	return domain.SellerProfile{
		User:      domain.PublicUser{ID: row.ID, Handle: row.Handle, DisplayName: row.DisplayName, AvatarURL: row.AvatarUrl, Location: row.Location, Bio: row.Bio},
		CreatedAt: row.CreatedAt.Time, ActiveListingCount: row.ActiveListingCount,
	}, nil
}

func (s *SellerStore) ListSellerListings(ctx context.Context, query app.SellerListingQuery) ([]domain.Listing, error) {
	rows, err := generateddb.New(s.pool).ListSellerListings(ctx, generateddb.ListSellerListingsParams{
		SellerID: query.SellerID, Statuses: query.Statuses, CategorySlug: query.Category,
		CursorStatusRank: query.CursorStatusRank, CursorUpdatedAt: nullableTimestamp(query.CursorUpdatedAt), CursorID: query.CursorID,
		PageLimit: int32(query.Limit),
	})
	if err != nil {
		return nil, fmt.Errorf("list seller listings: %w", err)
	}
	listings := make([]domain.Listing, 0, len(rows))
	for _, row := range rows {
		listings = append(listings, listingFromValues(
			row.ID, row.SellerID, row.CategoryID, row.Title, row.Description, row.PriceIdr, row.Quantity,
			row.Condition, row.Status, row.ModerationStatus, row.Negotiable, row.CreatedAt, row.UpdatedAt,
			row.CategorySlug, row.CategoryName, row.SellerHandle, row.SellerDisplayName, row.SellerAvatarUrl, row.SellerLocation, row.SellerBio,
		))
	}
	return listings, nil
}
