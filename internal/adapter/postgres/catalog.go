package postgres

import (
	"context"
	"errors"
	"fmt"

	"github.com/davidchandra95/keebhub/internal/app"
	"github.com/davidchandra95/keebhub/internal/domain"
	generateddb "github.com/davidchandra95/keebhub/internal/generated/db"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
)

// CatalogStore is the PostgreSQL implementation of the catalog repositories
// consumed by the application layer.
type CatalogStore struct {
	pool *pgxpool.Pool
}

func NewCatalogStore(pool *pgxpool.Pool) *CatalogStore {
	return &CatalogStore{pool: pool}
}

func (s *CatalogStore) ListActive(ctx context.Context) ([]domain.Category, error) {
	rows, err := generateddb.New(s.pool).ListActiveCategories(ctx)
	if err != nil {
		return nil, fmt.Errorf("list active categories: %w", err)
	}
	categories := make([]domain.Category, 0, len(rows))
	for _, row := range rows {
		categories = append(categories, domain.Category{
			ID:   row.ID,
			Slug: row.Slug,
			Name: row.Name,
		})
	}
	return categories, nil
}

func (s *CatalogStore) FindActiveBySlug(ctx context.Context, slug string) (domain.Category, error) {
	row, err := generateddb.New(s.pool).GetActiveCategoryBySlug(ctx, slug)
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.Category{}, domain.ErrCategoryNotFound
	}
	if err != nil {
		return domain.Category{}, fmt.Errorf("get active category by slug: %w", err)
	}
	return domain.Category{ID: row.ID, Slug: row.Slug, Name: row.Name}, nil
}

func (s *CatalogStore) Create(ctx context.Context, params app.CreateListingParams) (domain.Listing, error) {
	row, err := generateddb.New(s.pool).CreateListing(ctx, generateddb.CreateListingParams{
		SellerID:         params.SellerID,
		CategoryID:       params.CategoryID,
		Title:            params.Title,
		Description:      params.Description,
		PriceIdr:         params.PriceIDR,
		Quantity:         int32(params.Quantity),
		Condition:        string(params.Condition),
		Status:           string(params.Status),
		ModerationStatus: string(params.ModerationStatus),
		Negotiable:       params.Negotiable,
		CreatedAt:        timestamp(params.CreatedAt),
		UpdatedAt:        timestamp(params.UpdatedAt),
	})
	if err != nil {
		return domain.Listing{}, fmt.Errorf("insert listing: %w", err)
	}
	return mapCreateListing(row), nil
}

func (s *CatalogStore) GetByID(ctx context.Context, listingID int64) (domain.Listing, error) {
	row, err := generateddb.New(s.pool).GetListingByID(ctx, listingID)
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.Listing{}, domain.ErrListingNotFound
	}
	if err != nil {
		return domain.Listing{}, fmt.Errorf("get listing by ID: %w", err)
	}
	return mapGetListing(row), nil
}

