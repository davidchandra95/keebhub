package app_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/davidchandra95/keebhub/internal/app"
	"github.com/davidchandra95/keebhub/internal/domain"
)

func TestCatalogCreateAppliesDefaultsAndNormalizesFields(t *testing.T) {
	t.Parallel()

	repository := &catalogRepository{categories: map[string]domain.Category{"keyboard": {ID: 7, Slug: "keyboard", Active: true}}}
	now := time.Date(2026, 8, 8, 12, 0, 0, 0, time.UTC)
	service := app.NewCatalogService(repository, repository, func() time.Time { return now })
	_, err := service.CreateListing(context.Background(), 42, app.CreateListingInput{
		Title: "  Neo 98  ", CategorySlug: " keyboard ", PriceIDR: 3_000_000, Quantity: 1, Condition: domain.ListingConditionUsed,
	})
	if err != nil {
		t.Fatalf("CreateListing() error = %v", err)
	}
	if repository.create.SellerID != 42 || repository.create.CategoryID != 7 || repository.create.Title != "Neo 98" || repository.create.Description != "" || repository.create.Quantity != 1 || repository.create.Negotiable || repository.create.CreatedAt != now || repository.create.UpdatedAt != now {
		t.Errorf("create params = %+v", repository.create)
	}
}

func TestCatalogCreateRejectsInactiveCategory(t *testing.T) {
	t.Parallel()

	repository := &catalogRepository{categoryErr: domain.ErrNotFound}
	service := app.NewCatalogService(repository, repository, time.Now)
	_, err := service.CreateListing(context.Background(), 42, validCreateInput())
	var validation *domain.ValidationError
	if !errors.As(err, &validation) || validation.Fields["category_slug"] == "" {
		t.Fatalf("CreateListing() error = %v, want category validation error", err)
	}
}

func TestCatalogUpdateChecksOwnershipAndOnlyWritesProvidedFields(t *testing.T) {
	t.Parallel()

	title := "  Updated Neo 98  "
	price := int64(2_900_000)
	repository := &catalogRepository{
		listing:    domain.Listing{ID: 100, SellerID: 42, ModerationStatus: domain.ModerationStatusVisible},
		categories: map[string]domain.Category{"keyboard": {ID: 7, Slug: "keyboard", Active: true}},
	}
	now := time.Date(2026, 8, 8, 13, 0, 0, 0, time.UTC)
	service := app.NewCatalogService(repository, repository, func() time.Time { return now })
	_, err := service.UpdateListing(context.Background(), 42, 100, app.UpdateListingInput{Title: &title, PriceIDR: &price})
	if err != nil {
		t.Fatalf("UpdateListing() error = %v", err)
	}
	if repository.update.Title == nil || *repository.update.Title != "Updated Neo 98" || repository.update.PriceIDR == nil || *repository.update.PriceIDR != price || repository.update.Description != nil || repository.update.CategoryID != nil || repository.update.UpdatedAt != now {
		t.Errorf("update params = %+v", repository.update)
	}

	repository.updateCalled = false
	_, err = service.UpdateListing(context.Background(), 77, 100, app.UpdateListingInput{PriceIDR: &price})
	if !errors.Is(err, domain.ErrForbidden) || repository.updateCalled {
		t.Errorf("cross-owner update error/call = %v/%v", err, repository.updateCalled)
	}
}

func TestCatalogStatusTransitionNoOpDoesNotWrite(t *testing.T) {
	t.Parallel()

	repository := &catalogRepository{listing: domain.Listing{ID: 100, SellerID: 42, Status: domain.ListingStatusSold, ModerationStatus: domain.ModerationStatusVisible}}
	service := app.NewCatalogService(repository, repository, time.Now)
	listing, err := service.ChangeListingStatus(context.Background(), 42, 100, domain.ListingStatusSold)
	if err != nil || listing.ID != 100 || repository.statusCalled {
		t.Errorf("same status result = %+v, error = %v, write = %v", listing, err, repository.statusCalled)
	}
	_, err = service.ChangeListingStatus(context.Background(), 42, 100, domain.ListingStatusReserved)
	if !errors.Is(err, domain.ErrConflict) || repository.statusCalled {
		t.Errorf("invalid transition error/call = %v/%v", err, repository.statusCalled)
	}
}

