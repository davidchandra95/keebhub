package httpapi_test

import (
	"context"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/davidchandra95/keebhub/internal/adapter/httpapi"
	"github.com/davidchandra95/keebhub/internal/app"
	"github.com/davidchandra95/keebhub/internal/domain"
)

type catalogHTTPFake struct {
	categories  []domain.Category
	listing     domain.Listing
	page        app.ListingPage
	createInput app.CreateListingInput
	updateInput app.UpdateListingInput
	status      domain.ListingStatus
	searchInput app.SearchListingsInput
	getErr      error
}

func (f *catalogHTTPFake) ListCategories(context.Context) ([]domain.Category, error) {
	return f.categories, nil
}

func (f *catalogHTTPFake) CreateListing(_ context.Context, _ int64, input app.CreateListingInput) (domain.Listing, error) {
	f.createInput = input
	if input.QuantitySet && input.Quantity == 0 {
		return domain.Listing{}, &domain.ValidationError{Fields: map[string]string{"quantity": "Quantity is invalid."}}
	}
	return f.listing, nil
}

func (f *catalogHTTPFake) UpdateListing(_ context.Context, _ int64, _ int64, input app.UpdateListingInput) (domain.Listing, error) {
	f.updateInput = input
	return f.listing, nil
}

func (f *catalogHTTPFake) ChangeListingStatus(_ context.Context, _ int64, _ int64, status domain.ListingStatus) (domain.Listing, error) {
	f.status = status
	listing := f.listing
	listing.Status = status
	return listing, nil
}

func (f *catalogHTTPFake) ListOwnedListings(context.Context, int64, app.ListOwnedListingsInput) (app.ListingPage, error) {
	return f.page, nil
}

func (f *catalogHTTPFake) GetListing(_ context.Context, _ int64, _ *int64) (domain.Listing, error) {
	if f.getErr != nil {
		return domain.Listing{}, f.getErr
	}
	return f.listing, nil
}

func (f *catalogHTTPFake) SearchListings(_ context.Context, input app.SearchListingsInput) (app.ListingPage, error) {
	f.searchInput = input
	return f.page, nil
}

func httpCatalogListing() domain.Listing {
	now := time.Date(2026, 8, 8, 12, 0, 0, 0, time.UTC)
	return domain.Listing{
		ID: 1001, SellerID: 42, Title: "Neo 98", Description: "Anodized silver\nUsed once.",
		PriceIDR: 3000000, Quantity: 1, Category: domain.Category{ID: 1, Slug: "keyboard", Name: "Keyboard"},
		Condition: domain.ConditionUsed, Status: domain.StatusReserved, Negotiable: true,
		Seller:    domain.PublicUser{ID: 42, Handle: "gunawan", DisplayName: "Gunawan"},
		CreatedAt: now, UpdatedAt: now,
	}
}

func newCatalogHTTPHandler(t *testing.T, catalog *catalogHTTPFake) http.Handler {
	t.Helper()
	auth := &fakeAuthenticator{authenticateUser: domain.User{ID: 42, Status: domain.UserStatusActive}}
	return newHandlerConfig(t, fakePinger{}, httpapi.Config{
		AppBaseURL: "http://localhost:8080",
		Auth:       auth,
		Catalog:    catalog,
	})
}

func authenticatedHeaders() map[string]string {
	return map[string]string{"Cookie": "keebhub_session=valid", "Origin": "http://localhost:8080"}
}

func TestCatalogCategoriesAndListingResponseUsePublicShapes(t *testing.T) {
	t.Parallel()

	listing := httpCatalogListing()
	catalog := &catalogHTTPFake{
		categories: []domain.Category{{ID: 1, Slug: "keyboard", Name: "Keyboard", SortOrder: 10, Active: true}},
		listing:    listing,
		page:       app.ListingPage{Items: []domain.Listing{listing}},
	}
	handler := newCatalogHTTPHandler(t, catalog)

	categories := performRequest(handler, http.MethodGet, "/api/v1/categories", "", nil)
	if categories.Code != http.StatusOK || !strings.Contains(categories.Body.String(), `"id":"1"`) {
		t.Fatalf("categories = %d %s", categories.Code, categories.Body.String())
	}
	search := performRequest(handler, http.MethodGet, "/api/v1/listings?q=Neo", "", nil)
	if search.Code != http.StatusOK || !strings.Contains(search.Body.String(), `"id":"1001"`) || strings.Contains(search.Body.String(), `"seller_id"`) {
		t.Fatalf("search = %d %s", search.Code, search.Body.String())
	}
	detail := performRequest(handler, http.MethodGet, "/api/v1/listings/1001", "", nil)
	if detail.Code != http.StatusOK || !strings.Contains(detail.Body.String(), `"created_at":"2026-08-08T12:00:00Z"`) {
		t.Fatalf("detail = %d %s", detail.Code, detail.Body.String())
	}
}

