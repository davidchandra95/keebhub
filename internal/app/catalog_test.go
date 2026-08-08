package app

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/davidchandra95/keebhub/internal/domain"
)

type catalogTestStore struct {
	categories     []domain.Category
	categoryBySlug map[string]domain.Category
	categoryErr    error
	createFn       func(CreateListingParams) (domain.Listing, error)
	getFn          func(int64) (domain.Listing, error)
	updateFn       func(UpdateOwnedListingParams) (domain.Listing, error)
	changeStatusFn func(ChangeOwnedListingStatusParams) (domain.Listing, error)
	listOwnedFn    func(ListOwnedListingsParams) ([]domain.Listing, error)
	searchFn       func(SearchListingsParams) ([]domain.Listing, error)
}

func (s *catalogTestStore) ListActive(context.Context) ([]domain.Category, error) {
	if s.categoryErr != nil {
		return nil, s.categoryErr
	}
	return s.categories, nil
}

func (s *catalogTestStore) FindActiveBySlug(_ context.Context, slug string) (domain.Category, error) {
	if s.categoryErr != nil {
		return domain.Category{}, s.categoryErr
	}
	category, ok := s.categoryBySlug[slug]
	if !ok {
		return domain.Category{}, domain.ErrCategoryNotFound
	}
	return category, nil
}

func (s *catalogTestStore) Create(_ context.Context, params CreateListingParams) (domain.Listing, error) {
	if s.createFn == nil {
		return domain.Listing{}, errors.New("unexpected create")
	}
	return s.createFn(params)
}

func (s *catalogTestStore) GetByID(_ context.Context, listingID int64) (domain.Listing, error) {
	if s.getFn == nil {
		return domain.Listing{}, errors.New("unexpected get")
	}
	return s.getFn(listingID)
}

func (s *catalogTestStore) UpdateOwned(_ context.Context, params UpdateOwnedListingParams) (domain.Listing, error) {
	if s.updateFn == nil {
		return domain.Listing{}, errors.New("unexpected update")
	}
	return s.updateFn(params)
}

func (s *catalogTestStore) ChangeOwnedStatus(_ context.Context, params ChangeOwnedListingStatusParams) (domain.Listing, error) {
	if s.changeStatusFn == nil {
		return domain.Listing{}, errors.New("unexpected status write")
	}
	return s.changeStatusFn(params)
}

func (s *catalogTestStore) ListOwned(_ context.Context, params ListOwnedListingsParams) ([]domain.Listing, error) {
	if s.listOwnedFn == nil {
		return nil, errors.New("unexpected list owned")
	}
	return s.listOwnedFn(params)
}

func (s *catalogTestStore) Search(_ context.Context, params SearchListingsParams) ([]domain.Listing, error) {
	if s.searchFn == nil {
		return nil, errors.New("unexpected search")
	}
	return s.searchFn(params)
}

func TestCatalogServiceCreateListingAppliesDefaultsAndClock(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 8, 8, 12, 0, 0, 0, time.FixedZone("WIB", 7*60*60))
	store := &catalogTestStore{
		categoryBySlug: map[string]domain.Category{
			"keyboard": {ID: 10, Slug: "keyboard", Name: "Keyboard"},
		},
		createFn: func(params CreateListingParams) (domain.Listing, error) {
			if params.SellerID != 42 || params.CategoryID != 10 {
				t.Errorf("owner/category = %d/%d", params.SellerID, params.CategoryID)
			}
			if params.Title != "Neo 98" || params.Description != "line one\nline two" {
				t.Errorf("text params = %+v", params)
			}
			if params.PriceIDR != 3_000_000 || params.Quantity != 1 || params.Negotiable {
				t.Errorf("create defaults = %+v", params)
			}
			if params.Condition != domain.ListingConditionUsed || params.Status != domain.ListingStatusActive ||
				params.ModerationStatus != domain.ModerationStatusVisible {
				t.Errorf("listing state defaults = %+v", params)
			}
			if !params.CreatedAt.Equal(now.UTC()) || !params.UpdatedAt.Equal(now.UTC()) {
				t.Errorf("timestamps = %s / %s", params.CreatedAt, params.UpdatedAt)
			}
			return domain.Listing{ID: 99, SellerID: params.SellerID, Status: params.Status}, nil
		},
	}
	service := NewCatalogService(store, store, func() time.Time { return now })

	listing, err := service.CreateListing(context.Background(), domain.User{ID: 42}, CreateListingInput{
		Title:        "  Neo 98  ",
		Description:  "line one\nline two",
		PriceIDR:     3_000_000,
		Quantity:     1,
		CategorySlug: " keyboard ",
		Condition:    domain.ListingConditionUsed,
	})
	if err != nil {
		t.Fatalf("CreateListing() error = %v", err)
	}
	if listing.ID != 99 {
		t.Errorf("listing ID = %d, want 99", listing.ID)
	}
}

