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

func TestSellerProfileHTTPFlowWithPostgreSQL(t *testing.T) {
	database := testdatabase.Open(t)
	oauth := integrationOAuth{identity: domain.DiscordIdentity{ID: "800000000000000101", Username: "seller.profile", DisplayName: "Seller Profile"}}
	auth := app.NewAuthService(oauth, postgresadapter.NewAuthStore(database.Pool))
	catalogStore := postgresadapter.NewCatalogStore(database.Pool)
	sellerStore := postgresadapter.NewSellerStore(database.Pool)
	handler := newHandlerConfig(t, database.Pool, httpapi.Config{
		AppBaseURL: "http://localhost:8080", Auth: auth,
		Catalog:           app.NewCatalogService(catalogStore, catalogStore, nil),
		Seller:            app.NewSellerService(sellerStore, sellerStore, nil),
		SessionCookieName: "keebhub_session",
	})

	cookie := completeIntegrationLogin(t, handler)
	headers := map[string]string{
		"Origin": "http://localhost:8080", "Cookie": cookie.Name + "=" + cookie.Value, "Content-Type": "application/json",
	}
	updated := performRequest(handler, http.MethodPatch, "/api/v1/me", `{"location":" Jakarta Barat ","bio":"Keyboard enthusiast"}`, headers)
	if updated.Code != http.StatusOK {
		t.Fatalf("profile update = %d %s", updated.Code, updated.Body.String())
	}
	me := performRequest(handler, http.MethodGet, "/api/v1/me", "", map[string]string{"Cookie": cookie.Name + "=" + cookie.Value})
	var meBody struct {
		User struct {
			Location  *string `json:"location"`
			CreatedAt string  `json:"created_at"`
		} `json:"user"`
	}
	if err := json.Unmarshal(me.Body.Bytes(), &meBody); err != nil || meBody.User.Location == nil || *meBody.User.Location != "Jakarta Barat" || meBody.User.CreatedAt == "" {
		t.Fatalf("current user = %v %s", err, me.Body.String())
	}

	createSellerHTTPListing(t, handler, headers, "Active")
	reserved := createSellerHTTPListing(t, handler, headers, "Reserved")
	sold := createSellerHTTPListing(t, handler, headers, "Sold")
	for _, update := range []struct {
		id     string
		status string
	}{{reserved, "reserved"}, {sold, "sold"}} {
		response := performRequest(handler, http.MethodPost, "/api/v1/listings/"+update.id+"/status", `{"status":"`+update.status+`"}`, headers)
		if response.Code != http.StatusOK {
			t.Fatalf("set %s = %d %s", update.status, response.Code, response.Body.String())
		}
	}

	profile := performRequest(handler, http.MethodGet, "/api/v1/users/seller-profile", "", nil)
	var profileBody struct {
		User struct {
			ActiveListingCount int64 `json:"active_listing_count"`
		} `json:"user"`
	}
	if err := json.Unmarshal(profile.Body.Bytes(), &profileBody); err != nil || profile.Code != http.StatusOK || profileBody.User.ActiveListingCount != 1 {
		t.Fatalf("public profile = %d %v %s", profile.Code, err, profile.Body.String())
	}
	defaultPage := performRequest(handler, http.MethodGet, "/api/v1/users/seller-profile/listings?"+url.Values{"limit": {"20"}}.Encode(), "", nil)
	if defaultPage.Code != http.StatusOK || !sellerListingStatuses(t, defaultPage.Body.Bytes(), "active", "reserved") {
		t.Fatalf("default catalog = %d %s", defaultPage.Code, defaultPage.Body.String())
	}
	soldPage := performRequest(handler, http.MethodGet, "/api/v1/users/seller-profile/listings?status=sold", "", nil)
	if soldPage.Code != http.StatusOK || !sellerListingStatuses(t, soldPage.Body.Bytes(), "sold") {
		t.Fatalf("sold catalog = %d %s", soldPage.Code, soldPage.Body.String())
	}

	cleared := performRequest(handler, http.MethodPatch, "/api/v1/me", `{"location":" \t ","bio":null}`, headers)
	if cleared.Code != http.StatusOK || !strings.Contains(cleared.Body.String(), `"location":null`) || !strings.Contains(cleared.Body.String(), `"bio":null`) {
		t.Fatalf("cleared profile = %d %s", cleared.Code, cleared.Body.String())
	}
}

func createSellerHTTPListing(t *testing.T, handler http.Handler, headers map[string]string, title string) string {
	t.Helper()
	response := performRequest(handler, http.MethodPost, "/api/v1/listings", `{"title":"`+title+`","price_idr":100,"category_slug":"keyboard","condition":"used"}`, headers)
	if response.Code != http.StatusCreated {
		t.Fatalf("create %s = %d %s", title, response.Code, response.Body.String())
	}
	return responseListingID(t, response.Body.Bytes())
}

func sellerListingStatuses(t *testing.T, body []byte, want ...string) bool {
	t.Helper()
	var page struct {
		Items []struct {
			Status string `json:"status"`
		} `json:"items"`
	}
	if err := json.Unmarshal(body, &page); err != nil || len(page.Items) != len(want) {
		return false
	}
	for index, status := range want {
		if page.Items[index].Status != status {
			return false
		}
	}
	return true
}
