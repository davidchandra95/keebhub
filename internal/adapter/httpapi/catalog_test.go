package httpapi_test

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/davidchandra95/keebhub/internal/adapter/httpapi"
	"github.com/davidchandra95/keebhub/internal/app"
	"github.com/davidchandra95/keebhub/internal/domain"
)

type fakeCatalog struct {
	listCategoriesFn func() ([]domain.Category, error)
	createFn         func(domain.User, app.CreateListingInput) (domain.Listing, error)
	updateFn         func(domain.User, int64, app.UpdateListingInput) (domain.Listing, error)
	changeStatusFn   func(domain.User, int64, domain.ListingStatus) (app.ListingStatusChange, error)
	listOwnedFn      func(domain.User, app.OwnedListingsInput) (app.ListingPage, error)
	getFn            func(int64, *domain.User) (domain.Listing, error)
	searchFn         func(app.SearchListingsInput) (app.ListingPage, error)
}

func (f *fakeCatalog) ListCategories(context.Context) ([]domain.Category, error) {
	if f.listCategoriesFn == nil {
		return nil, errors.New("unexpected list categories")
	}
	return f.listCategoriesFn()
}

func (f *fakeCatalog) CreateListing(_ context.Context, seller domain.User, input app.CreateListingInput) (domain.Listing, error) {
	if f.createFn == nil {
		return domain.Listing{}, errors.New("unexpected create")
	}
	return f.createFn(seller, input)
}

func (f *fakeCatalog) UpdateListing(_ context.Context, seller domain.User, listingID int64, input app.UpdateListingInput) (domain.Listing, error) {
	if f.updateFn == nil {
		return domain.Listing{}, errors.New("unexpected update")
	}
	return f.updateFn(seller, listingID, input)
}

func (f *fakeCatalog) ChangeListingStatus(_ context.Context, seller domain.User, listingID int64, status domain.ListingStatus) (app.ListingStatusChange, error) {
	if f.changeStatusFn == nil {
		return app.ListingStatusChange{}, errors.New("unexpected status change")
	}
	return f.changeStatusFn(seller, listingID, status)
}

func (f *fakeCatalog) ListOwnedListings(_ context.Context, seller domain.User, input app.OwnedListingsInput) (app.ListingPage, error) {
	if f.listOwnedFn == nil {
		return app.ListingPage{}, errors.New("unexpected owned list")
	}
	return f.listOwnedFn(seller, input)
}

func (f *fakeCatalog) GetListing(_ context.Context, listingID int64, viewer *domain.User) (domain.Listing, error) {
	if f.getFn == nil {
		return domain.Listing{}, errors.New("unexpected get")
	}
	return f.getFn(listingID, viewer)
}

func (f *fakeCatalog) SearchListings(_ context.Context, input app.SearchListingsInput) (app.ListingPage, error) {
	if f.searchFn == nil {
		return app.ListingPage{}, errors.New("unexpected search")
	}
	return f.searchFn(input)
}

func TestCatalogCategoriesUsesItemsAndStringIDs(t *testing.T) {
	t.Parallel()

	catalog := &fakeCatalog{
		listCategoriesFn: func() ([]domain.Category, error) {
			return []domain.Category{{ID: 1, Slug: "keyboard", Name: "Keyboard"}}, nil
		},
	}
	response := performRequest(newCatalogHandler(t, catalog, nil), http.MethodGet, "/api/v1/categories", "", nil)
	if response.Code != http.StatusOK {
		t.Fatalf("status = %d: %s", response.Code, response.Body.String())
	}
	var body map[string]any
	if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode body: %v", err)
	}
	if _, found := body["categories"]; found {
		t.Errorf("unexpected categories response key: %s", response.Body.String())
	}
	items, ok := body["items"].([]any)
	if !ok || len(items) != 1 {
		t.Fatalf("items = %#v", body["items"])
	}
	if items[0].(map[string]any)["id"] != "1" {
		t.Errorf("category ID = %#v, want string", items[0])
	}
}