func TestCatalogServiceRejectsMissingOrInactiveCategory(t *testing.T) {
	t.Parallel()

	store := &catalogTestStore{categoryBySlug: map[string]domain.Category{}}
	service := NewCatalogService(store, store, time.Now)
	_, err := service.CreateListing(context.Background(), domain.User{ID: 1}, validCreateInput())
	var validationError *domain.ValidationError
	if !errors.As(err, &validationError) || validationError.Field != "category_slug" {
		t.Fatalf("error = %v, want category_slug validation error", err)
	}
}

func TestCatalogServicePreservesRepositoryFailures(t *testing.T) {
	t.Parallel()

	want := errors.New("database unavailable")
	store := &catalogTestStore{
		categoryErr: want,
		searchFn: func(SearchListingsParams) ([]domain.Listing, error) {
			return nil, want
		},
	}
	service := NewCatalogService(store, store, time.Now)
	if _, err := service.ListCategories(context.Background()); !errors.Is(err, want) {
		t.Errorf("ListCategories() error = %v, want wrapped repository error", err)
	}
	if _, err := service.SearchListings(context.Background(), SearchListingsInput{}); !errors.Is(err, want) {
		t.Errorf("SearchListings() error = %v, want wrapped repository error", err)
	}
}

func TestCatalogServiceUpdateChecksOwnershipThenUsesPartialFields(t *testing.T) {
	t.Parallel()

	title := "  Updated title  "
	condition := domain.ListingConditionNew
	store := &catalogTestStore{
		categoryBySlug: map[string]domain.Category{
			"keycaps": {ID: 20, Slug: "keycaps", Name: "Keycaps"},
		},
		getFn: func(listingID int64) (domain.Listing, error) {
			return domain.Listing{ID: listingID, SellerID: 42, ModerationStatus: domain.ModerationStatusVisible}, nil
		},
		updateFn: func(params UpdateOwnedListingParams) (domain.Listing, error) {
			if params.Title == nil || *params.Title != "Updated title" {
				t.Errorf("title = %v", params.Title)
			}
			if params.CategoryID == nil || *params.CategoryID != 20 {
				t.Errorf("category ID = %v", params.CategoryID)
			}
			if params.Condition == nil || *params.Condition != condition {
				t.Errorf("condition = %v", params.Condition)
			}
			if params.Description != nil || params.PriceIDR != nil || params.Quantity != nil || params.Negotiable != nil {
				t.Errorf("unexpected partial update values = %+v", params)
			}
			return domain.Listing{ID: params.ListingID, SellerID: params.SellerID}, nil
		},
	}
	service := NewCatalogService(store, store, func() time.Time {
		return time.Date(2026, 8, 8, 12, 0, 0, 0, time.UTC)
	})

	_, err := service.UpdateListing(context.Background(), domain.User{ID: 42}, 10, UpdateListingInput{
		Title:        &title,
		CategorySlug: pointer("keycaps"),
		Condition:    &condition,
	})
	if err != nil {
		t.Fatalf("UpdateListing() error = %v", err)
	}
}

