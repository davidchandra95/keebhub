package httpapi_test

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/davidchandra95/keebhub/internal/adapter/httpapi"
	"github.com/davidchandra95/keebhub/internal/app"
	"github.com/davidchandra95/keebhub/internal/domain"
)

func TestCatalogPublicRoutesSerializeIDsAndNullCursor(t *testing.T) {
	t.Parallel()

	catalog := &catalogHTTPFake{
		categories: []domain.Category{{ID: 7, Slug: "keyboard", Name: "Keyboard"}},
		page:       app.ListingPage{Items: []domain.Listing{httpListingFixture()}},
	}
	handler := newHandlerConfig(t, fakePinger{}, httpapi.Config{AppBaseURL: "http://localhost:8080", Catalog: catalog})

	categories := performRequest(handler, http.MethodGet, "/api/v1/categories", "", nil)
	if categories.Code != http.StatusOK || !strings.Contains(categories.Body.String(), `"id":"7"`) {
		t.Fatalf("categories = %d %s", categories.Code, categories.Body.String())
	}
	listings := performRequest(handler, http.MethodGet, "/api/v1/listings?q=%20Neo%20&sort=price_asc", "", nil)
	if listings.Code != http.StatusOK || !strings.Contains(listings.Body.String(), `"id":"100"`) || !strings.Contains(listings.Body.String(), `"next_cursor":null`) {
		t.Fatalf("listings = %d %s", listings.Code, listings.Body.String())
	}
	if catalog.searchOptions.Query != " Neo " || catalog.searchOptions.Sort != app.ListingSortPriceAsc {
		t.Errorf("search options = %+v", catalog.searchOptions)
	}
}

func TestCatalogCreateUsesStrictJSONAndDefaults(t *testing.T) {
	t.Parallel()

	catalog := &catalogHTTPFake{listing: httpListingFixture()}
	handler := authenticatedCatalogHandler(t, catalog)
	headers := map[string]string{"Origin": "http://localhost:8080", "Cookie": "keebhub_session=valid", "Content-Type": "application/json; charset=utf-8"}
	response := performRequest(handler, http.MethodPost, "/api/v1/listings", `{"title":"Neo 98","price_idr":3000000,"category_slug":"keyboard","condition":"used"}`, headers)
	if response.Code != http.StatusCreated || !strings.Contains(response.Body.String(), `"id":"100"`) {
		t.Fatalf("create = %d %s", response.Code, response.Body.String())
	}
	if catalog.createInput.Description != "" || catalog.createInput.Quantity != 1 || catalog.createInput.Negotiable {
		t.Errorf("create input defaults = %+v", catalog.createInput)
	}

	tests := []struct {
		name       string
		path       string
		body       string
		headers    map[string]string
		wantStatus int
		wantCode   string
	}{
		{name: "unknown field", path: "/api/v1/listings", body: `{"title":"Neo","price_idr":1,"category_slug":"keyboard","condition":"used","status":"sold"}`, headers: headers, wantStatus: http.StatusBadRequest, wantCode: "bad_request"},
		{name: "explicit null", path: "/api/v1/listings", body: `{"title":null,"price_idr":1,"category_slug":"keyboard","condition":"used"}`, headers: headers, wantStatus: http.StatusUnprocessableEntity, wantCode: "validation_failed"},
		{name: "empty patch", path: "/api/v1/listings/100", body: `{}`, headers: headers, wantStatus: http.StatusUnprocessableEntity, wantCode: "validation_failed"},
		{name: "multiple values", path: "/api/v1/listings", body: `{"title":"Neo","price_idr":1,"category_slug":"keyboard","condition":"used"} {}`, headers: headers, wantStatus: http.StatusBadRequest, wantCode: "bad_request"},
		{name: "unsupported content type", path: "/api/v1/listings", body: `{"title":"Neo"}`, headers: map[string]string{"Origin": "http://localhost:8080", "Cookie": "keebhub_session=valid", "Content-Type": "text/plain"}, wantStatus: http.StatusUnsupportedMediaType, wantCode: "unsupported_media_type"},
		{name: "oversized body", path: "/api/v1/listings", body: `{"title":"` + strings.Repeat("x", 33<<10) + `","price_idr":1,"category_slug":"keyboard","condition":"used"}`, headers: headers, wantStatus: http.StatusRequestEntityTooLarge, wantCode: "request_too_large"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			response := performRequest(handler, http.MethodPost, tt.path, tt.body, tt.headers)
			if tt.path == "/api/v1/listings/100" {
				response = performRequest(handler, http.MethodPatch, tt.path, tt.body, tt.headers)
			}
			if response.Code != tt.wantStatus {
				t.Fatalf("status = %d, want %d: %s", response.Code, tt.wantStatus, response.Body.String())
			}
			assertErrorCode(t, response.Body.Bytes(), tt.wantCode)
		})
	}
}

