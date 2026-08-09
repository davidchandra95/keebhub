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

func TestSellerProfileEndpoints(t *testing.T) {
	t.Parallel()

	createdAt := time.Date(2026, 8, 9, 10, 0, 0, 0, time.UTC)
	service := &sellerHTTPFake{profile: domain.SellerProfile{
		User: domain.PublicUser{ID: 42, Handle: "seller-one", DisplayName: "Seller One"}, CreatedAt: createdAt, ActiveListingCount: 2,
	}}
	handler := sellerAuthenticatedHandler(t, service, createdAt)
	headers := map[string]string{"Origin": "http://localhost:8080", "Cookie": "keebhub_session=valid", "Content-Type": "application/json"}

	me := performRequest(handler, http.MethodGet, "/api/v1/me", "", map[string]string{"Cookie": "keebhub_session=valid"})
	if me.Code != http.StatusOK || !strings.Contains(me.Body.String(), `"created_at":"2026-08-09T10:00:00Z"`) {
		t.Fatalf("me = %d %s", me.Code, me.Body.String())
	}

	updated := performRequest(handler, http.MethodPatch, "/api/v1/me", `{"location":null,"bio":" Keyboard enthusiast "}`, headers)
	if updated.Code != http.StatusOK || !service.updateInput.Location.Present || service.updateInput.Location.Value != nil || !service.updateInput.Bio.Present || service.updateInput.Bio.Value == nil || *service.updateInput.Bio.Value != " Keyboard enthusiast " {
		t.Fatalf("update = %d %s input=%+v", updated.Code, updated.Body.String(), service.updateInput)
	}

	profile := performRequest(handler, http.MethodGet, "/api/v1/users/seller-one", "", nil)
	if profile.Code != http.StatusOK || !strings.Contains(profile.Body.String(), `"active_listing_count":2`) || !strings.Contains(profile.Body.String(), `"id":"42"`) {
		t.Fatalf("profile = %d %s", profile.Code, profile.Body.String())
	}

	list := performRequest(handler, http.MethodGet, "/api/v1/users/seller-one/listings?status=sold&category=keyboard&limit=10", "", nil)
	if list.Code != http.StatusOK || service.listOptions.Status == nil || *service.listOptions.Status != domain.ListingStatusSold || service.listOptions.Category != "keyboard" || service.listOptions.Limit != 10 {
		t.Fatalf("list = %d %s options=%+v", list.Code, list.Body.String(), service.listOptions)
	}
}

func TestProfileWriteRejectsMalformedBodiesAndUnauthorizedRequests(t *testing.T) {
	t.Parallel()

	service := &sellerHTTPFake{}
	handler := sellerAuthenticatedHandler(t, service, time.Date(2026, 8, 9, 10, 0, 0, 0, time.UTC))
	baseHeaders := map[string]string{"Origin": "http://localhost:8080", "Cookie": "keebhub_session=valid", "Content-Type": "application/json"}
	tests := []struct {
		name    string
		body    string
		headers map[string]string
		want    int
	}{
		{name: "empty object", body: `{}`, headers: baseHeaders, want: http.StatusBadRequest},
		{name: "empty body", body: ``, headers: baseHeaders, want: http.StatusBadRequest},
		{name: "top level null", body: `null`, headers: baseHeaders, want: http.StatusBadRequest},
		{name: "unknown field", body: `{"handle":"new"}`, headers: baseHeaders, want: http.StatusBadRequest},
		{name: "multiple values", body: `{"bio":"one"}{"bio":"two"}`, headers: baseHeaders, want: http.StatusBadRequest},
		{name: "invalid JSON", body: `{"bio":`, headers: baseHeaders, want: http.StatusBadRequest},
		{name: "wrong content type", body: `{"bio":"one"}`, headers: map[string]string{"Origin": "http://localhost:8080", "Cookie": "keebhub_session=valid", "Content-Type": "text/plain"}, want: http.StatusUnsupportedMediaType},
		{name: "too large", body: `{"bio":"` + strings.Repeat("x", 32<<10) + `"}`, headers: baseHeaders, want: http.StatusRequestEntityTooLarge},
		{name: "missing session", body: `{"bio":"one"}`, headers: map[string]string{"Origin": "http://localhost:8080", "Content-Type": "application/json"}, want: http.StatusUnauthorized},
		{name: "cross origin", body: `{"bio":"one"}`, headers: map[string]string{"Origin": "https://evil.example", "Cookie": "keebhub_session=valid", "Content-Type": "application/json"}, want: http.StatusForbidden},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			response := performRequest(handler, http.MethodPatch, "/api/v1/me", tt.body, tt.headers)
			if response.Code != tt.want {
				t.Fatalf("status = %d, want %d: %s", response.Code, tt.want, response.Body.String())
			}
		})
	}
}

func TestSellerEndpointErrorMapping(t *testing.T) {
	t.Parallel()

	service := &sellerHTTPFake{profileErr: domain.ErrNotFound, updateErr: domain.ErrUserDisabled}
	handler := sellerAuthenticatedHandler(t, service, time.Now())
	update := performRequest(handler, http.MethodPatch, "/api/v1/me", `{"bio":"one"}`, map[string]string{"Origin": "http://localhost:8080", "Cookie": "keebhub_session=valid", "Content-Type": "application/json"})
	if update.Code != http.StatusForbidden || !strings.Contains(update.Body.String(), `"account_disabled"`) {
		t.Fatalf("disabled update = %d %s", update.Code, update.Body.String())
	}
	profile := performRequest(handler, http.MethodGet, "/api/v1/users/bad", "", nil)
	if profile.Code != http.StatusNotFound || !strings.Contains(profile.Body.String(), `"seller_not_found"`) {
		t.Fatalf("unknown profile = %d %s", profile.Code, profile.Body.String())
	}
}

type sellerHTTPFake struct {
	profile     domain.SellerProfile
	page        app.ListingPage
	updateInput app.UpdateProfileInput
	listOptions app.SellerListingOptions
	updateErr   error
	profileErr  error
	listErr     error
}

func (f *sellerHTTPFake) UpdateProfile(_ context.Context, user domain.User, input app.UpdateProfileInput) (domain.User, error) {
	f.updateInput = input
	if f.updateErr != nil {
		return domain.User{}, f.updateErr
	}
	return user, nil
}

func (f *sellerHTTPFake) GetSellerProfile(context.Context, string) (domain.SellerProfile, error) {
	if f.profileErr != nil {
		return domain.SellerProfile{}, f.profileErr
	}
	return f.profile, nil
}

func (f *sellerHTTPFake) ListSellerListings(_ context.Context, _ string, options app.SellerListingOptions) (app.ListingPage, error) {
	f.listOptions = options
	return f.page, f.listErr
}

func sellerAuthenticatedHandler(t *testing.T, seller httpapi.SellerService, createdAt time.Time) http.Handler {
	t.Helper()
	return newHandlerConfig(t, fakePinger{}, httpapi.Config{
		AppBaseURL: "http://localhost:8080",
		Auth: &fakeAuthenticator{authenticateUser: domain.User{
			ID: 42, Handle: "seller-one", DiscordUsername: "seller-one", DisplayName: "Seller One", Status: domain.UserStatusActive, CreatedAt: createdAt,
		}},
		Seller: seller, SessionCookieName: "keebhub_session",
	})
}