func TestCatalogServiceRejectsCrossSellerUpdate(t *testing.T) {
	t.Parallel()

	title := "Updated"
	store := &catalogTestStore{
		getFn: func(listingID int64) (domain.Listing, error) {
			return domain.Listing{ID: listingID, SellerID: 99, ModerationStatus: domain.ModerationStatusVisible}, nil
		},
	}
	service := NewCatalogService(store, store, time.Now)
	_, err := service.UpdateListing(context.Background(), domain.User{ID: 42}, 10, UpdateListingInput{Title: &title})
	if !errors.Is(err, domain.ErrForbidden) {
		t.Errorf("error = %v, want forbidden", err)
	}
}

func TestCatalogServiceSameStatusIsNoWrite(t *testing.T) {
	t.Parallel()

	updatedAt := time.Date(2026, 8, 8, 12, 0, 0, 0, time.UTC)
	store := &catalogTestStore{
		getFn: func(listingID int64) (domain.Listing, error) {
			return domain.Listing{
				ID: listingID, SellerID: 42, Status: domain.ListingStatusActive,
				ModerationStatus: domain.ModerationStatusVisible, UpdatedAt: updatedAt,
			}, nil
		},
		changeStatusFn: func(ChangeOwnedListingStatusParams) (domain.Listing, error) {
			t.Fatal("same-status transition must not write")
			return domain.Listing{}, nil
		},
	}
	service := NewCatalogService(store, store, time.Now)
	change, err := service.ChangeListingStatus(context.Background(), domain.User{ID: 42}, 10, domain.ListingStatusActive)
	if err != nil {
		t.Fatalf("ChangeListingStatus() error = %v", err)
	}
	if change.Changed || change.OldStatus != domain.ListingStatusActive || !change.Listing.UpdatedAt.Equal(updatedAt) {
		t.Errorf("change = %+v", change)
	}
}

func TestCatalogServiceListingVisibility(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name         string
		listing      domain.Listing
		viewer       *domain.User
		wantNotFound bool
	}{
		{
			name:    "active anonymous",
			listing: domain.Listing{ID: 1, SellerID: 42, Status: domain.ListingStatusActive, ModerationStatus: domain.ModerationStatusVisible},
		},
		{
			name:         "archived anonymous",
			listing:      domain.Listing{ID: 1, SellerID: 42, Status: domain.ListingStatusArchived, ModerationStatus: domain.ModerationStatusVisible},
			wantNotFound: true,
		},
		{
			name:    "archived owner",
			listing: domain.Listing{ID: 1, SellerID: 42, Status: domain.ListingStatusArchived, ModerationStatus: domain.ModerationStatusVisible},
			viewer:  &domain.User{ID: 42},
		},
		{
			name:         "removed owner",
			listing:      domain.Listing{ID: 1, SellerID: 42, Status: domain.ListingStatusActive, ModerationStatus: domain.ModerationStatusRemoved},
			viewer:       &domain.User{ID: 42},
			wantNotFound: true,
		},
	}

	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			store := &catalogTestStore{
				getFn: func(int64) (domain.Listing, error) { return test.listing, nil },
			}
			service := NewCatalogService(store, store, time.Now)
			_, err := service.GetListing(context.Background(), 1, test.viewer)
			if test.wantNotFound {
				if !errors.Is(err, domain.ErrListingNotFound) {
					t.Errorf("error = %v, want not found", err)
				}
				return
			}
			if err != nil {
				t.Fatalf("GetListing() error = %v", err)
			}
		})
	}
}

