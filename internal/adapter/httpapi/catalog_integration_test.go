package httpapi_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/url"
	"testing"

	"github.com/davidchandra95/keebhub/internal/adapter/httpapi"
	postgresadapter "github.com/davidchandra95/keebhub/internal/adapter/postgres"
	"github.com/davidchandra95/keebhub/internal/app"
	"github.com/davidchandra95/keebhub/internal/domain"
	"github.com/davidchandra95/keebhub/internal/testutil/testdatabase"
)

func TestCatalogHTTPFlowWithPostgreSQL(t *testing.T) {
	database := testdatabase.Open(t)
	oauth := integrationOAuth{identity: domain.DiscordIdentity{ID: "800000000000000001", Username: "catalog.seller", DisplayName: "Catalog Seller"}}
	auth := app.NewAuthService(oauth, postgresadapter.NewAuthStore(database.Pool))
	store := postgresadapter.NewCatalogStore(database.Pool)
	catalog := app.NewCatalogService(store, store, nil)
	handler := newHandlerConfig(t, database.Pool, httpapi.Config{
		AppBaseURL: "http://localhost:8080", Auth: auth, Catalog: catalog, SessionCookieName: "keebhub_session",
	})

	cookie := completeIntegrationLogin(t, handler)
	headers := map[string]string{
		"Origin": "http://localhost:8080", "Cookie": cookie.Name + "=" + cookie.Value, "Content-Type": "application/json",
	}
	created := performRequest(handler, http.MethodPost, "/api/v1/listings", `{"title":"Neo 98","price_idr":3000000,"category_slug":"keyboard","condition":"used","negotiable":true}`, headers)
	if created.Code != http.StatusCreated {
		t.Fatalf("create = %d %s", created.Code, created.Body.String())
	}
	listingID := responseListingID(t, created.Body.Bytes())

	updated := performRequest(handler, http.MethodPatch, "/api/v1/listings/"+listingID, `{"price_idr":2900000,"description":"Edited description"}`, headers)
	if updated.Code != http.StatusOK || !containsJSONField(t, updated.Body.Bytes(), "price_idr", float64(2900000)) {
		t.Fatalf("update = %d %s", updated.Code, updated.Body.String())
	}
	reserved := performRequest(handler, http.MethodPost, "/api/v1/listings/"+listingID+"/status", `{"status":"reserved"}`, headers)
	if reserved.Code != http.StatusOK || !containsJSONField(t, reserved.Body.Bytes(), "status", "reserved") {
		t.Fatalf("reserve = %d %s", reserved.Code, reserved.Body.String())
	}
	owned := performRequest(handler, http.MethodGet, "/api/v1/me/listings?status=reserved", "", map[string]string{"Cookie": cookie.Name + "=" + cookie.Value})
	if owned.Code != http.StatusOK || !containsJSONField(t, owned.Body.Bytes(), "id", listingID) {
		t.Fatalf("owned = %d %s", owned.Code, owned.Body.String())
	}
	detail := performRequest(handler, http.MethodGet, "/api/v1/listings/"+listingID, "", nil)
	if detail.Code != http.StatusOK || !containsJSONField(t, detail.Body.Bytes(), "description", "Edited description") {
		t.Fatalf("public detail = %d %s", detail.Code, detail.Body.String())
	}
	search := performRequest(handler, http.MethodGet, "/api/v1/listings?"+url.Values{
		"q": {"Neo"}, "category": {"keyboard"}, "condition": {"used"}, "min_price": {"2900000"}, "max_price": {"2900000"},
	}.Encode(), "", nil)
	if search.Code != http.StatusOK || !containsJSONField(t, search.Body.Bytes(), "id", listingID) {
		t.Fatalf("public search = %d %s", search.Code, search.Body.String())
	}

	var listingCount int
	if err := database.Pool.QueryRow(context.Background(), `SELECT count(*) FROM listings WHERE id = $1 AND status = 'reserved' AND price_idr = 2900000`, listingID).Scan(&listingCount); err != nil || listingCount != 1 {
		t.Fatalf("stored listing count = %d, error = %v", listingCount, err)
	}
}

func responseListingID(t *testing.T, body []byte) string {
	t.Helper()
	var response struct {
		Listing struct {
			ID string `json:"id"`
		} `json:"listing"`
	}
	if err := json.Unmarshal(body, &response); err != nil || response.Listing.ID == "" {
		t.Fatalf("decode listing ID: %v, %s", err, body)
	}
	return response.Listing.ID
}

func containsJSONField(t *testing.T, body []byte, field string, want any) bool {
	t.Helper()
	var value any
	if err := json.Unmarshal(body, &value); err != nil {
		t.Fatalf("decode JSON: %v", err)
	}
	return findJSONField(value, field, want)
}

func findJSONField(value any, field string, want any) bool {
	switch typed := value.(type) {
	case map[string]any:
		if typed[field] == want {
			return true
		}
		for _, child := range typed {
			if findJSONField(child, field, want) {
				return true
			}
		}
	case []any:
		for _, child := range typed {
			if findJSONField(child, field, want) {
				return true
			}
		}
	}
	return false
}
