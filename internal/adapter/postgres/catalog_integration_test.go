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

func TestCatalogStoreLifecycleAndSearch(t *testing.T) {
	database := testdatabase.Open(t)
	ctx := context.Background()
	sellerID := insertCatalogUser(t, ctx, database, "700000000000000001", "seller-one")
	store := postgresadapter.NewCatalogStore(database.Pool)
	now := time.Date(2026, 8, 8, 12, 0, 0, 0, time.UTC)
	service := app.NewCatalogService(store, store, func() time.Time { return now })

	categories, err := service.ListCategories(ctx)
	if err != nil {
		t.Fatalf("ListCategories() error = %v", err)
	}
	wantCategories := []string{"keyboard", "keycaps", "switches", "parts", "accessories", "other"}
	if len(categories) != len(wantCategories) {
		t.Fatalf("categories = %+v", categories)
	}
	for index, slug := range wantCategories {
		if categories[index].Slug != slug || categories[index].SortOrder != int32((index+1)*10) {
			t.Errorf("category %d = %+v, want %q at %d", index, categories[index], slug, (index+1)*10)
		}
	}

	first := createCatalogListing(t, ctx, service, sellerID, "100%_ Neo 98", 3_000_000, "keyboard")
	second := createCatalogListing(t, ctx, service, sellerID, "Keycap set", 2_000_000, "keycaps")
	third := createCatalogListing(t, ctx, service, sellerID, "Second Neo", 3_000_000, "keyboard")
	if first.ID == 0 || first.Seller.Handle != "seller-one" || first.Category.Slug != "keyboard" || first.Status != domain.ListingStatusActive {
		t.Errorf("created listing = %+v", first)
	}

	description := "Updated\npreserving line breaks"
	updated, err := service.UpdateListing(ctx, sellerID, first.ID, app.UpdateListingInput{Description: &description})
	if err != nil || updated.Description != description || !updated.UpdatedAt.Equal(now) {
		t.Fatalf("UpdateListing() = %+v, %v", updated, err)
	}
	reserved, err := service.ChangeListingStatus(ctx, sellerID, second.ID, domain.ListingStatusReserved)
	if err != nil || reserved.Status != domain.ListingStatusReserved {
		t.Fatalf("ChangeListingStatus() = %+v, %v", reserved, err)
	}

	owned, err := service.ListOwnedListings(ctx, sellerID, app.OwnedListingOptions{Limit: 2})
	if err != nil || len(owned.Items) != 2 || owned.NextCursor == nil {
		t.Fatalf("ListOwnedListings() = %+v, %v", owned, err)
	}
	nextOwned, err := service.ListOwnedListings(ctx, sellerID, app.OwnedListingOptions{Limit: 2, Cursor: *owned.NextCursor})
	if err != nil || len(nextOwned.Items) != 1 || nextOwned.Items[0].ID == owned.Items[0].ID || nextOwned.Items[0].ID == owned.Items[1].ID {
		t.Fatalf("second owner page = %+v, %v", nextOwned, err)
	}

	search, err := service.SearchListings(ctx, app.SearchListingsOptions{Query: "100%_", Limit: 20})
	if err != nil || len(search.Items) != 1 || search.Items[0].ID != first.ID {
		t.Fatalf("literal search = %+v, %v", search, err)
	}
	ascending, err := service.SearchListings(ctx, app.SearchListingsOptions{Sort: app.ListingSortPriceAsc, Limit: 2})
	if err != nil || len(ascending.Items) != 2 || ascending.NextCursor == nil || ascending.Items[0].ID != second.ID {
		t.Fatalf("ascending search = %+v, %v", ascending, err)
	}
	nextAscending, err := service.SearchListings(ctx, app.SearchListingsOptions{Sort: app.ListingSortPriceAsc, Limit: 2, Cursor: *ascending.NextCursor})
	if err != nil || len(nextAscending.Items) != 1 || nextAscending.Items[0].ID != third.ID {
		t.Fatalf("ascending second page = %+v, %v", nextAscending, err)
	}
	descending, err := service.SearchListings(ctx, app.SearchListingsOptions{Sort: app.ListingSortPriceDesc, Limit: 20})
	if err != nil || len(descending.Items) != 3 || descending.Items[0].ID != third.ID || descending.Items[1].ID != first.ID {
		t.Fatalf("descending search = %+v, %v", descending, err)
	}
}

