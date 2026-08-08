package httpapi_test

import (
	"context"
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/davidchandra95/keebhub/internal/adapter/httpapi"
	postgresadapter "github.com/davidchandra95/keebhub/internal/adapter/postgres"
	"github.com/davidchandra95/keebhub/internal/app"
	"github.com/davidchandra95/keebhub/internal/domain"
	"github.com/davidchandra95/keebhub/internal/testutil/testdatabase"
)

func TestCatalogHTTPFlowWithPostgreSQL(t *testing.T) {
	database := testdatabase.Open(t)
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	authStore := postgresadapter.NewAuthStore(database.Pool)
	auth := app.NewAuthService(integrationOAuth{identity: domain.DiscordIdentity{
		ID: "800000000000000001", Username: "catalog.integration", DisplayName: "Catalog Integration",
	}}, authStore)
	catalogStore := postgresadapter.NewCatalogStore(database.Pool)
	catalog := app.NewCatalogService(catalogStore, catalogStore, time.Now)
	handler := newHandlerConfig(t, database.Pool, httpapi.Config{
		AppBaseURL:        "http://localhost:8080",
		Auth:              auth,
		Catalog:           catalog,
		SessionCookieName: "keebhub_session",
	})

	session := completeIntegrationLogin(t, handler)
	headers := catalogJSONHeaders(session)
	categories := performRequest(handler, http.MethodGet, "/api/v1/categories", "", nil)
	if categories.Code != http.StatusOK || !strings.Contains(categories.Body.String(), `"slug":"keyboard"`) {
		t.Fatalf("categories = %d %s", categories.Code, categories.Body.String())
	}

	created := performRequest(handler, http.MethodPost, "/api/v1/listings", `{
		"title":"Neo 98",
		"description":"Original description",
		"price_idr":3000000,
		"quantity":1,
		"category_slug":"keyboard",
		"condition":"used"
	}`, headers)
	if created.Code != http.StatusCreated {
		t.Fatalf("create listing = %d %s", created.Code, created.Body.String())
	}
	listing := decodeCatalogHTTPListing(t, created.Body.Bytes())
	listingID, err := strconv.ParseInt(listing.ID, 10, 64)
	if err != nil || listingID <= 0 {
		t.Fatalf("listing ID = %q, error = %v", listing.ID, err)
	}
	if listing.Status != "active" || listing.Category.ID == "" || listing.Seller.ID == "" {
		t.Fatalf("created listing response = %+v", listing)
	}

	patched := performRequest(handler, http.MethodPatch, "/api/v1/listings/"+listing.ID, `{
		"description":"Edited description",
		"price_idr":2900000
	}`, headers)
	if patched.Code != http.StatusOK {
		t.Fatalf("patch listing = %d %s", patched.Code, patched.Body.String())
	}
	updated := decodeCatalogHTTPListing(t, patched.Body.Bytes())
	if updated.Description != "Edited description" || updated.PriceIDR != 2_900_000 || updated.Title != "Neo 98" {
		t.Errorf("patched listing = %+v", updated)
	}

	otherAuth := app.NewAuthService(integrationOAuth{identity: domain.DiscordIdentity{
		ID: "800000000000000002", Username: "other.catalog", DisplayName: "Other Catalog",
	}}, authStore)
	otherLogin, err := otherAuth.LoginWithDiscord(ctx, "other-code", "")
	if err != nil {
		t.Fatalf("create other user session: %v", err)
	}
	crossSeller := performRequest(handler, http.MethodPatch, "/api/v1/listings/"+listing.ID, `{"title":"Nope"}`, catalogJSONHeaders(&http.Cookie{
		Name: "keebhub_session", Value: otherLogin.RawToken,
	}))
	if crossSeller.Code != http.StatusForbidden {
		t.Fatalf("cross-seller patch = %d %s", crossSeller.Code, crossSeller.Body.String())
	}

	reserved := performRequest(handler, http.MethodPost, "/api/v1/listings/"+listing.ID+"/status", `{"status":"reserved"}`, headers)
	if reserved.Code != http.StatusOK {
		t.Fatalf("reserve listing = %d %s", reserved.Code, reserved.Body.String())
	}
	reservedListing := decodeCatalogHTTPListing(t, reserved.Body.Bytes())
	if reservedListing.Status != "reserved" {
		t.Errorf("reserved status = %q", reservedListing.Status)
	}
	sameStatus := performRequest(handler, http.MethodPost, "/api/v1/listings/"+listing.ID+"/status", `{"status":"reserved"}`, headers)
	if sameStatus.Code != http.StatusOK {
		t.Fatalf("same status = %d %s", sameStatus.Code, sameStatus.Body.String())
	}
	if got := decodeCatalogHTTPListing(t, sameStatus.Body.Bytes()); got.UpdatedAt != reservedListing.UpdatedAt {
		t.Errorf("same status updated_at = %q, want %q", got.UpdatedAt, reservedListing.UpdatedAt)
	}

	owned := performRequest(handler, http.MethodGet, "/api/v1/me/listings?status=reserved", "", map[string]string{
		"Cookie": session.Name + "=" + session.Value,
	})
	if owned.Code != http.StatusOK || !strings.Contains(owned.Body.String(), `"id":"`+listing.ID+`"`) || !strings.Contains(owned.Body.String(), `"next_cursor":null`) {
		t.Fatalf("owner page = %d %s", owned.Code, owned.Body.String())
	}

	publicDetail := performRequest(handler, http.MethodGet, "/api/v1/listings/"+listing.ID, "", nil)
	if publicDetail.Code != http.StatusOK {
		t.Fatalf("public detail = %d %s", publicDetail.Code, publicDetail.Body.String())
	}
	publicListing := decodeCatalogHTTPListing(t, publicDetail.Body.Bytes())
	if publicListing.Status != "reserved" || publicListing.Description != "Edited description" {
		t.Errorf("public detail = %+v", publicListing)
	}

	for _, sort := range []string{"newest", "price_asc", "price_desc"} {
		search := performRequest(handler, http.MethodGet, "/api/v1/listings?q=Neo&category=keyboard&condition=used&min_price=2000000&max_price=3000000&sort="+sort, "", nil)
		if search.Code != http.StatusOK || !strings.Contains(search.Body.String(), `"id":"`+listing.ID+`"`) {
			t.Fatalf("public %s search = %d %s", sort, search.Code, search.Body.String())
		}
	}
	invalidSearch := performRequest(handler, http.MethodGet, "/api/v1/listings?min_price=not-a-number", "", nil)
	if invalidSearch.Code != http.StatusBadRequest {
		t.Fatalf("invalid search = %d %s", invalidSearch.Code, invalidSearch.Body.String())
	}

	archivedCreated := performRequest(handler, http.MethodPost, "/api/v1/listings", `{
		"title":"Archive me",
		"price_idr":1000000,
		"category_slug":"keyboard",
		"condition":"new"
	}`, headers)
	if archivedCreated.Code != http.StatusCreated {
		t.Fatalf("create archived listing = %d %s", archivedCreated.Code, archivedCreated.Body.String())
	}
	archivedID := decodeCatalogHTTPListing(t, archivedCreated.Body.Bytes()).ID
	archivedStatus := performRequest(handler, http.MethodPost, "/api/v1/listings/"+archivedID+"/status", `{"status":"archived"}`, headers)
	if archivedStatus.Code != http.StatusOK {
		t.Fatalf("archive listing = %d %s", archivedStatus.Code, archivedStatus.Body.String())
	}
	archivedAnonymous := performRequest(handler, http.MethodGet, "/api/v1/listings/"+archivedID, "", nil)
	if archivedAnonymous.Code != http.StatusNotFound {
		t.Fatalf("anonymous archived detail = %d %s", archivedAnonymous.Code, archivedAnonymous.Body.String())
	}
	archivedOwner := performRequest(handler, http.MethodGet, "/api/v1/listings/"+archivedID, "", map[string]string{
		"Cookie": session.Name + "=" + session.Value,
	})
	if archivedOwner.Code != http.StatusOK {
		t.Fatalf("owner archived detail = %d %s", archivedOwner.Code, archivedOwner.Body.String())
	}

	if _, err := database.Pool.Exec(ctx, `UPDATE listings SET moderation_status = 'removed' WHERE id = $1`, listingID); err != nil {
		t.Fatalf("remove listing: %v", err)
	}
	removedOwner := performRequest(handler, http.MethodGet, "/api/v1/listings/"+listing.ID, "", map[string]string{
		"Cookie": session.Name + "=" + session.Value,
	})
	if removedOwner.Code != http.StatusNotFound {
		t.Fatalf("owner removed detail = %d %s", removedOwner.Code, removedOwner.Body.String())
	}
	removedSearch := performRequest(handler, http.MethodGet, "/api/v1/listings?q=Neo", "", nil)
	if removedSearch.Code != http.StatusOK || strings.Contains(removedSearch.Body.String(), `"id":"`+listing.ID+`"`) {
		t.Fatalf("removed public search = %d %s", removedSearch.Code, removedSearch.Body.String())
	}
}

type catalogHTTPListing struct {
	ID          string `json:"id"`
	Title       string `json:"title"`
	Description string `json:"description"`
	PriceIDR    int64  `json:"price_idr"`
	Status      string `json:"status"`
	UpdatedAt   string `json:"updated_at"`
	Category    struct {
		ID string `json:"id"`
	} `json:"category"`
	Seller struct {
		ID string `json:"id"`
	} `json:"seller"`
}

func decodeCatalogHTTPListing(t *testing.T, body []byte) catalogHTTPListing {
	t.Helper()
	var response struct {
		Listing catalogHTTPListing `json:"listing"`
	}
	if err := json.Unmarshal(body, &response); err != nil {
		t.Fatalf("decode listing response: %v (%s)", err, body)
	}
	return response.Listing
}

func catalogJSONHeaders(session *http.Cookie) map[string]string {
	return map[string]string{
		"Origin":       "http://localhost:8080",
		"Content-Type": "application/json; charset=utf-8",
		"Cookie":       session.Name + "=" + session.Value,
	}
}
