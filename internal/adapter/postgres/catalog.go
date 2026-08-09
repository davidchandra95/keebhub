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
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
)

// CatalogStore is the PostgreSQL implementation of the catalog repositories.
type CatalogStore struct {
	pool *pgxpool.Pool
}

func NewCatalogStore(pool *pgxpool.Pool) *CatalogStore {
	return &CatalogStore{pool: pool}
}

func (s *CatalogStore) ListActiveCategories(ctx context.Context) ([]domain.Category, error) {
	rows, err := generateddb.New(s.pool).ListActiveCategories(ctx)
	if err != nil {
		return nil, fmt.Errorf("list active categories: %w", err)
	}
	result := make([]domain.Category, 0, len(rows))
	for _, row := range rows {
		result = append(result, mapCategory(row))
	}
	return result, nil
}

func (s *CatalogStore) FindActiveCategoryBySlug(ctx context.Context, slug string) (domain.Category, error) {
	row, err := generateddb.New(s.pool).GetActiveCategoryBySlug(ctx, slug)
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.Category{}, domain.ErrNotFound
	}
	if err != nil {
		return domain.Category{}, fmt.Errorf("get active category by slug: %w", err)
	}
	return mapCategory(row), nil
}

func (s *CatalogStore) CreateListing(ctx context.Context, params app.CreateListingParams) (domain.Listing, error) {
	row, err := generateddb.New(s.pool).CreateListing(ctx, generateddb.CreateListingParams{
		SellerID: params.SellerID, CategoryID: params.CategoryID, Title: params.Title,
		Description: params.Description, PriceIdr: params.PriceIDR, Quantity: params.Quantity,
		Condition: string(params.Condition), Negotiable: params.Negotiable,
		CreatedAt: timestamp(params.CreatedAt), UpdatedAt: timestamp(params.UpdatedAt),
	})
	if err != nil {
		return domain.Listing{}, fmt.Errorf("create listing: %w", err)
	}
	return mapCreatedListing(row), nil
}

func (s *CatalogStore) GetListing(ctx context.Context, listingID int64) (domain.Listing, error) {
	row, err := generateddb.New(s.pool).GetListingByID(ctx, listingID)
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.Listing{}, domain.ErrNotFound
	}
	if err != nil {
		return domain.Listing{}, fmt.Errorf("get listing: %w", err)
	}
	return mapListingByID(row), nil
}