func TestCatalogDatabaseConstraintsAndVisibility(t *testing.T) {
	database := testdatabase.Open(t)
	ctx := context.Background()
	sellerID := insertCatalogUser(t, ctx, database, "700000000000000002", "seller-two")
	store := postgresadapter.NewCatalogStore(database.Pool)
	service := app.NewCatalogService(store, store, time.Now)
	listing := createCatalogListing(t, ctx, service, sellerID, "Visible listing", 100, "keyboard")

	checks := []struct {
		name  string
		query string
	}{
		{name: "blank title", query: `INSERT INTO listings (seller_id, category_id, title, price_idr, quantity, condition) VALUES ($1, $2, ' ', 1, 1, 'new')`},
		{name: "long description", query: `INSERT INTO listings (seller_id, category_id, title, description, price_idr, quantity, condition) VALUES ($1, $2, 'title', repeat('x', 5001), 1, 1, 'new')`},
		{name: "price range", query: `INSERT INTO listings (seller_id, category_id, title, price_idr, quantity, condition) VALUES ($1, $2, 'title', 0, 1, 'new')`},
		{name: "quantity range", query: `INSERT INTO listings (seller_id, category_id, title, price_idr, quantity, condition) VALUES ($1, $2, 'title', 1, 0, 'new')`},
		{name: "condition", query: `INSERT INTO listings (seller_id, category_id, title, price_idr, quantity, condition) VALUES ($1, $2, 'title', 1, 1, 'broken')`},
		{name: "status", query: `INSERT INTO listings (seller_id, category_id, title, price_idr, quantity, condition, status) VALUES ($1, $2, 'title', 1, 1, 'new', 'broken')`},
		{name: "moderation", query: `INSERT INTO listings (seller_id, category_id, title, price_idr, quantity, condition, moderation_status) VALUES ($1, $2, 'title', 1, 1, 'new', 'broken')`},
	}
	for _, check := range checks {
		t.Run(check.name, func(t *testing.T) {
			if _, err := database.Pool.Exec(ctx, check.query, sellerID, listing.CategoryID); err == nil {
				t.Error("invalid listing was accepted")
			}
		})
	}
	var uniqueCount int
	if err := database.Pool.QueryRow(ctx, `SELECT count(*) FROM pg_constraint WHERE conname = 'listings_id_seller_id_key'`).Scan(&uniqueCount); err != nil || uniqueCount != 1 {
		t.Fatalf("composite unique constraint count = %d, error = %v", uniqueCount, err)
	}
	if _, err := database.Pool.Exec(ctx, `UPDATE listings SET status = 'archived' WHERE id = $1`, listing.ID); err != nil {
		t.Fatalf("archive listing: %v", err)
	}
	if _, err := service.GetListing(ctx, listing.ID, nil); !errors.Is(err, domain.ErrNotFound) {
		t.Errorf("anonymous archived detail error = %v", err)
	}
	if _, err := service.GetListing(ctx, listing.ID, &sellerID); err != nil {
		t.Errorf("owner archived detail error = %v", err)
	}
	if _, err := database.Pool.Exec(ctx, `UPDATE listings SET moderation_status = 'removed' WHERE id = $1`, listing.ID); err != nil {
		t.Fatalf("remove listing: %v", err)
	}
	if _, err := service.GetListing(ctx, listing.ID, &sellerID); !errors.Is(err, domain.ErrNotFound) {
		t.Errorf("removed detail error = %v", err)
	}
	owned, err := service.ListOwnedListings(ctx, sellerID, app.OwnedListingOptions{})
	if err != nil || len(owned.Items) != 0 {
		t.Errorf("removed owner list = %+v, %v", owned, err)
	}
}

func insertCatalogUser(t *testing.T, ctx context.Context, database testdatabase.Database, discordID, handle string) int64 {
	t.Helper()
	var id int64
	err := database.Pool.QueryRow(ctx, `INSERT INTO users (discord_id, discord_username, display_name, handle) VALUES ($1, $2, $3, $4) RETURNING id`, discordID, handle, handle, handle).Scan(&id)
	if err != nil {
		t.Fatalf("insert user: %v", err)
	}
	return id
}

func createCatalogListing(t *testing.T, ctx context.Context, service *app.CatalogService, sellerID int64, title string, price int64, category string) domain.Listing {
	t.Helper()
	listing, err := service.CreateListing(ctx, sellerID, app.CreateListingInput{Title: title, PriceIDR: price, Quantity: 1, CategorySlug: category, Condition: domain.ListingConditionUsed})
	if err != nil {
		t.Fatalf("create listing %q: %v", title, err)
	}
	return listing
}
