package postgres_test

import (
	"context"
	"errors"
	"testing"
	"time"

	postgresadapter "github.com/davidchandra95/keebhub/internal/adapter/postgres"
	"github.com/davidchandra95/keebhub/internal/app"
	"github.com/davidchandra95/keebhub/internal/domain"
	"github.com/davidchandra95/keebhub/internal/testutil/testdatabase"
)

func TestSellerStoreProfileAndCatalog(t *testing.T) {
	database := testdatabase.Open(t)
	ctx := context.Background()
	sellerID := insertCatalogUser(t, ctx, database, "700000000000000101", "seller-profile")
	store := postgresadapter.NewSellerStore(database.Pool)
	now := time.Date(2026, 8, 9, 12, 0, 0, 0, time.UTC)
	service := app.NewSellerService(store, store, func() time.Time { return now })
	user := domain.User{ID: sellerID, Handle: "seller-profile", Status: domain.UserStatusActive}

	updated, err := service.UpdateProfile(ctx, user, app.UpdateProfileInput{
		Location: profileValue(" Jakarta Barat "),
		Bio:      profileValue(" Keyboard enthusiast\nwith vintage switches "),
	})
	if err != nil || updated.Location == nil || *updated.Location != "Jakarta Barat" || updated.Bio == nil || *updated.Bio != "Keyboard enthusiast\nwith vintage switches" || !updated.UpdatedAt.Equal(now) {
		t.Fatalf("UpdateProfile() = %+v, %v", updated, err)
	}
	updatedAt := updated.UpdatedAt
	unchanged, err := service.UpdateProfile(ctx, updated, app.UpdateProfileInput{Location: profileValue("Jakarta Barat")})
	if err != nil || !unchanged.UpdatedAt.Equal(updatedAt) {
		t.Fatalf("no-op UpdateProfile() = %+v, %v", unchanged, err)
	}

	catalog := app.NewCatalogService(postgresadapter.NewCatalogStore(database.Pool), postgresadapter.NewCatalogStore(database.Pool), func() time.Time { return now })
	activeEarlier := createCatalogListing(t, ctx, catalog, sellerID, "Active earlier", 100, "keyboard")
	activeLater := createCatalogListing(t, ctx, catalog, sellerID, "Active later", 100, "keyboard")
	reserved := createCatalogListing(t, ctx, catalog, sellerID, "Reserved", 100, "keyboard")
	sold := createCatalogListing(t, ctx, catalog, sellerID, "Sold", 100, "keycaps")
	archived := createCatalogListing(t, ctx, catalog, sellerID, "Archived", 100, "keyboard")
	removed := createCatalogListing(t, ctx, catalog, sellerID, "Removed", 100, "keyboard")
	for _, update := range []struct {
		id         int64
		status     string
		moderation string
	}{
		{id: reserved.ID, status: "reserved", moderation: "visible"},
		{id: sold.ID, status: "sold", moderation: "visible"},
		{id: archived.ID, status: "archived", moderation: "visible"},
		{id: removed.ID, status: "active", moderation: "removed"},
	} {
		if _, err := database.Pool.Exec(ctx, `UPDATE listings SET status = $2, moderation_status = $3, updated_at = $4 WHERE id = $1`, update.id, update.status, update.moderation, now); err != nil {
			t.Fatalf("set listing state: %v", err)
		}
	}

	profile, err := service.GetSellerProfile(ctx, "seller-profile")
	if err != nil || profile.ActiveListingCount != 2 || profile.User.Location == nil || *profile.User.Location != "Jakarta Barat" {
		t.Fatalf("GetSellerProfile() = %+v, %v", profile, err)
	}
	page, err := service.ListSellerListings(ctx, "seller-profile", app.SellerListingOptions{Limit: 2})
	if err != nil || len(page.Items) != 2 || page.NextCursor == nil || page.Items[0].ID != activeLater.ID || page.Items[1].ID != activeEarlier.ID {
		t.Fatalf("default seller page = %+v, %v", page, err)
	}
	next, err := service.ListSellerListings(ctx, "seller-profile", app.SellerListingOptions{Limit: 2, Cursor: *page.NextCursor})
	if err != nil || len(next.Items) != 1 || next.Items[0].ID != reserved.ID {
		t.Fatalf("next seller page = %+v, %v", next, err)
	}
	soldStatus := domain.ListingStatusSold
	soldPage, err := service.ListSellerListings(ctx, "seller-profile", app.SellerListingOptions{Status: &soldStatus, Limit: 20})
	if err != nil || len(soldPage.Items) != 1 || soldPage.Items[0].ID != sold.ID {
		t.Fatalf("sold seller page = %+v, %v", soldPage, err)
	}
	unknownCategory, err := service.ListSellerListings(ctx, "seller-profile", app.SellerListingOptions{Category: "unknown", Limit: 20})
	if err != nil || len(unknownCategory.Items) != 0 {
		t.Fatalf("unknown category page = %+v, %v", unknownCategory, err)
	}

	if _, err := database.Pool.Exec(ctx, `UPDATE users SET status = 'disabled' WHERE id = $1`, sellerID); err != nil {
		t.Fatalf("disable seller: %v", err)
	}
	if _, err := service.UpdateProfile(ctx, updated, app.UpdateProfileInput{Location: profileValue("Bandung")}); !errors.Is(err, domain.ErrUserDisabled) {
		t.Fatalf("disabled UpdateProfile() error = %v", err)
	}
	if profile, err := service.GetSellerProfile(ctx, "seller-profile"); err != nil || profile.User.ID != sellerID {
		t.Fatalf("disabled public seller profile = %+v, %v", profile, err)
	}
	if page, err := service.ListSellerListings(ctx, "seller-profile", app.SellerListingOptions{Limit: 20}); err != nil || len(page.Items) != 3 {
		t.Fatalf("disabled public seller catalog = %+v, %v", page, err)
	}
}

func profileValue(value string) app.ProfileField {
	return app.ProfileField{Present: true, Value: &value}
}
