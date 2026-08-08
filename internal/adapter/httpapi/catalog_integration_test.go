package httpapi_test

import (
	"encoding/json"
	"net/http"
	"net/url"
	"strings"
	"testing"

	"github.com/davidchandra95/keebhub/internal/adapter/httpapi"
	postgresadapter "github.com/davidchandra95/keebhub/internal/adapter/postgres"
	"github.com/davidchandra95/keebhub/internal/app"
	"github.com/davidchandra95/keebhub/internal/domain"
	"github.com/davidchandra95/keebhub/internal/testutil/testdatabase"
)

func TestCatalogHTTPFlowWithPostgreSQL(t *testing.T) {
	database := testdatabase.Open(t)
	store := postgresadapter.NewCatalogStore(database.Pool)
	auth := app.NewAuthService(integrationOAuth{identity: domain.DiscordIdentity{
		ID: "800000000000000001", Username: "catalog.integration", DisplayName: "Catalog Integration",
	}}, postgresadapter.NewAuthStore(database.Pool))
	catalog := app.NewCatalogService(store, store)
	handler := newHandlerConfig(t, database.Pool, httpapi.Config{
		AppBaseURL:        "http://localhost:8080",
		Auth:              auth,
		Catalog:           catalog,
		SessionCookieName: "keebhub_session",
	})

	session := completeIntegrationLogin(t, handler)
	create := performRequest(handler, http.MethodPost, "/api/v1/listings", `{"title":"Neo 98","description":"First description","price_idr":3000000,"category_slug":"keyboard","condition":"used","negotiable":true}`, map[string]string{
		"Cookie": session.Name + "=" + session.Value, "Origin": "http://localhost:8080", "Content-Type": "application/json; charset=utf-8",
	})
	if create.Code != http.StatusCreated {
		t.Fatalf("create = %d %s", create.Code, create.Body.String())
	}
	var created struct {
		Listing struct {
			ID string `json:"id"`
		} `json:"listing"`
	}
	if err := json.Unmarshal(create.Body.Bytes(), &created); err != nil {
		t.Fatalf("decode create: %v", err)
	}
	if created.Listing.ID == "" {
		t.Fatal("created listing ID is empty")
	}

	update := performRequest(handler, http.MethodPatch, "/api/v1/listings/"+created.Listing.ID, `{"price_idr":2900000,"description":"Edited description"}`, map[string]string{
		"Cookie": session.Name + "=" + session.Value, "Origin": "http://localhost:8080", "Content-Type": "application/json",
	})
	if update.Code != http.StatusOK || !strings.Contains(update.Body.String(), `"price_idr":2900000`) {
		t.Fatalf("update = %d %s", update.Code, update.Body.String())
	}
	status := performRequest(handler, http.MethodPost, "/api/v1/listings/"+created.Listing.ID+"/status", `{"status":"reserved"}`, map[string]string{
		"Cookie": session.Name + "=" + session.Value, "Origin": "http://localhost:8080", "Content-Type": "application/json",
	})
	if status.Code != http.StatusOK || !strings.Contains(status.Body.String(), `"status":"reserved"`) {
		t.Fatalf("status = %d %s", status.Code, status.Body.String())
	}

	owned := performRequest(handler, http.MethodGet, "/api/v1/me/listings", "", map[string]string{"Cookie": session.Name + "=" + session.Value})
	if owned.Code != http.StatusOK || !strings.Contains(owned.Body.String(), `"id":"`+created.Listing.ID+`"`) {
		t.Fatalf("owned = %d %s", owned.Code, owned.Body.String())
	}
	detail := performRequest(handler, http.MethodGet, "/api/v1/listings/"+created.Listing.ID, "", nil)
	if detail.Code != http.StatusOK || !strings.Contains(detail.Body.String(), `"price_idr":2900000`) || !strings.Contains(detail.Body.String(), `"status":"reserved"`) {
		t.Fatalf("anonymous detail = %d %s", detail.Code, detail.Body.String())
	}

	query := url.Values{
		"q": {"Neo 98"}, "category": {"keyboard"}, "condition": {"used"},
		"min_price": {"2900000"}, "max_price": {"2900000"}, "sort": {"price_asc"},
	}
	search := performRequest(handler, http.MethodGet, "/api/v1/listings?"+query.Encode(), "", nil)
	if search.Code != http.StatusOK || !strings.Contains(search.Body.String(), `"id":"`+created.Listing.ID+`"`) {
		t.Fatalf("search = %d %s", search.Code, search.Body.String())
	}
}