func (s *CatalogStore) UpdateOwned(ctx context.Context, params app.UpdateOwnedListingParams) (domain.Listing, error) {
	row, err := generateddb.New(s.pool).UpdateOwnedListing(ctx, generateddb.UpdateOwnedListingParams{
		HasTitle:       params.Title != nil,
		Title:          stringValue(params.Title),
		HasDescription: params.Description != nil,
		Description:    stringValue(params.Description),
		HasPriceIdr:    params.PriceIDR != nil,
		PriceIdr:       int64Value(params.PriceIDR),
		HasQuantity:    params.Quantity != nil,
		Quantity:       int32Value(params.Quantity),
		HasCategoryID:  params.CategoryID != nil,
		CategoryID:     int64Value(params.CategoryID),
		HasCondition:   params.Condition != nil,
		Condition:      conditionValue(params.Condition),
		HasNegotiable:  params.Negotiable != nil,
		Negotiable:     boolValue(params.Negotiable),
		UpdatedAt:      timestamp(params.UpdatedAt),
		ListingID:      params.ListingID,
		SellerID:       params.SellerID,
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.Listing{}, domain.ErrListingNotFound
	}
	if err != nil {
		return domain.Listing{}, fmt.Errorf("update owned listing: %w", err)
	}
	return mapUpdateListing(row), nil
}

func (s *CatalogStore) ChangeOwnedStatus(ctx context.Context, params app.ChangeOwnedListingStatusParams) (domain.Listing, error) {
	row, err := generateddb.New(s.pool).UpdateOwnedListingStatus(ctx, generateddb.UpdateOwnedListingStatusParams{
		Status:    string(params.Status),
		UpdatedAt: timestamp(params.UpdatedAt),
		ListingID: params.ListingID,
		SellerID:  params.SellerID,
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.Listing{}, domain.ErrListingNotFound
	}
	if err != nil {
		return domain.Listing{}, fmt.Errorf("update owned listing status: %w", err)
	}
	return mapUpdateListingStatus(row), nil
}

func (s *CatalogStore) ListOwned(ctx context.Context, params app.ListOwnedListingsParams) ([]domain.Listing, error) {
	status := ""
	if params.Status != nil {
		status = string(*params.Status)
	}
	cursorUpdatedAt := pgtype.Timestamptz{}
	cursorID := int64(0)
	if params.Cursor != nil {
		cursorUpdatedAt = timestamp(params.Cursor.UpdatedAt)
		cursorID = params.Cursor.ID
	}
	rows, err := generateddb.New(s.pool).ListOwnedListings(ctx, generateddb.ListOwnedListingsParams{
		SellerID:        params.SellerID,
		HasStatus:       params.Status != nil,
		Status:          status,
		HasCursor:       params.Cursor != nil,
		CursorUpdatedAt: cursorUpdatedAt,
		CursorID:        cursorID,
		PageSize:        int32(params.PageSize),
	})
	if err != nil {
		return nil, fmt.Errorf("list owned listings: %w", err)
	}
	listings := make([]domain.Listing, 0, len(rows))
	for _, row := range rows {
		listings = append(listings, mapListOwnedListing(row))
	}
	return listings, nil
}

func (s *CatalogStore) Search(ctx context.Context, params app.SearchListingsParams) ([]domain.Listing, error) {
	switch params.Sort {
	case app.ListingSortNewest:
		return s.searchNewest(ctx, params)
	case app.ListingSortPriceAsc:
		return s.searchPriceAscending(ctx, params)
	case app.ListingSortPriceDesc:
		return s.searchPriceDescending(ctx, params)
	default:
		return nil, fmt.Errorf("search listings: unsupported sort %q", params.Sort)
	}
}

func (s *CatalogStore) searchNewest(ctx context.Context, params app.SearchListingsParams) ([]domain.Listing, error) {
	cursorCreatedAt := pgtype.Timestamptz{}
	cursorID := int64(0)
	if params.Cursor != nil {
		cursorCreatedAt = timestamp(params.Cursor.CreatedAt)
		cursorID = params.Cursor.ID
	}
	rows, err := generateddb.New(s.pool).SearchListingsNewest(ctx, generateddb.SearchListingsNewestParams{
		HasQuery:        params.Query != nil,
		Query:           stringValue(params.Query),
		HasCategory:     params.Category != nil,
		Category:        stringValue(params.Category),
		HasCondition:    params.Condition != nil,
		Condition:       conditionValue(params.Condition),
		HasMinPrice:     params.MinPrice != nil,
		MinPrice:        int64Value(params.MinPrice),
		HasMaxPrice:     params.MaxPrice != nil,
		MaxPrice:        int64Value(params.MaxPrice),
		HasCursor:       params.Cursor != nil,
		CursorCreatedAt: cursorCreatedAt,
		CursorID:        cursorID,
		PageSize:        int32(params.PageSize),
	})
	if err != nil {
		return nil, fmt.Errorf("search newest listings: %w", err)
	}
	listings := make([]domain.Listing, 0, len(rows))
	for _, row := range rows {
		listings = append(listings, mapSearchNewestListing(row))
	}
	return listings, nil
}

func (s *CatalogStore) searchPriceAscending(ctx context.Context, params app.SearchListingsParams) ([]domain.Listing, error) {
	rows, err := generateddb.New(s.pool).SearchListingsPriceAscending(ctx, generateddb.SearchListingsPriceAscendingParams{
		HasQuery:       params.Query != nil,
		Query:          stringValue(params.Query),
		HasCategory:    params.Category != nil,
		Category:       stringValue(params.Category),
		HasCondition:   params.Condition != nil,
		Condition:      conditionValue(params.Condition),
		HasMinPrice:    params.MinPrice != nil,
		MinPrice:       int64Value(params.MinPrice),
		HasMaxPrice:    params.MaxPrice != nil,
		MaxPrice:       int64Value(params.MaxPrice),
		HasCursor:      params.Cursor != nil,
		CursorPriceIdr: cursorPriceIDR(params.Cursor),
		CursorID:       cursorID(params.Cursor),
		PageSize:       int32(params.PageSize),
	})
	if err != nil {
		return nil, fmt.Errorf("search price ascending listings: %w", err)
	}
	listings := make([]domain.Listing, 0, len(rows))
	for _, row := range rows {
		listings = append(listings, mapSearchPriceAscendingListing(row))
	}
	return listings, nil
}

func (s *CatalogStore) searchPriceDescending(ctx context.Context, params app.SearchListingsParams) ([]domain.Listing, error) {
	rows, err := generateddb.New(s.pool).SearchListingsPriceDescending(ctx, generateddb.SearchListingsPriceDescendingParams{
		HasQuery:       params.Query != nil,
		Query:          stringValue(params.Query),
		HasCategory:    params.Category != nil,
		Category:       stringValue(params.Category),
		HasCondition:   params.Condition != nil,
		Condition:      conditionValue(params.Condition),
		HasMinPrice:    params.MinPrice != nil,
		MinPrice:       int64Value(params.MinPrice),
		HasMaxPrice:    params.MaxPrice != nil,
		MaxPrice:       int64Value(params.MaxPrice),
		HasCursor:      params.Cursor != nil,
		CursorPriceIdr: cursorPriceIDR(params.Cursor),
		CursorID:       cursorID(params.Cursor),
		PageSize:       int32(params.PageSize),
	})
	if err != nil {
		return nil, fmt.Errorf("search price descending listings: %w", err)
	}
	listings := make([]domain.Listing, 0, len(rows))
	for _, row := range rows {
		listings = append(listings, mapSearchPriceDescendingListing(row))
	}
	return listings, nil
}

func stringValue(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}

func int64Value(value *int64) int64 {
	if value == nil {
		return 0
	}
	return *value
}

func int32Value(value *int) int32 {
	if value == nil {
		return 0
	}
	return int32(*value)
}

func boolValue(value *bool) bool {
	return value != nil && *value
}

func conditionValue(value *domain.ListingCondition) string {
	if value == nil {
		return ""
	}
	return string(*value)
}

func cursorPriceIDR(cursor *app.PublicListingCursor) int64 {
	if cursor == nil {
		return 0
	}
	return cursor.PriceIDR
}

func cursorID(cursor *app.PublicListingCursor) int64 {
	if cursor == nil {
		return 0
	}
	return cursor.ID
}

func mapCreateListing(row generateddb.CreateListingRow) domain.Listing {
	return mapCatalogListing(
		row.ListingID, row.ListingSellerID, row.ListingCategoryID,
		row.Title, row.Description, row.PriceIdr, row.Quantity,
		row.Condition, row.Status, row.ModerationStatus, row.Negotiable,
		row.CreatedAt, row.UpdatedAt,
		row.CategoryID, row.CategorySlug, row.CategoryName,
		row.SellerID, row.SellerHandle, row.SellerDisplayName, row.SellerAvatarUrl, row.SellerLocation,
	)
}

func mapGetListing(row generateddb.GetListingByIDRow) domain.Listing {
	return mapCatalogListing(
		row.ListingID, row.ListingSellerID, row.ListingCategoryID,
		row.Title, row.Description, row.PriceIdr, row.Quantity,
		row.Condition, row.Status, row.ModerationStatus, row.Negotiable,
		row.CreatedAt, row.UpdatedAt,
		row.CategoryID, row.CategorySlug, row.CategoryName,
		row.SellerID, row.SellerHandle, row.SellerDisplayName, row.SellerAvatarUrl, row.SellerLocation,
	)
}

func mapUpdateListing(row generateddb.UpdateOwnedListingRow) domain.Listing {
	return mapCatalogListing(
		row.ListingID, row.ListingSellerID, row.ListingCategoryID,
		row.Title, row.Description, row.PriceIdr, row.Quantity,
		row.Condition, row.Status, row.ModerationStatus, row.Negotiable,
		row.CreatedAt, row.UpdatedAt,
		row.CategoryID, row.CategorySlug, row.CategoryName,
		row.SellerID, row.SellerHandle, row.SellerDisplayName, row.SellerAvatarUrl, row.SellerLocation,
	)
}

func mapUpdateListingStatus(row generateddb.UpdateOwnedListingStatusRow) domain.Listing {
	return mapCatalogListing(
		row.ListingID, row.ListingSellerID, row.ListingCategoryID,
		row.Title, row.Description, row.PriceIdr, row.Quantity,
		row.Condition, row.Status, row.ModerationStatus, row.Negotiable,
		row.CreatedAt, row.UpdatedAt,
		row.CategoryID, row.CategorySlug, row.CategoryName,
		row.SellerID, row.SellerHandle, row.SellerDisplayName, row.SellerAvatarUrl, row.SellerLocation,
	)
}

func mapListOwnedListing(row generateddb.ListOwnedListingsRow) domain.Listing {
	return mapCatalogListing(
		row.ListingID, row.ListingSellerID, row.ListingCategoryID,
		row.Title, row.Description, row.PriceIdr, row.Quantity,
		row.Condition, row.Status, row.ModerationStatus, row.Negotiable,
		row.CreatedAt, row.UpdatedAt,
		row.CategoryID, row.CategorySlug, row.CategoryName,
		row.SellerID, row.SellerHandle, row.SellerDisplayName, row.SellerAvatarUrl, row.SellerLocation,
	)
}

func mapSearchNewestListing(row generateddb.SearchListingsNewestRow) domain.Listing {
	return mapCatalogListing(
		row.ListingID, row.ListingSellerID, row.ListingCategoryID,
		row.Title, row.Description, row.PriceIdr, row.Quantity,
		row.Condition, row.Status, row.ModerationStatus, row.Negotiable,
		row.CreatedAt, row.UpdatedAt,
		row.CategoryID, row.CategorySlug, row.CategoryName,
		row.SellerID, row.SellerHandle, row.SellerDisplayName, row.SellerAvatarUrl, row.SellerLocation,
	)
}

func mapSearchPriceAscendingListing(row generateddb.SearchListingsPriceAscendingRow) domain.Listing {
	return mapCatalogListing(
		row.ListingID, row.ListingSellerID, row.ListingCategoryID,
		row.Title, row.Description, row.PriceIdr, row.Quantity,
		row.Condition, row.Status, row.ModerationStatus, row.Negotiable,
		row.CreatedAt, row.UpdatedAt,
		row.CategoryID, row.CategorySlug, row.CategoryName,
		row.SellerID, row.SellerHandle, row.SellerDisplayName, row.SellerAvatarUrl, row.SellerLocation,
	)
}

func mapSearchPriceDescendingListing(row generateddb.SearchListingsPriceDescendingRow) domain.Listing {
	return mapCatalogListing(
		row.ListingID, row.ListingSellerID, row.ListingCategoryID,
		row.Title, row.Description, row.PriceIdr, row.Quantity,
		row.Condition, row.Status, row.ModerationStatus, row.Negotiable,
		row.CreatedAt, row.UpdatedAt,
		row.CategoryID, row.CategorySlug, row.CategoryName,
		row.SellerID, row.SellerHandle, row.SellerDisplayName, row.SellerAvatarUrl, row.SellerLocation,
	)
}

func mapCatalogListing(
	listingID, listingSellerID, listingCategoryID int64,
	title, description string,
	priceIDR int64,
	quantity int32,
	condition, status, moderationStatus string,
	negotiable bool,
	createdAt, updatedAt pgtype.Timestamptz,
	categoryID int64,
	categorySlug, categoryName string,
	sellerID int64,
	sellerHandle, sellerDisplayName string,
	sellerAvatarURL, sellerLocation *string,
) domain.Listing {
	return domain.Listing{
		ID:               listingID,
		SellerID:         listingSellerID,
		CategoryID:       listingCategoryID,
		Title:            title,
		Description:      description,
		PriceIDR:         priceIDR,
		Quantity:         int(quantity),
		Category:         domain.Category{ID: categoryID, Slug: categorySlug, Name: categoryName},
		Condition:        domain.ListingCondition(condition),
		Status:           domain.ListingStatus(status),
		ModerationStatus: domain.ModerationStatus(moderationStatus),
		Negotiable:       negotiable,
		Seller: domain.PublicUser{
			ID:          sellerID,
			Handle:      sellerHandle,
			DisplayName: sellerDisplayName,
			AvatarURL:   sellerAvatarURL,
			Location:    sellerLocation,
		},
		CreatedAt: createdAt.Time.UTC(),
		UpdatedAt: updatedAt.Time.UTC(),
	}
}
