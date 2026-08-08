package app

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/davidchandra95/keebhub/internal/domain"
)

type catalogRepoFake struct {
	categories    []domain.Category
	listings      map[int64]domain.Listing
	searchResults []domain.Listing
	createParams  ListingCreateParams
	updateParams  ListingUpdateParams
	statusCalls   int
	searchParams  SearchListingsParams
	categoryErr   error
	listingErr    error
}

func (f *catalogRepoFake) ListActiveCategories(context.Context) ([]domain.Category, error) {
	return append([]domain.Category(nil), f.categories...), nil
}

func (f *catalogRepoFake) GetActiveCategoryBySlug(_ context.Context, slug string) (domain.Category, error) {
	if f.categoryErr != nil {
		return domain.Category{}, f.categoryErr
	}
	for _, category := range f.categories {
		if category.Active && category.Slug == slug {
			return category, nil
		}
	}
	return domain.Category{}, domain.ErrNotFound
}

func (f *catalogRepoFake) CreateListing(_ context.Context, params ListingCreateParams) (domain.Listing, error) {
	f.createParams = params
	listing := domain.Listing{
		ID: 1, SellerID: params.SellerID, Title: params.Title, Description: params.Description,
		PriceIDR: params.PriceIDR, Quantity: params.Quantity, Condition: params.Condition,
		Status: domain.StatusActive, ModerationStatus: domain.ModerationVisible,
		CreatedAt: params.CreatedAt, UpdatedAt: params.CreatedAt,
		Category: domain.Category{ID: params.CategoryID, Slug: "keyboard", Name: "Keyboard"},
		Seller:   domain.PublicUser{ID: params.SellerID, Handle: "seller", DisplayName: "Seller"},
	}
	if f.listings == nil {
		f.listings = map[int64]domain.Listing{}
	}
	f.listings[listing.ID] = listing
	return listing, nil
}

func (f *catalogRepoFake) GetListing(_ context.Context, id int64) (domain.Listing, error) {
	if f.listingErr != nil {
		return domain.Listing{}, f.listingErr
	}
	listing, ok := f.listings[id]
	if !ok {
		return domain.Listing{}, domain.ErrNotFound
	}
	return listing, nil
}

func (f *catalogRepoFake) UpdateListing(_ context.Context, params ListingUpdateParams) (domain.Listing, error) {
	f.updateParams = params
	listing, ok := f.listings[params.ID]
	if !ok || listing.SellerID != params.SellerID {
		return domain.Listing{}, domain.ErrNotFound
	}
	if params.Title != nil {
		listing.Title = *params.Title
	}
	if params.Description != nil {
		listing.Description = *params.Description
	}
	if params.PriceIDR != nil {
		listing.PriceIDR = *params.PriceIDR
	}
	if params.Quantity != nil {
		listing.Quantity = *params.Quantity
	}
	if params.CategoryID != nil {
		listing.Category.ID = *params.CategoryID
	}
	if params.Condition != nil {
		listing.Condition = *params.Condition
	}
	if params.Negotiable != nil {
		listing.Negotiable = *params.Negotiable
	}
	listing.UpdatedAt = params.UpdatedAt
	f.listings[params.ID] = listing
	return listing, nil
}

func (f *catalogRepoFake) ChangeListingStatus(_ context.Context, params ListingStatusChangeParams) (domain.Listing, error) {
	f.statusCalls++
	listing, ok := f.listings[params.ID]
	if !ok || listing.SellerID != params.SellerID {
		return domain.Listing{}, domain.ErrNotFound
	}
	listing.Status = params.Status
	listing.UpdatedAt = params.UpdatedAt
	f.listings[params.ID] = listing
	return listing, nil
}

func (f *catalogRepoFake) ListOwnedListings(_ context.Context, params OwnedListingsParams) ([]domain.Listing, error) {
	items := make([]domain.Listing, 0, len(f.listings))
	for _, listing := range f.listings {
		if listing.SellerID == params.SellerID && listing.ModerationStatus != domain.ModerationRemoved && (params.Status == nil || listing.Status == *params.Status) {
			items = append(items, listing)
		}
	}
	return items, nil
}