func TestCatalogDetailVisibility(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		listing  domain.Listing
		viewerID *int64
		wantErr  error
	}{
		{name: "active anonymous", listing: domain.Listing{SellerID: 42, Status: domain.ListingStatusActive, ModerationStatus: domain.ModerationStatusVisible}},
		{name: "archived anonymous", listing: domain.Listing{SellerID: 42, Status: domain.ListingStatusArchived, ModerationStatus: domain.ModerationStatusVisible}, wantErr: domain.ErrNotFound},
		{name: "archived owner", listing: domain.Listing{SellerID: 42, Status: domain.ListingStatusArchived, ModerationStatus: domain.ModerationStatusVisible}, viewerID: int64Pointer(42)},
		{name: "removed owner", listing: domain.Listing{SellerID: 42, Status: domain.ListingStatusActive, ModerationStatus: domain.ModerationStatusRemoved}, viewerID: int64Pointer(42), wantErr: domain.ErrNotFound},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			repository := &catalogRepository{listing: tt.listing}
			service := app.NewCatalogService(repository, repository, time.Now)
			_, err := service.GetListing(context.Background(), 100, tt.viewerID)
			if !errors.Is(err, tt.wantErr) {
				t.Errorf("GetListing() error = %v, want %v", err, tt.wantErr)
			}
		})
	}
}

func TestCatalogSearchNormalizesOptionsAndEscapesWildcards(t *testing.T) {
	t.Parallel()

	price := int64(100)
	condition := domain.ListingConditionUsed
	repository := &catalogRepository{searchRows: []domain.Listing{{ID: 1}, {ID: 2}}}
	service := app.NewCatalogService(repository, repository, time.Now)
	page, err := service.SearchListings(context.Background(), app.SearchListingsOptions{
		Query: "  100%_\\  ", Category: " keyboard ", Condition: &condition, MinimumPrice: &price, Sort: app.ListingSortPriceAsc, Limit: 1,
	})
	if err != nil {
		t.Fatalf("SearchListings() error = %v", err)
	}
	if repository.search.Query == nil || *repository.search.Query != "100\\%\\_\\\\" || repository.search.Category == nil || *repository.search.Category != "keyboard" || repository.search.Sort != app.ListingSortPriceAsc || len(page.Items) != 1 || page.NextCursor == nil {
		t.Errorf("search params/page = %+v/%+v", repository.search, page)
	}
	_, err = service.SearchListings(context.Background(), app.SearchListingsOptions{MinimumPrice: int64Pointer(200), MaximumPrice: int64Pointer(100)})
	var query *domain.QueryError
	if !errors.As(err, &query) || query.Fields["min_price"] == "" {
		t.Errorf("invalid range error = %v", err)
	}
}

func validCreateInput() app.CreateListingInput {
	return app.CreateListingInput{Title: "Neo 98", CategorySlug: "keyboard", PriceIDR: 3_000_000, Quantity: 1, Condition: domain.ListingConditionUsed}
}

type catalogRepository struct {
	categories   map[string]domain.Category
	categoryErr  error
	listing      domain.Listing
	getErr       error
	create       app.CreateListingParams
	update       app.UpdateListingParams
	search       app.SearchListingsQuery
	searchRows   []domain.Listing
	updateCalled bool
	statusCalled bool
}

func (r *catalogRepository) ListActiveCategories(context.Context) ([]domain.Category, error) {
	return nil, nil
}
func (r *catalogRepository) FindActiveCategoryBySlug(_ context.Context, slug string) (domain.Category, error) {
	if r.categoryErr != nil {
		return domain.Category{}, r.categoryErr
	}
	category, ok := r.categories[slug]
	if !ok {
		return domain.Category{}, domain.ErrNotFound
	}
	return category, nil
}
func (r *catalogRepository) CreateListing(_ context.Context, params app.CreateListingParams) (domain.Listing, error) {
	r.create = params
	return domain.Listing{ID: 100, SellerID: params.SellerID, CategoryID: params.CategoryID}, nil
}
func (r *catalogRepository) GetListing(context.Context, int64) (domain.Listing, error) {
	return r.listing, r.getErr
}
func (r *catalogRepository) UpdateOwnedListing(_ context.Context, params app.UpdateListingParams) (domain.Listing, error) {
	r.updateCalled = true
	r.update = params
	return r.listing, nil
}
func (r *catalogRepository) UpdateOwnedListingStatus(_ context.Context, _, _ int64, status domain.ListingStatus, _ time.Time) (domain.Listing, error) {
	r.statusCalled = true
	r.listing.Status = status
	return r.listing, nil
}
func (r *catalogRepository) ListOwnedListings(context.Context, app.OwnedListingQuery) ([]domain.Listing, error) {
	return nil, nil
}
func (r *catalogRepository) SearchListings(_ context.Context, params app.SearchListingsQuery) ([]domain.Listing, error) {
	r.search = params
	return r.searchRows, nil
}

func int64Pointer(value int64) *int64 { return &value }