func TestCatalogServiceSearchNormalizesEscapesAndBindsCursor(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 8, 8, 12, 0, 0, 0, time.UTC)
	var firstParams SearchListingsParams
	store := &catalogTestStore{
		searchFn: func(params SearchListingsParams) ([]domain.Listing, error) {
			firstParams = params
			return []domain.Listing{
				{ID: 3, PriceIDR: 10, CreatedAt: now},
				{ID: 2, PriceIDR: 20, CreatedAt: now.Add(-time.Second)},
				{ID: 1, PriceIDR: 30, CreatedAt: now.Add(-2 * time.Second)},
			}, nil
		},
	}
	service := NewCatalogService(store, store, time.Now)
	minPrice, maxPrice := int64(10), int64(100)
	page, err := service.SearchListings(context.Background(), SearchListingsInput{
		Query:     "%_\\ ",
		Category:  " keyboard ",
		Condition: "used",
		MinPrice:  &minPrice,
		MaxPrice:  &maxPrice,
		Sort:      "price_asc",
		Limit:     2,
	})
	if err != nil {
		t.Fatalf("SearchListings() error = %v", err)
	}
	if firstParams.Query == nil || *firstParams.Query != "\\%\\_\\\\" {
		t.Errorf("escaped query = %v", firstParams.Query)
	}
	if firstParams.Category == nil || *firstParams.Category != "keyboard" || firstParams.PageSize != 3 {
		t.Errorf("search params = %+v", firstParams)
	}
	if len(page.Items) != 2 || page.NextCursor == nil {
		t.Fatalf("page = %+v", page)
	}

	var secondParams SearchListingsParams
	store.searchFn = func(params SearchListingsParams) ([]domain.Listing, error) {
		secondParams = params
		return []domain.Listing{}, nil
	}
	_, err = service.SearchListings(context.Background(), SearchListingsInput{
		Query:     "%_\\",
		Category:  "keyboard",
		Condition: "used",
		MinPrice:  &minPrice,
		MaxPrice:  &maxPrice,
		Sort:      "price_asc",
		Cursor:    *page.NextCursor,
		Limit:     1,
	})
	if err != nil {
		t.Fatalf("SearchListings() with cursor error = %v", err)
	}
	if secondParams.Cursor == nil || secondParams.Cursor.PriceIDR != 20 || secondParams.Cursor.ID != 2 {
		t.Errorf("second cursor = %+v", secondParams.Cursor)
	}

	_, err = service.SearchListings(context.Background(), SearchListingsInput{
		Query:     "changed",
		Category:  "keyboard",
		Condition: "used",
		MinPrice:  &minPrice,
		MaxPrice:  &maxPrice,
		Sort:      "price_asc",
		Cursor:    *page.NextCursor,
	})
	if !errors.Is(err, ErrInvalidCatalogQuery) {
		t.Errorf("reused cursor error = %v, want invalid catalog query", err)
	}
}

func TestCatalogServiceSearchOptionValidation(t *testing.T) {
	t.Parallel()

	service := NewCatalogService(nil, &catalogTestStore{}, time.Now)
	min := int64(10)
	max := int64(5)
	tests := []SearchListingsInput{
		{Query: strings.Repeat("a", 101)},
		{Condition: "vintage"},
		{MinPrice: &min, MaxPrice: &max},
		{Sort: "oldest"},
		{Limit: 101},
	}
	for _, input := range tests {
		_, err := service.SearchListings(context.Background(), input)
		if !errors.Is(err, ErrInvalidCatalogQuery) {
			t.Errorf("input %+v error = %v", input, err)
		}
	}
}

func TestCatalogServiceOwnedCursorIsBoundToStatus(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 8, 8, 12, 0, 0, 0, time.UTC)
	store := &catalogTestStore{
		listOwnedFn: func(ListOwnedListingsParams) ([]domain.Listing, error) {
			return []domain.Listing{
				{ID: 2, UpdatedAt: now},
				{ID: 1, UpdatedAt: now.Add(-time.Second)},
			}, nil
		},
	}
	service := NewCatalogService(store, store, time.Now)
	page, err := service.ListOwnedListings(context.Background(), domain.User{ID: 42}, OwnedListingsInput{
		Status: "active", Limit: 1,
	})
	if err != nil || page.NextCursor == nil {
		t.Fatalf("first owned page = %+v, error = %v", page, err)
	}
	_, err = service.ListOwnedListings(context.Background(), domain.User{ID: 42}, OwnedListingsInput{
		Status: "sold", Cursor: *page.NextCursor,
	})
	if !errors.Is(err, ErrInvalidCatalogQuery) {
		t.Errorf("changed status cursor error = %v", err)
	}
}

func validCreateInput() CreateListingInput {
	return CreateListingInput{
		Title:        "Neo 98",
		PriceIDR:     3_000_000,
		Quantity:     1,
		CategorySlug: "keyboard",
		Condition:    domain.ListingConditionUsed,
	}
}

func pointer[T any](value T) *T {
	return &value
}