func (f *catalogRepoFake) SearchListings(_ context.Context, params SearchListingsParams) ([]domain.Listing, error) {
	f.searchParams = params
	return append([]domain.Listing(nil), f.searchResults...), nil
}

func testCategory() domain.Category {
	return domain.Category{ID: 10, Slug: "keyboard", Name: "Keyboard", SortOrder: 10, Active: true}
}

func testListing(id int64, createdAt time.Time) domain.Listing {
	return domain.Listing{
		ID: id, SellerID: 42, Category: testCategory(), Title: "Keyboard",
		Description: "Description", PriceIDR: 1000 + id, Quantity: 1,
		Condition: domain.ConditionUsed, Status: domain.StatusActive,
		ModerationStatus: domain.ModerationVisible, CreatedAt: createdAt, UpdatedAt: createdAt,
		Seller: domain.PublicUser{ID: 42, Handle: "seller", DisplayName: "Seller"},
	}
}

func TestCatalogServiceCreateDefaultsAndTrimsTitle(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 8, 8, 12, 0, 0, 0, time.UTC)
	repo := &catalogRepoFake{categories: []domain.Category{testCategory()}}
	service := NewCatalogServiceWithClock(repo, repo, func() time.Time { return now })
	listing, err := service.CreateListing(context.Background(), 42, CreateListingInput{
		Title: "  Neo 98  ", CategorySlug: " keyboard ", PriceIDR: 3000000, Condition: domain.ConditionUsed,
	})
	if err != nil {
		t.Fatalf("CreateListing() error = %v", err)
	}
	if listing.Title != "Neo 98" || repo.createParams.Quantity != 1 || repo.createParams.Description != "" || repo.createParams.Negotiable {
		t.Errorf("listing = %+v, params = %+v", listing, repo.createParams)
	}
	if !repo.createParams.CreatedAt.Equal(now) {
		t.Errorf("created at = %s, want %s", repo.createParams.CreatedAt, now)
	}
}

func TestCatalogServiceRejectsInactiveCategoryAndInvalidFields(t *testing.T) {
	t.Parallel()

	repo := &catalogRepoFake{categories: []domain.Category{{ID: 11, Slug: "keyboard", Name: "Keyboard", Active: false}}}
	service := NewCatalogService(repo, repo)
	_, err := service.CreateListing(context.Background(), 42, CreateListingInput{
		Title: strings.Repeat("字", domain.MaximumListingTitleLength+1), CategorySlug: "keyboard", PriceIDR: 0,
		Quantity: 0, Condition: domain.ListingCondition("broken"),
	})
	var validation *domain.ValidationError
	if !errors.As(err, &validation) {
		t.Fatalf("error = %v, want validation error", err)
	}
	for _, field := range []string{"title", "price_idr", "condition"} {
		if _, ok := validation.Fields[field]; !ok {
			t.Errorf("validation fields = %v, missing %q", validation.Fields, field)
		}
	}
	_, err = service.CreateListing(context.Background(), 42, CreateListingInput{
		Title: "Keyboard", CategorySlug: "missing", PriceIDR: 1000, Condition: domain.ConditionNew,
	})
	if !errors.As(err, &validation) || validation.Fields["category_slug"] == "" {
		t.Fatalf("inactive category error = %v", err)
	}
}

func TestCatalogServiceUpdateIsPartialAndChecksOwnership(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 8, 8, 13, 0, 0, 0, time.UTC)
	repo := &catalogRepoFake{
		categories: []domain.Category{testCategory()},
		listings:   map[int64]domain.Listing{7: testListing(7, now.Add(-time.Hour))},
	}
	service := NewCatalogServiceWithClock(repo, repo, func() time.Time { return now })
	description := "line one\nline two"
	listing, err := service.UpdateListing(context.Background(), 42, 7, UpdateListingInput{Description: &description})
	if err != nil {
		t.Fatalf("UpdateListing() error = %v", err)
	}
	if listing.Description != description || repo.updateParams.Description == nil || repo.updateParams.Title != nil || repo.updateParams.PriceIDR != nil {
		t.Errorf("listing = %+v, params = %+v", listing, repo.updateParams)
	}
	if _, err := service.UpdateListing(context.Background(), 99, 7, UpdateListingInput{Description: &description}); !errors.Is(err, domain.ErrForbidden) {
		t.Errorf("cross-seller update error = %v, want forbidden", err)
	}
}