func (s *CatalogStore) UpdateOwnedListing(ctx context.Context, params app.UpdateListingParams) (domain.Listing, error) {
	row, err := generateddb.New(s.pool).UpdateOwnedListing(ctx, generateddb.UpdateOwnedListingParams{
		SetCategoryID:  params.CategoryID != nil,
		CategoryID:     valueOrZero(params.CategoryID),
		SetTitle:       params.Title != nil,
		Title:          valueOrZero(params.Title),
		SetDescription: params.Description != nil,
		Description:    valueOrZero(params.Description),
		SetPriceIdr:    params.PriceIDR != nil,
		PriceIdr:       valueOrZero(params.PriceIDR),
		SetQuantity:    params.Quantity != nil,
		Quantity:       valueOrZero(params.Quantity),
		SetCondition:   params.Condition != nil,
		Condition:      string(valueOrZero(params.Condition)),
		SetNegotiable:  params.Negotiable != nil,
		Negotiable:     valueOrZero(params.Negotiable),
		UpdatedAt:      timestamp(params.UpdatedAt),
		ListingID:      params.ListingID,
		SellerID:       params.SellerID,
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.Listing{}, domain.ErrNotFound
	}
	if err != nil {
		return domain.Listing{}, fmt.Errorf("update owned listing: %w", err)
	}
	return mapUpdatedListing(row), nil
}

func (s *CatalogStore) UpdateOwnedListingStatus(ctx context.Context, listingID, sellerID int64, status domain.ListingStatus, updatedAt time.Time) (domain.Listing, error) {
	row, err := generateddb.New(s.pool).UpdateOwnedListingStatus(ctx, generateddb.UpdateOwnedListingStatusParams{
		ListingID: listingID, SellerID: sellerID, Status: string(status), UpdatedAt: timestamp(updatedAt),
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.Listing{}, domain.ErrNotFound
	}
	if err != nil {
		return domain.Listing{}, fmt.Errorf("update owned listing status: %w", err)
	}
	return mapStatusUpdatedListing(row), nil
}

func (s *CatalogStore) ListOwnedListings(ctx context.Context, params app.OwnedListingQuery) ([]domain.Listing, error) {
	var status *string
	if params.Status != nil {
		value := string(*params.Status)
		status = &value
	}
	rows, err := generateddb.New(s.pool).ListOwnedListings(ctx, generateddb.ListOwnedListingsParams{
		SellerID: params.SellerID, StatusFilter: status, CursorUpdatedAt: nullableTimestamp(params.CursorUpdatedAt),
		CursorID: params.CursorID, PageLimit: int32(params.Limit),
	})
	if err != nil {
		return nil, fmt.Errorf("list owned listings: %w", err)
	}
	result := make([]domain.Listing, 0, len(rows))
	for _, row := range rows {
		result = append(result, mapOwnedListing(row))
	}
	return result, nil
}

func (s *CatalogStore) SearchListings(ctx context.Context, params app.SearchListingsQuery) ([]domain.Listing, error) {
	var condition *string
	if params.Condition != nil {
		value := string(*params.Condition)
		condition = &value
	}
	queries := generateddb.New(s.pool)
	switch params.Sort {
	case app.ListingSortNewest:
		rows, err := queries.SearchListingsNewest(ctx, generateddb.SearchListingsNewestParams{
			QueryText: params.Query, CategorySlug: params.Category, ConditionFilter: condition,
			MinPrice: params.MinimumPrice, MaxPrice: params.MaximumPrice,
			CursorCreatedAt: nullableTimestamp(params.CursorCreatedAt), CursorID: params.CursorID, PageLimit: int32(params.Limit),
		})
		if err != nil {
			return nil, fmt.Errorf("search newest listings: %w", err)
		}
		result := make([]domain.Listing, 0, len(rows))
		for _, row := range rows {
			result = append(result, mapNewestListing(row))
		}
		return result, nil
	case app.ListingSortPriceAsc:
		rows, err := queries.SearchListingsPriceAscending(ctx, generateddb.SearchListingsPriceAscendingParams{
			QueryText: params.Query, CategorySlug: params.Category, ConditionFilter: condition,
			MinPrice: params.MinimumPrice, MaxPrice: params.MaximumPrice,
			CursorPriceIdr: params.CursorPriceIDR, CursorID: params.CursorID, PageLimit: int32(params.Limit),
		})
		if err != nil {
			return nil, fmt.Errorf("search price ascending listings: %w", err)
		}
		result := make([]domain.Listing, 0, len(rows))
		for _, row := range rows {
			result = append(result, mapPriceAscendingListing(row))
		}
		return result, nil
	case app.ListingSortPriceDesc:
		rows, err := queries.SearchListingsPriceDescending(ctx, generateddb.SearchListingsPriceDescendingParams{
			QueryText: params.Query, CategorySlug: params.Category, ConditionFilter: condition,
			MinPrice: params.MinimumPrice, MaxPrice: params.MaximumPrice,
			CursorPriceIdr: params.CursorPriceIDR, CursorID: params.CursorID, PageLimit: int32(params.Limit),
		})
		if err != nil {
			return nil, fmt.Errorf("search price descending listings: %w", err)
		}
		result := make([]domain.Listing, 0, len(rows))
		for _, row := range rows {
			result = append(result, mapPriceDescendingListing(row))
		}
		return result, nil
	default:
		return nil, fmt.Errorf("unsupported listing sort %q", params.Sort)
	}
}

func mapCategory(row generateddb.Category) domain.Category {
	return domain.Category{ID: row.ID, Slug: row.Slug, Name: row.Name, SortOrder: row.SortOrder, Active: row.Active}
}

func mapCreatedListing(row generateddb.CreateListingRow) domain.Listing {
	return listingFromValues(row.ID, row.SellerID, row.CategoryID, row.Title, row.Description, row.PriceIdr, row.Quantity, row.Condition, row.Status, row.ModerationStatus, row.Negotiable, row.CreatedAt, row.UpdatedAt, row.CategorySlug, row.CategoryName, row.SellerHandle, row.SellerDisplayName, row.SellerAvatarUrl, row.SellerLocation, row.SellerBio)
}

func mapListingByID(row generateddb.GetListingByIDRow) domain.Listing {
	return listingFromValues(row.ID, row.SellerID, row.CategoryID, row.Title, row.Description, row.PriceIdr, row.Quantity, row.Condition, row.Status, row.ModerationStatus, row.Negotiable, row.CreatedAt, row.UpdatedAt, row.CategorySlug, row.CategoryName, row.SellerHandle, row.SellerDisplayName, row.SellerAvatarUrl, row.SellerLocation, row.SellerBio)
}

func mapUpdatedListing(row generateddb.UpdateOwnedListingRow) domain.Listing {
	return listingFromValues(row.ID, row.SellerID, row.CategoryID, row.Title, row.Description, row.PriceIdr, row.Quantity, row.Condition, row.Status, row.ModerationStatus, row.Negotiable, row.CreatedAt, row.UpdatedAt, row.CategorySlug, row.CategoryName, row.SellerHandle, row.SellerDisplayName, row.SellerAvatarUrl, row.SellerLocation, row.SellerBio)
}

func mapStatusUpdatedListing(row generateddb.UpdateOwnedListingStatusRow) domain.Listing {
	return listingFromValues(row.ID, row.SellerID, row.CategoryID, row.Title, row.Description, row.PriceIdr, row.Quantity, row.Condition, row.Status, row.ModerationStatus, row.Negotiable, row.CreatedAt, row.UpdatedAt, row.CategorySlug, row.CategoryName, row.SellerHandle, row.SellerDisplayName, row.SellerAvatarUrl, row.SellerLocation, row.SellerBio)
}

func mapOwnedListing(row generateddb.ListOwnedListingsRow) domain.Listing {
	return listingFromValues(row.ID, row.SellerID, row.CategoryID, row.Title, row.Description, row.PriceIdr, row.Quantity, row.Condition, row.Status, row.ModerationStatus, row.Negotiable, row.CreatedAt, row.UpdatedAt, row.CategorySlug, row.CategoryName, row.SellerHandle, row.SellerDisplayName, row.SellerAvatarUrl, row.SellerLocation, row.SellerBio)
}

func mapNewestListing(row generateddb.SearchListingsNewestRow) domain.Listing {
	return listingFromValues(row.ID, row.SellerID, row.CategoryID, row.Title, row.Description, row.PriceIdr, row.Quantity, row.Condition, row.Status, row.ModerationStatus, row.Negotiable, row.CreatedAt, row.UpdatedAt, row.CategorySlug, row.CategoryName, row.SellerHandle, row.SellerDisplayName, row.SellerAvatarUrl, row.SellerLocation, row.SellerBio)
}

func mapPriceAscendingListing(row generateddb.SearchListingsPriceAscendingRow) domain.Listing {
	return listingFromValues(row.ID, row.SellerID, row.CategoryID, row.Title, row.Description, row.PriceIdr, row.Quantity, row.Condition, row.Status, row.ModerationStatus, row.Negotiable, row.CreatedAt, row.UpdatedAt, row.CategorySlug, row.CategoryName, row.SellerHandle, row.SellerDisplayName, row.SellerAvatarUrl, row.SellerLocation, row.SellerBio)
}

func mapPriceDescendingListing(row generateddb.SearchListingsPriceDescendingRow) domain.Listing {
	return listingFromValues(row.ID, row.SellerID, row.CategoryID, row.Title, row.Description, row.PriceIdr, row.Quantity, row.Condition, row.Status, row.ModerationStatus, row.Negotiable, row.CreatedAt, row.UpdatedAt, row.CategorySlug, row.CategoryName, row.SellerHandle, row.SellerDisplayName, row.SellerAvatarUrl, row.SellerLocation, row.SellerBio)
}

func listingFromValues(id, sellerID, categoryID int64, title, description string, priceIDR int64, quantity int32, condition, status, moderationStatus string, negotiable bool, createdAt, updatedAt pgtype.Timestamptz, categorySlug, categoryName, sellerHandle, sellerDisplayName string, sellerAvatarURL, sellerLocation, sellerBio *string) domain.Listing {
	return domain.Listing{
		ID: id, SellerID: sellerID, CategoryID: categoryID, Title: title, Description: description,
		PriceIDR: priceIDR, Quantity: quantity, Condition: domain.ListingCondition(condition),
		Status: domain.ListingStatus(status), ModerationStatus: domain.ModerationStatus(moderationStatus),
		Negotiable: negotiable, CreatedAt: createdAt.Time, UpdatedAt: updatedAt.Time,
		Category: domain.Category{ID: categoryID, Slug: categorySlug, Name: categoryName},
		Seller:   domain.PublicUser{ID: sellerID, Handle: sellerHandle, DisplayName: sellerDisplayName, AvatarURL: sellerAvatarURL, Location: sellerLocation, Bio: sellerBio},
	}
}

func nullableTimestamp(value *time.Time) pgtype.Timestamptz {
	if value == nil {
		return pgtype.Timestamptz{}
	}
	return timestamp(*value)
}

func valueOrZero[T any](value *T) T {
	var zero T
	if value == nil {
		return zero
	}
	return *value
}