func TestCatalogCreateRequiresAuthenticationAndStrictJSON(t *testing.T) {
	t.Parallel()

	auth := &fakeAuthenticator{authenticateUser: domain.User{ID: 42, Status: domain.UserStatusActive}}
	catalog := &fakeCatalog{
		createFn: func(seller domain.User, input app.CreateListingInput) (domain.Listing, error) {
			if seller.ID != 42 || input.Description != "" || input.Quantity != 1 || input.Negotiable {
				t.Errorf("create input = %+v for seller %+v", input, seller)
			}
			return sampleListing(), nil
		},
	}
	handler := newCatalogHandler(t, catalog, auth)
	headers := authenticatedJSONHeaders()

	unauthenticated := performRequest(handler, http.MethodPost, "/api/v1/listings", `{"title":"Neo 98"}`, map[string]string{
		"Origin":       "http://localhost:8080",
		"Content-Type": "application/json",
	})
	if unauthenticated.Code != http.StatusUnauthorized {
		t.Fatalf("unauthenticated status = %d", unauthenticated.Code)
	}

	valid := performRequest(handler, http.MethodPost, "/api/v1/listings", `{"title":"Neo 98","price_idr":3000000,"category_slug":"keyboard","condition":"used"}`, headers)
	if valid.Code != http.StatusCreated {
		t.Fatalf("valid create = %d: %s", valid.Code, valid.Body.String())
	}
	assertResponseListingID(t, valid.Body.Bytes(), "1001")

	tests := []struct {
		name       string
		body       string
		headers    map[string]string
		wantStatus int
		wantCode   string
	}{
		{name: "missing content type", body: `{"title":"Neo 98"}`, headers: map[string]string{"Origin": "http://localhost:8080", "Cookie": "keebhub_session=valid"}, wantStatus: http.StatusUnsupportedMediaType, wantCode: "unsupported_media_type"},
		{name: "unknown field", body: `{"title":"Neo 98","price_idr":3000000,"category_slug":"keyboard","condition":"used","extra":true}`, headers: headers, wantStatus: http.StatusBadRequest, wantCode: "bad_request"},
		{name: "multiple values", body: `{"title":"Neo 98","price_idr":3000000,"category_slug":"keyboard","condition":"used"} {}`, headers: headers, wantStatus: http.StatusBadRequest, wantCode: "bad_request"},
		{name: "explicit null", body: `{"title":null,"price_idr":3000000,"category_slug":"keyboard","condition":"used"}`, headers: headers, wantStatus: http.StatusUnprocessableEntity, wantCode: "validation_failed"},
		{name: "empty body", body: "", headers: headers, wantStatus: http.StatusBadRequest, wantCode: "bad_request"},
		{name: "too large", body: `{"title":"` + strings.Repeat("a", 33<<10) + `","price_idr":3000000,"category_slug":"keyboard","condition":"used"}`, headers: headers, wantStatus: http.StatusRequestEntityTooLarge, wantCode: "request_too_large"},
	}
	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			response := performRequest(handler, http.MethodPost, "/api/v1/listings", test.body, test.headers)
			if response.Code != test.wantStatus {
				t.Fatalf("status = %d, want %d: %s", response.Code, test.wantStatus, response.Body.String())
			}
			assertErrorCode(t, response.Body.Bytes(), test.wantCode)
		})
	}
}