func TestCatalogServiceSameStatusDoesNotWriteAndVisibilityRules(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 8, 8, 14, 0, 0, 0, time.UTC)
	archived := testListing(8, now)
	archived.Status = domain.StatusArchived
	removed := testListing(9, now)
	removed.ModerationStatus = domain.ModerationRemoved
	repo := &catalogRepoFake{listings: map[int64]domain.Listing{8: archived, 9: removed}}
	service := NewCatalogServiceWithClock(repo, repo, func() time.Time { return now.Add(time.Hour) })
	listing, err := service.ChangeListingStatus(context.Background(), 42, 8, domain.StatusArchived)
	if err != nil || listing.UpdatedAt != archived.UpdatedAt || repo.statusCalls != 0 {
		t.Fatalf("same status = %+v, error = %v, status calls = %d", listing, err, repo.statusCalls)
	}
	if _, err := service.GetListing(context.Background(), 8, nil); !errors.Is(err, domain.ErrNotFound) {
		t.Errorf("anonymous archived detail error = %v", err)
	}
	viewerID := int64(42)
	if _, err := service.GetListing(context.Background(), 8, &viewerID); err != nil {
		t.Errorf("owner archived detail error = %v", err)
	}
	if _, err := service.GetListing(context.Background(), 9, &viewerID); !errors.Is(err, domain.ErrNotFound) {
		t.Errorf("removed detail error = %v", err)
	}
}

func TestCatalogServiceSearchNormalizesEscapesAndBindsCursor(t *testing.T) {
	t.Parallel()

	base := time.Date(2026, 8, 8, 15, 0, 0, 0, time.UTC)
	repo := &catalogRepoFake{
		categories:    []domain.Category{testCategory()},
		searchResults: []domain.Listing{testListing(3, base), testListing(2, base.Add(-time.Minute)), testListing(1, base.Add(-2*time.Minute))},
	}
	service := NewCatalogService(repo, repo)
	page, err := service.SearchListings(context.Background(), SearchListingsInput{Query: " 100% ", Category: " keyboard ", Sort: SortNewest, Limit: 2})
	if err != nil {
		t.Fatalf("SearchListings() error = %v", err)
	}
	if page.NextCursor == nil || repo.searchParams.Query == nil || *repo.searchParams.Query != `100\%` {
		t.Fatalf("page = %+v, search params = %+v", page, repo.searchParams)
	}
	if repo.searchParams.CategoryID == nil || *repo.searchParams.CategoryID != testCategory().ID {
		t.Errorf("category ID = %v", repo.searchParams.CategoryID)
	}
	page, err = service.SearchListings(context.Background(), SearchListingsInput{Query: "100%", Category: "keyboard", Sort: SortNewest, Cursor: *page.NextCursor, Limit: 2})
	if err != nil {
		t.Fatalf("second SearchListings() error = %v", err)
	}
	if repo.searchParams.CursorID == nil || *repo.searchParams.CursorID != 2 {
		t.Errorf("cursor ID = %v", repo.searchParams.CursorID)
	}
	var badRequest *BadRequestError
	if _, err := service.SearchListings(context.Background(), SearchListingsInput{Query: "different", Sort: SortNewest, Cursor: *page.NextCursor, Limit: 2}); !errors.As(err, &badRequest) {
		t.Errorf("changed-filter cursor error = %v", err)
	}
	if len(page.Items) == 0 {
		t.Error("unexpected empty second page")
	}
}