func TestCatalogListingWritesDecodeStrictly(t *testing.T) {
	t.Parallel()

	catalog := &catalogHTTPFake{listing: httpCatalogListing()}
	handler := newCatalogHTTPHandler(t, catalog)
	validBody := `{"title":"Neo 98","description":"line one\nline two","price_idr":3000000,"quantity":1,"category_slug":"keyboard","condition":"used","negotiable":true}`
	created := performRequest(handler, http.MethodPost, "/api/v1/listings", validBody, mergeHeaders(authenticatedHeaders(), map[string]string{"Content-Type": "application/json; charset=utf-8"}))
	if created.Code != http.StatusCreated {
		t.Fatalf("create = %d %s", created.Code, created.Body.String())
	}
	if catalog.createInput.Description != "line one\nline two" || !catalog.createInput.Negotiable {
		t.Errorf("create input = %+v", catalog.createInput)
	}

	tests := []struct {
		name        string
		method      string
		path        string
		body        string
		contentType string
		wantStatus  int
		wantCode    string
	}{
		{name: "unknown field", method: http.MethodPost, path: "/api/v1/listings", body: `{"title":"Neo","price_idr":1,"category_slug":"keyboard","condition":"new","extra":true}`, contentType: "application/json", wantStatus: http.StatusBadRequest, wantCode: "validation_failed"},
		{name: "explicit null", method: http.MethodPost, path: "/api/v1/listings", body: `{"title":null,"price_idr":1,"category_slug":"keyboard","condition":"new"}`, contentType: "application/json", wantStatus: http.StatusBadRequest, wantCode: "validation_failed"},
		{name: "zero quantity", method: http.MethodPost, path: "/api/v1/listings", body: `{"title":"Neo","price_idr":1,"quantity":0,"category_slug":"keyboard","condition":"new"}`, contentType: "application/json", wantStatus: http.StatusUnprocessableEntity, wantCode: "validation_failed"},
		{name: "multiple values", method: http.MethodPost, path: "/api/v1/listings", body: validBody + validBody, contentType: "application/json", wantStatus: http.StatusBadRequest, wantCode: "validation_failed"},
		{name: "content type", method: http.MethodPost, path: "/api/v1/listings", body: validBody, contentType: "text/plain", wantStatus: http.StatusUnsupportedMediaType, wantCode: "unsupported_media_type"},
		{name: "empty patch", method: http.MethodPatch, path: "/api/v1/listings/1001", body: `{}`, contentType: "application/json", wantStatus: http.StatusBadRequest, wantCode: "validation_failed"},
		{name: "null patch", method: http.MethodPatch, path: "/api/v1/listings/1001", body: `{"description":null}`, contentType: "application/json", wantStatus: http.StatusBadRequest, wantCode: "validation_failed"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			headers := mergeHeaders(authenticatedHeaders(), map[string]string{"Content-Type": tt.contentType})
			response := performRequest(handler, tt.method, tt.path, tt.body, headers)
			if response.Code != tt.wantStatus {
				t.Fatalf("status = %d, want %d: %s", response.Code, tt.wantStatus, response.Body.String())
			}
			assertErrorCode(t, response.Body.Bytes(), tt.wantCode)
		})
	}

	oversized := `{"title":"` + strings.Repeat("a", 33*1024) + `","price_idr":1,"category_slug":"keyboard","condition":"new"}`
	response := performRequest(handler, http.MethodPost, "/api/v1/listings", oversized, mergeHeaders(authenticatedHeaders(), map[string]string{"Content-Type": "application/json"}))
	if response.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("oversized status = %d, want 413", response.Code)
	}
}

func TestCatalogStatusAndPathValidation(t *testing.T) {
	t.Parallel()

	catalog := &catalogHTTPFake{listing: httpCatalogListing()}
	handler := newCatalogHTTPHandler(t, catalog)
	status := performRequest(handler, http.MethodPost, "/api/v1/listings/1001/status", `{"status":"sold"}`, mergeHeaders(authenticatedHeaders(), map[string]string{"Content-Type": "application/json"}))
	if status.Code != http.StatusOK || catalog.status != domain.StatusSold {
		t.Fatalf("status = %d %s, recorded = %q", status.Code, status.Body.String(), catalog.status)
	}
	malformed := performRequest(handler, http.MethodGet, "/api/v1/listings/not-an-id", "", nil)
	if malformed.Code != http.StatusNotFound {
		t.Fatalf("malformed listing ID status = %d", malformed.Code)
	}
	unauthenticated := performRequest(newHandlerConfig(t, fakePinger{}, httpapi.Config{
		AppBaseURL: "http://localhost:8080", Catalog: catalog,
	}), http.MethodPost, "/api/v1/listings", `{}`, mergeHeaders(map[string]string{}, map[string]string{"Content-Type": "application/json", "Origin": "http://localhost:8080"}))
	if unauthenticated.Code != http.StatusUnauthorized {
		t.Fatalf("unauthenticated create status = %d", unauthenticated.Code)
	}
}

func mergeHeaders(first, second map[string]string) map[string]string {
	merged := make(map[string]string, len(first)+len(second))
	for key, value := range first {
		merged[key] = value
	}
	for key, value := range second {
		merged[key] = value
	}
	return merged
}