func TestCatalogPatchAndStatusValidation(t *testing.T) {
	t.Parallel()

	auth := &fakeAuthenticator{authenticateUser: domain.User{ID: 42, Status: domain.UserStatusActive}}
	catalog := &fakeCatalog{
		updateFn: func(_ domain.User, listingID int64, input app.UpdateListingInput) (domain.Listing, error) {
			if listingID != 1001 || input.Title == nil || *input.Title != "Updated" {
				t.Errorf("update = %d %+v", listingID, input)
			}
			return sampleListing(), nil
		},
		changeStatusFn: func(_ domain.User, listingID int64, status domain.ListingStatus) (app.ListingStatusChange, error) {
			if listingID != 1001 || status != domain.ListingStatusReserved {
				t.Errorf("status input = %d %q", listingID, status)
			}
			listing := sampleListing()
			listing.Status = domain.ListingStatusReserved
			return app.ListingStatusChange{Listing: listing, OldStatus: domain.ListingStatusActive, Changed: true}, nil
		},
	}
	handler := newCatalogHandler(t, catalog, auth)
	headers := authenticatedJSONHeaders()

	emptyPatch := performRequest(handler, http.MethodPatch, "/api/v1/listings/1001", `{}`, headers)
	if emptyPatch.Code != http.StatusUnprocessableEntity {
		t.Fatalf("empty patch = %d: %s", emptyPatch.Code, emptyPatch.Body.String())
	}
	nullPatch := performRequest(handler, http.MethodPatch, "/api/v1/listings/1001", `{"title":null}`, headers)
	if nullPatch.Code != http.StatusUnprocessableEntity {
		t.Fatalf("null patch = %d: %s", nullPatch.Code, nullPatch.Body.String())
	}
	validPatch := performRequest(handler, http.MethodPatch, "/api/v1/listings/1001", `{"title":"Updated"}`, headers)
	if validPatch.Code != http.StatusOK {
		t.Fatalf("valid patch = %d: %s", validPatch.Code, validPatch.Body.String())
	}
	badID := performRequest(handler, http.MethodPatch, "/api/v1/listings/not-an-id", `{"title":"Updated"}`, headers)
	if badID.Code != http.StatusNotFound {
		t.Fatalf("bad ID = %d: %s", badID.Code, badID.Body.String())
	}

	status := performRequest(handler, http.MethodPost, "/api/v1/listings/1001/status", `{"status":"reserved"}`, headers)
	if status.Code != http.StatusOK {
		t.Fatalf("status = %d: %s", status.Code, status.Body.String())
	}
	conflictCatalog := &fakeCatalog{
		changeStatusFn: func(domain.User, int64, domain.ListingStatus) (app.ListingStatusChange, error) {
			return app.ListingStatusChange{}, domain.ErrConflict
		},
	}
	conflict := performRequest(newCatalogHandler(t, conflictCatalog, auth), http.MethodPost, "/api/v1/listings/1001/status", `{"status":"sold"}`, headers)
	if conflict.Code != http.StatusConflict {
		t.Fatalf("conflict = %d: %s", conflict.Code, conflict.Body.String())
	}
}