func TestCatalogAuthorizationAndErrorMapping(t *testing.T) {
	t.Parallel()

	catalog := &catalogHTTPFake{listing: httpListingFixture(), updateErr: domain.ErrForbidden, statusErr: domain.ErrConflict}
	handler := authenticatedCatalogHandler(t, catalog)

	noSession := performRequest(handler, http.MethodPost, "/api/v1/listings", `{}`, map[string]string{"Origin": "http://localhost:8080", "Content-Type": "application/json"})
	if noSession.Code != http.StatusUnauthorized {
		t.Fatalf("no-session create = %d", noSession.Code)
	}
	headers := map[string]string{"Origin": "http://localhost:8080", "Cookie": "keebhub_session=valid", "Content-Type": "application/json"}
	update := performRequest(handler, http.MethodPatch, "/api/v1/listings/100", `{"price_idr":2900000}`, headers)
	if update.Code != http.StatusForbidden {
		t.Fatalf("cross-owner update = %d %s", update.Code, update.Body.String())
	}
	status := performRequest(handler, http.MethodPost, "/api/v1/listings/100/status", `{"status":"sold"}`, headers)
	if status.Code != http.StatusConflict {
		t.Fatalf("invalid transition = %d %s", status.Code, status.Body.String())
	}
	invalidID := performRequest(handler, http.MethodGet, "/api/v1/listings/not-an-id", "", nil)
	if invalidID.Code != http.StatusNotFound {
		t.Fatalf("invalid ID = %d %s", invalidID.Code, invalidID.Body.String())
	}
}

func TestCatalogResponsesUseRFC3339UTC(t *testing.T) {
	t.Parallel()

	listing := httpListingFixture()
	listing.CreatedAt = time.Date(2026, 8, 8, 12, 0, 0, 123, time.FixedZone("WIB", 7*60*60))
	listing.UpdatedAt = listing.CreatedAt
	catalog := &catalogHTTPFake{listing: listing}
	handler := newHandlerConfig(t, fakePinger{}, httpapi.Config{AppBaseURL: "http://localhost:8080", Catalog: catalog})
	response := performRequest(handler, http.MethodGet, "/api/v1/listings/100", "", nil)
	if response.Code != http.StatusOK {
		t.Fatalf("detail = %d %s", response.Code, response.Body.String())
	}
	var body struct {
		Listing struct {
			CreatedAt string `json:"created_at"`
		} `json:"listing"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if body.Listing.CreatedAt != "2026-08-08T05:00:00Z" {
		t.Errorf("created_at = %q", body.Listing.CreatedAt)
	}
}

type catalogHTTPFake struct {
	categories    []domain.Category
	listing       domain.Listing
	page          app.ListingPage
	createInput   app.CreateListingInput
	searchOptions app.SearchListingsOptions
	updateErr     error
	statusErr     error
}

func (f *catalogHTTPFake) ListCategories(context.Context) ([]domain.Category, error) {
	return f.categories, nil
}
func (f *catalogHTTPFake) CreateListing(_ context.Context, _ int64, input app.CreateListingInput) (domain.Listing, error) {
	f.createInput = input
	return f.listing, nil
}
func (f *catalogHTTPFake) UpdateListing(context.Context, int64, int64, app.UpdateListingInput) (domain.Listing, error) {
	return f.listing, f.updateErr
}
func (f *catalogHTTPFake) ChangeListingStatus(context.Context, int64, int64, domain.ListingStatus) (domain.Listing, error) {
	return f.listing, f.statusErr
}
func (f *catalogHTTPFake) ListOwnedListings(context.Context, int64, app.OwnedListingOptions) (app.ListingPage, error) {
	return f.page, nil
}
func (f *catalogHTTPFake) GetListing(context.Context, int64, *int64) (domain.Listing, error) {
	return f.listing, nil
}
func (f *catalogHTTPFake) SearchListings(_ context.Context, options app.SearchListingsOptions) (app.ListingPage, error) {
	f.searchOptions = options
	return f.page, nil
}

func authenticatedCatalogHandler(t *testing.T, catalog httpapi.CatalogService) http.Handler {
	t.Helper()
	return newHandlerConfig(t, fakePinger{}, httpapi.Config{
		AppBaseURL:        "http://localhost:8080",
		Auth:              &fakeAuthenticator{authenticateUser: domain.User{ID: 42, Handle: "seller", DiscordUsername: "seller", DisplayName: "Seller"}},
		Catalog:           catalog,
		SessionCookieName: "keebhub_session",
	})
}

func httpListingFixture() domain.Listing {
	createdAt := time.Date(2026, 8, 8, 12, 0, 0, 0, time.UTC)
	return domain.Listing{
		ID: 100, SellerID: 42, CategoryID: 7, Title: "Neo 98", Description: "Used Neo 98", PriceIDR: 3_000_000, Quantity: 1,
		Condition: domain.ListingConditionUsed, Status: domain.ListingStatusActive, ModerationStatus: domain.ModerationStatusVisible,
		Category: domain.Category{ID: 7, Slug: "keyboard", Name: "Keyboard"},
		Seller:   domain.PublicUser{ID: 42, Handle: "seller", DisplayName: "Seller"}, CreatedAt: createdAt, UpdatedAt: createdAt,
	}
}
