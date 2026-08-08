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

type CatalogStore struct {
	pool *pgxpool.Pool
}

func NewCatalogStore(pool *pgxpool.Pool) *CatalogStore {
	return &CatalogStore{pool: pool}
}

func (s *CatalogStore) ListActiveCategories(ctx context.Context) ([]domain.Category, error) {
	items, err := generateddb.New(s.pool).ListActiveCategories(ctx)
	if err != nil {
		return nil, fmt.Errorf("list active categories: %w", err)
	}
	categories := make([]domain.Category, 0, len(items))
	for _, item := range items {
		categories = append(categories, mapCategory(item))
	}
	return categories, nil
}

func (s *CatalogStore) GetActiveCategoryBySlug(ctx context.Context, slug string) (domain.Category, error) {
	item, err := generateddb.New(s.pool).GetActiveCategoryBySlug(ctx, slug)
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.Category{}, domain.ErrNotFound
	}
	if err != nil {
		return domain.Category{}, fmt.Errorf("get active category by slug: %w", err)
	}
	return mapCategory(item), nil
}

func (s *CatalogStore) CreateListing(ctx context.Context, params app.ListingCreateParams) (domain.Listing, error) {
	tx, err := s.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return domain.Listing{}, fmt.Errorf("begin create listing transaction: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	queries := generateddb.New(tx)
	id, err := queries.CreateListing(ctx, generateddb.CreateListingParams{
		SellerID:    params.SellerID,
		CategoryID:  params.CategoryID,
		Title:       params.Title,
		Description: params.Description,
		PriceIdr:    params.PriceIDR,
		Quantity:    params.Quantity,
		Condition:   string(params.Condition),
		Negotiable:  params.Negotiable,
		CreatedAt:   timestamp(params.CreatedAt),
	})
	if err != nil {
		return domain.Listing{}, fmt.Errorf("insert listing: %w", err)
	}
	item, err := queries.GetListingByID(ctx, id)
	if err != nil {
		return domain.Listing{}, fmt.Errorf("read created listing: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return domain.Listing{}, fmt.Errorf("commit create listing transaction: %w", err)
	}
	return mapListing(item.Listing, item.Category, item.User), nil
}

func (s *CatalogStore) GetListing(ctx context.Context, id int64) (domain.Listing, error) {
	item, err := generateddb.New(s.pool).GetListingByID(ctx, id)
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.Listing{}, domain.ErrNotFound
	}
	if err != nil {
		return domain.Listing{}, fmt.Errorf("get listing by ID: %w", err)
	}
	return mapListing(item.Listing, item.Category, item.User), nil
}

func (s *CatalogStore) UpdateListing(ctx context.Context, params app.ListingUpdateParams) (domain.Listing, error) {
	var condition *string
	if params.Condition != nil {
		value := string(*params.Condition)
		condition = &value
	}
	tx, err := s.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return domain.Listing{}, fmt.Errorf("begin update listing transaction: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	queries := generateddb.New(tx)
	id, err := queries.UpdateListing(ctx, generateddb.UpdateListingParams{
		Title:       params.Title,
		Description: params.Description,
		PriceIdr:    params.PriceIDR,
		Quantity:    params.Quantity,
		CategoryID:  params.CategoryID,
		Condition:   condition,
		Negotiable:  params.Negotiable,
		UpdatedAt:   timestamp(params.UpdatedAt),
		ID:          params.ID,
		SellerID:    params.SellerID,
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.Listing{}, domain.ErrNotFound
	}
	if err != nil {
		return domain.Listing{}, fmt.Errorf("update listing: %w", err)
	}
	item, err := queries.GetListingByID(ctx, id)
	if err != nil {
		return domain.Listing{}, fmt.Errorf("read updated listing: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return domain.Listing{}, fmt.Errorf("commit update listing transaction: %w", err)
	}
	return mapListing(item.Listing, item.Category, item.User), nil
}

func (s *CatalogStore) ChangeListingStatus(ctx context.Context, params app.ListingStatusChangeParams) (domain.Listing, error) {
	tx, err := s.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return domain.Listing{}, fmt.Errorf("begin change listing status transaction: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	queries := generateddb.New(tx)
	id, err := queries.ChangeListingStatus(ctx, generateddb.ChangeListingStatusParams{
		Status:    string(params.Status),
		UpdatedAt: timestamp(params.UpdatedAt),
		ID:        params.ID,
		SellerID:  params.SellerID,
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.Listing{}, domain.ErrNotFound
	}
	if err != nil {
		return domain.Listing{}, fmt.Errorf("change listing status: %w", err)
	}
	item, err := queries.GetListingByID(ctx, id)
	if err != nil {
		return domain.Listing{}, fmt.Errorf("read changed listing status: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return domain.Listing{}, fmt.Errorf("commit change listing status transaction: %w", err)
	}
	return mapListing(item.Listing, item.Category, item.User), nil
}

func (s *CatalogStore) ListOwnedListings(ctx context.Context, params app.OwnedListingsParams) ([]domain.Listing, error) {
	var status *string
	if params.Status != nil {
		value := string(*params.Status)
		status = &value
	}
	rows, err := generateddb.New(s.pool).ListOwnedListings(ctx, generateddb.ListOwnedListingsParams{
		SellerID:        params.SellerID,
		Status:          status,
		CursorUpdatedAt: nullableTimestamp(params.CursorUpdatedAt),
		CursorID:        params.CursorID,
		PageLimit:       int32(params.Limit),
	})
	if err != nil {
		return nil, fmt.Errorf("list owned listings: %w", err)
	}
	items := make([]domain.Listing, 0, len(rows))
	for _, row := range rows {
		items = append(items, mapListing(row.Listing, row.Category, row.User))
	}
	return items, nil
}

func (s *CatalogStore) SearchListings(ctx context.Context, params app.SearchListingsParams) ([]domain.Listing, error) {
	var condition *string
	if params.Condition != nil {
		value := string(*params.Condition)
		condition = &value
	}
	switch params.Sort {
	case app.SortNewest:
		rows, err := generateddb.New(s.pool).SearchListingsNewest(ctx, generateddb.SearchListingsNewestParams{
			Query:           params.Query,
			CategoryID:      params.CategoryID,
			Condition:       condition,
			MinPrice:        params.MinPrice,
			MaxPrice:        params.MaxPrice,
			CursorCreatedAt: nullableTimestamp(params.CursorCreatedAt),
			CursorID:        params.CursorID,
			PageLimit:       int32(params.Limit),
		})
		return mapSearchNewestRows(rows, err)
	case app.SortPriceAsc:
		rows, err := generateddb.New(s.pool).SearchListingsPriceAsc(ctx, generateddb.SearchListingsPriceAscParams{
			Query:          params.Query,
			CategoryID:     params.CategoryID,
			Condition:      condition,
			MinPrice:       params.MinPrice,
			MaxPrice:       params.MaxPrice,
			CursorPriceIdr: params.CursorPriceIDR,
			CursorID:       params.CursorID,
			PageLimit:      int32(params.Limit),
		})
		return mapSearchPriceAscRows(rows, err)
	case app.SortPriceDesc:
		rows, err := generateddb.New(s.pool).SearchListingsPriceDesc(ctx, generateddb.SearchListingsPriceDescParams{
			Query:          params.Query,
			CategoryID:     params.CategoryID,
			Condition:      condition,
			MinPrice:       params.MinPrice,
			MaxPrice:       params.MaxPrice,
			CursorPriceIdr: params.CursorPriceIDR,
			CursorID:       params.CursorID,
			PageLimit:      int32(params.Limit),
		})
		return mapSearchPriceDescRows(rows, err)
	default:
		return nil, fmt.Errorf("unsupported listing sort %q", params.Sort)
	}
}

func mapSearchNewestRows(rows []generateddb.SearchListingsNewestRow, err error) ([]domain.Listing, error) {
	if err != nil {
		return nil, fmt.Errorf("search listings by newest: %w", err)
	}
	items := make([]domain.Listing, 0, len(rows))
	for _, row := range rows {
		items = append(items, mapListing(row.Listing, row.Category, row.User))
	}
	return items, nil
}

func mapSearchPriceAscRows(rows []generateddb.SearchListingsPriceAscRow, err error) ([]domain.Listing, error) {
	if err != nil {
		return nil, fmt.Errorf("search listings by ascending price: %w", err)
	}
	items := make([]domain.Listing, 0, len(rows))
	for _, row := range rows {
		items = append(items, mapListing(row.Listing, row.Category, row.User))
	}
	return items, nil
}

func mapSearchPriceDescRows(rows []generateddb.SearchListingsPriceDescRow, err error) ([]domain.Listing, error) {
	if err != nil {
		return nil, fmt.Errorf("search listings by descending price: %w", err)
	}
	items := make([]domain.Listing, 0, len(rows))
	for _, row := range rows {
		items = append(items, mapListing(row.Listing, row.Category, row.User))
	}
	return items, nil
}

func mapCategory(category generateddb.Category) domain.Category {
	return domain.Category{
		ID: category.ID, Slug: category.Slug, Name: category.Name,
		SortOrder: category.SortOrder, Active: category.Active,
	}
}

func mapListing(listing generateddb.Listing, category generateddb.Category, seller generateddb.User) domain.Listing {
	return domain.Listing{
		ID:               listing.ID,
		SellerID:         listing.SellerID,
		Category:         mapCategory(category),
		Title:            listing.Title,
		Description:      listing.Description,
		PriceIDR:         listing.PriceIdr,
		Quantity:         listing.Quantity,
		Condition:        domain.ListingCondition(listing.Condition),
		Status:           domain.ListingStatus(listing.Status),
		ModerationStatus: domain.ModerationStatus(listing.ModerationStatus),
		Negotiable:       listing.Negotiable,
		Seller: domain.PublicUser{
			ID: seller.ID, Handle: seller.Handle, DisplayName: seller.DisplayName,
			AvatarURL: seller.AvatarUrl, Location: seller.Location,
		},
		CreatedAt: listing.CreatedAt.Time.UTC(),
		UpdatedAt: listing.UpdatedAt.Time.UTC(),
	}
}

func nullableTimestamp(value *time.Time) pgtype.Timestamptz {
	if value == nil {
		return pgtype.Timestamptz{}
	}
	return timestamp(value.UTC())
}