func TestCatalogPublicDetailSearchAndOwnedPage(t *testing.T) {
	t.Parallel()

	listing := sampleListing()
	var viewer *domain.User
	var search app.SearchListingsInput
	var owned app.OwnedListingsInput
	auth := &fakeAuthenticator{authenticateUser: domain.User{ID: 42, Status: domain.UserStatusActive}}
	catalog := &fakeCatalog{
		getFn: func(_ int64, value *domain.User) (domain.Listing, error) {
			viewer = value
			return listing, nil
		},
		searchFn: func(input app.SearchListingsInput) (app.ListingPage, error) {
			search = input
			return app.ListingPage{Items: []domain.Listing{listing}}, nil
		},
		listOwnedFn: func(_ domain.User, input app.OwnedListingsInput) (app.ListingPage, error) {
			owned = input
			return app.ListingPage{Items: []domain.Listing{listing}}, nil
		},
	}
	handler := newCatalogHandler(t, catalog, auth)

	detail := performRequest(handler, http.MethodGet, "/api/v1/listings/1001", "", nil)
	if detail.Code != http.StatusOK || viewer != nil {
		t.Fatalf("detail = %d viewer=%+v", detail.Code, viewer)
	}
	searchResponse := performRequest(handler, http.MethodGet, "/api/v1/listings?q=Neo&category=keyboard&condition=used&min_price=1&max_price=4&sort=newest&limit=2", "", nil)
	if searchResponse.Code != http.StatusOK {
		t.Fatalf("search = %d: %s", searchResponse.Code, searchResponse.Body.String())
	}
	if search.Query != "Neo" || search.MinPrice == nil || *search.MinPrice != 1 || search.Limit != 2 {
		t.Errorf("search input = %+v", search)
	}
	if !strings.Contains(searchResponse.Body.String(), `"next_cursor":null`) {
		t.Errorf("search null cursor response = %s", searchResponse.Body.String())
	}

	ownedResponse := performRequest(handler, http.MethodGet, "/api/v1/me/listings?status=active&limit=1", "", map[string]string{
		"Cookie": "keebhub_session=valid",
	})
	if ownedResponse.Code != http.StatusOK {
		t.Fatalf("owned = %d: %s", ownedResponse.Code, ownedResponse.Body.String())
	}
	if owned.Status != "active" || owned.Limit != 1 {
		t.Errorf("owned input = %+v", owned)
	}
	invalidQueryCatalog := &fakeCatalog{
		searchFn: func(app.SearchListingsInput) (app.ListingPage, error) {
			return app.ListingPage{}, app.ErrInvalidCatalogQuery
		},
	}
	invalidQuery := performRequest(newCatalogHandler(t, invalidQueryCatalog, nil), http.MethodGet, "/api/v1/listings?cursor=bad", "", nil)
	if invalidQuery.Code != http.StatusBadRequest {
		t.Fatalf("invalid query = %d: %s", invalidQuery.Code, invalidQuery.Body.String())
	}
}

func newCatalogHandler(t *testing.T, catalog httpapi.Catalog, auth httpapi.Authenticator) http.Handler {
	t.Helper()
	return newHandlerConfig(t, fakePinger{}, httpapi.Config{
		AppBaseURL:        "http://localhost:8080",
		Auth:              auth,
		Catalog:           catalog,
		SessionCookieName: "keebhub_session",
		Now: func() time.Time {
			return time.Date(2026, 8, 8, 12, 0, 0, 0, time.UTC)
		},
	})
}

func authenticatedJSONHeaders() map[string]string {
	return map[string]string{
		"Origin":       "http://localhost:8080",
		"Cookie":       "keebhub_session=valid",
		"Content-Type": "application/json; charset=utf-8",
	}
}

func sampleListing() domain.Listing {
	return domain.Listing{
		ID:               1001,
		SellerID:         42,
		CategoryID:       1,
		Title:            "Neo 98",
		Description:      "Description",
		PriceIDR:         3_000_000,
		Quantity:         1,
		Category:         domain.Category{ID: 1, Slug: "keyboard", Name: "Keyboard"},
		Condition:        domain.ListingConditionUsed,
		Status:           domain.ListingStatusActive,
		ModerationStatus: domain.ModerationStatusVisible,
		Negotiable:       true,
		Seller:           domain.PublicUser{ID: 42, Handle: "seller", DisplayName: "Seller"},
		CreatedAt:        time.Date(2026, 8, 8, 12, 0, 0, 0, time.UTC),
		UpdatedAt:        time.Date(2026, 8, 8, 12, 0, 0, 0, time.UTC),
	}
}

func assertResponseListingID(t *testing.T, body []byte, want string) {
	t.Helper()
	var response struct {
		Listing struct {
			ID string `json:"id"`
		} `json:"listing"`
	}
	if err := json.Unmarshal(body, &response); err != nil {
		t.Fatalf("decode listing response: %v", err)
	}
	if response.Listing.ID != want {
		t.Errorf("listing ID = %q, want %q", response.Listing.ID, want)
	}
}
