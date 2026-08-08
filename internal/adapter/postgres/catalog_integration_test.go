package postgres_test

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	postgresadapter "github.com/davidchandra95/keebhub/internal/adapter/postgres"
	"github.com/davidchandra95/keebhub/internal/app"
	"github.com/davidchandra95/keebhub/internal/domain"
	"github.com/davidchandra95/keebhub/internal/testutil/testdatabase"
	"github.com/jackc/pgx/v5/pgxpool"
)

func TestCatalogStoreAndServicePostgreSQLFlow(t *testing.T) {
	database := testdatabase.Open(t)
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	store := postgresadapter.NewCatalogStore(database.Pool)
	if database.MigrationVersion != 3 {
		t.Fatalf("migration version = %d, want 3", database.MigrationVersion)
	}

	var categoryCount int
	if err := database.Pool.QueryRow(ctx, `SELECT count(*) FROM categories`).Scan(&categoryCount); err != nil {
		t.Fatalf("count categories: %v", err)
	}
	if categoryCount != 6 {
		t.Fatalf("category count = %d, want 6", categoryCount)
	}
	categories, err := store.ListActiveCategories(ctx)
	if err != nil {
		t.Fatalf("list categories: %v", err)
	}
	wantSlugs := []string{"keyboard", "keycaps", "switches", "parts", "accessories", "other"}
	for index, category := range categories {
		if category.Slug != wantSlugs[index] || category.SortOrder != int32((index+1)*10) {
			t.Errorf("category %d = %+v", index, category)
		}
	}

	sellerID := seedCatalogUser(t, database.Pool, "700000000000000001", "catalog-seller", "Catalog Seller")
	otherSellerID := seedCatalogUser(t, database.Pool, "700000000000000002", "other-seller", "Other Seller")
	base := time.Date(2026, 8, 8, 12, 0, 0, 0, time.UTC)
	now := base
	service := app.NewCatalogServiceWithClock(store, store, func() time.Time { return now })
	create := func(input app.CreateListingInput) domain.Listing {
		listing, createErr := service.CreateListing(ctx, sellerID, input)
		if createErr != nil {
			t.Fatalf("create %q: %v", input.Title, createErr)
		}
		now = now.Add(time.Minute)
		return listing
	}
	neo := create(app.CreateListingInput{Title: "Neo 98", Description: "Silver keyboard", PriceIDR: 3000000, CategorySlug: "keyboard", Condition: domain.ConditionUsed})
	percent := create(app.CreateListingInput{Title: "100% Board", Description: "Literal wildcard item", PriceIDR: 1000000, CategorySlug: "keyboard", Condition: domain.ConditionNew})
	create(app.CreateListingInput{Title: "GMK Keycaps", PriceIDR: 2000000, CategorySlug: "keycaps", Condition: domain.ConditionUsed})
	sold := create(app.CreateListingInput{Title: "Switch Pack", PriceIDR: 4000000, CategorySlug: "switches", Condition: domain.ConditionNew})
	archived := create(app.CreateListingInput{Title: "Archived Parts", PriceIDR: 5000000, CategorySlug: "parts", Condition: domain.ConditionUsed})
	other := createForSeller(t, service, ctx, otherSellerID, app.CreateListingInput{Title: "Other Seller Keyboard", PriceIDR: 3000000, CategorySlug: "keyboard", Condition: domain.ConditionNew})
	if _, err := service.ChangeListingStatus(ctx, sellerID, sold.ID, domain.StatusSold); err != nil {
		t.Fatalf("mark sold: %v", err)
	}
	if _, err := service.ChangeListingStatus(ctx, sellerID, archived.ID, domain.StatusArchived); err != nil {
		t.Fatalf("archive: %v", err)
	}

	if neo.Seller.ID != sellerID || neo.Seller.Handle != "catalog-seller" || neo.Category.Slug != "keyboard" {
		t.Errorf("joined created listing = %+v", neo)
	}
	updatedDescription := "line one\nline two"
	updated, err := service.UpdateListing(ctx, sellerID, neo.ID, app.UpdateListingInput{Description: &updatedDescription})
	if err != nil || updated.Description != updatedDescription || updated.Title != neo.Title {
		t.Fatalf("partial update = %+v, error = %v", updated, err)
	}
	if _, err := service.UpdateListing(ctx, otherSellerID, neo.ID, app.UpdateListingInput{Description: &updatedDescription}); !errors.Is(err, domain.ErrForbidden) {
		t.Errorf("cross-seller update error = %v", err)
	}

	ownerPage, err := service.ListOwnedListings(ctx, sellerID, app.ListOwnedListingsInput{Limit: 2})
	if err != nil || len(ownerPage.Items) != 2 || ownerPage.NextCursor == nil {
		t.Fatalf("owner first page = %+v, error = %v", ownerPage, err)
	}
	ownerSecondPage, err := service.ListOwnedListings(ctx, sellerID, app.ListOwnedListingsInput{Cursor: *ownerPage.NextCursor, Limit: 2})
	if err != nil || len(ownerSecondPage.Items) != 2 || ownerSecondPage.NextCursor == nil {
		t.Fatalf("owner second page = %+v, error = %v", ownerSecondPage, err)
	}
	ownerThirdPage, err := service.ListOwnedListings(ctx, sellerID, app.ListOwnedListingsInput{Cursor: *ownerSecondPage.NextCursor, Limit: 2})
	if err != nil || len(ownerThirdPage.Items) != 1 || ownerThirdPage.NextCursor != nil {
		t.Fatalf("owner third page = %+v, error = %v", ownerThirdPage, err)
	}
	for _, item := range ownerSecondPage.Items {
		if item.ModerationStatus == domain.ModerationRemoved {
			t.Errorf("owner list exposed removed listing: %+v", item)
		}
	}

	newest, err := service.SearchListings(ctx, app.SearchListingsInput{Sort: app.SortNewest, Limit: 2})
	if err != nil || len(newest.Items) != 2 || newest.NextCursor == nil {
		t.Fatalf("newest first page = %+v, error = %v", newest, err)
	}
	newestSecond, err := service.SearchListings(ctx, app.SearchListingsInput{Sort: app.SortNewest, Cursor: *newest.NextCursor, Limit: 2})
	if err != nil || len(newestSecond.Items) != 2 || newestSecond.NextCursor != nil {
		t.Fatalf("newest second page = %+v, error = %v", newestSecond, err)
	}
	seen := map[int64]bool{}
	for _, item := range append(newest.Items, newestSecond.Items...) {
		if seen[item.ID] {
			t.Errorf("duplicate search item %d", item.ID)
		}
		seen[item.ID] = true
		if item.ID == sold.ID || item.ID == archived.ID {
			t.Errorf("non-public status in search: %+v", item)
		}
	}

	priceAsc, err := service.SearchListings(ctx, app.SearchListingsInput{Sort: app.SortPriceAsc, Limit: 100})
	if err != nil || len(priceAsc.Items) != 4 || priceAsc.Items[0].PriceIDR != 1000000 || priceAsc.Items[len(priceAsc.Items)-1].PriceIDR != 3000000 {
		t.Fatalf("price ascending = %+v, error = %v", priceAsc, err)
	}
	priceDesc, err := service.SearchListings(ctx, app.SearchListingsInput{Sort: app.SortPriceDesc, Limit: 100})
	if err != nil || len(priceDesc.Items) != 4 || priceDesc.Items[0].PriceIDR != 3000000 || priceDesc.Items[len(priceDesc.Items)-1].PriceIDR != 1000000 {
		t.Fatalf("price descending = %+v, error = %v", priceDesc, err)
	}
	for _, sortValue := range []app.ListingSort{app.SortPriceAsc, app.SortPriceDesc} {
		seen := make(map[int64]bool)
		cursor := ""
		for pageNumber := 0; pageNumber < 10; pageNumber++ {
			page, pageErr := service.SearchListings(ctx, app.SearchListingsInput{Sort: sortValue, Cursor: cursor, Limit: 1})
			if pageErr != nil {
				t.Fatalf("%s page %d: %v", sortValue, pageNumber, pageErr)
			}
			for _, item := range page.Items {
				if seen[item.ID] {
					t.Errorf("%s duplicate item %d", sortValue, item.ID)
				}
				seen[item.ID] = true
			}
			if page.NextCursor == nil {
				break
			}
			cursor = *page.NextCursor
		}
		if len(seen) != 4 {
			t.Errorf("%s paged item count = %d, want 4", sortValue, len(seen))
		}
	}

	percentPage, err := service.SearchListings(ctx, app.SearchListingsInput{Query: "100%", Sort: app.SortNewest, Limit: 20})
	if err != nil || len(percentPage.Items) != 1 || percentPage.Items[0].ID != percent.ID {
		t.Fatalf("literal wildcard search = %+v, error = %v", percentPage, err)
	}
	filtered, err := service.SearchListings(ctx, app.SearchListingsInput{
		Category: " keyboard ", Condition: ptrCondition(domain.ConditionNew),
		MinPrice: ptrInt64(1000000), MaxPrice: ptrInt64(1000000), Sort: app.SortNewest, Limit: 20,
	})
	if err != nil || len(filtered.Items) != 1 || filtered.Items[0].ID != percent.ID {
		t.Fatalf("combined filters = %+v, error = %v", filtered, err)
	}
	unknownCategory, err := service.SearchListings(ctx, app.SearchListingsInput{Category: "does-not-exist", Limit: 20})
	if err != nil || len(unknownCategory.Items) != 0 || unknownCategory.NextCursor != nil {
		t.Fatalf("unknown category = %+v, error = %v", unknownCategory, err)
	}

	if _, err := service.GetListing(ctx, archived.ID, nil); !errors.Is(err, domain.ErrNotFound) {
		t.Errorf("anonymous archived detail = %v", err)
	}
	ownerID := sellerID
	if _, err := service.GetListing(ctx, archived.ID, &ownerID); err != nil {
		t.Errorf("owner archived detail = %v", err)
	}
	if _, err := service.GetListing(ctx, sold.ID, nil); err != nil {
		t.Errorf("sold detail = %v", err)
	}
	if _, err := database.Pool.Exec(ctx, `UPDATE listings SET moderation_status = 'removed' WHERE id = $1`, neo.ID); err != nil {
		t.Fatalf("remove listing: %v", err)
	}
	if _, err := service.GetListing(ctx, neo.ID, &ownerID); !errors.Is(err, domain.ErrNotFound) {
		t.Errorf("removed detail = %v", err)
	}

	assertCatalogConstraints(t, database.Pool, sellerID)
	_ = other
}

func assertCatalogConstraints(t *testing.T, pool *pgxpool.Pool, sellerID int64) {
	t.Helper()
	var constraintCount int
	if err := pool.QueryRow(context.Background(), `
		SELECT count(*)
		FROM pg_constraint
		WHERE conrelid = 'listings'::regclass
		  AND conname = 'listings_id_seller_id_key'`).Scan(&constraintCount); err != nil {
		t.Fatalf("find composite listing constraint: %v", err)
	}
	if constraintCount != 1 {
		t.Fatalf("composite listing constraint count = %d", constraintCount)
	}

	categoryID := int64(1)
	invalid := []struct {
		name string
		args []any
	}{
		{name: "blank title", args: []any{sellerID, categoryID, "   ", "", int64(1000), int32(1), "new", "active", "visible"}},
		{name: "long description", args: []any{sellerID, categoryID, "Valid", strings.Repeat("x", 5001), int64(1000), int32(1), "new", "active", "visible"}},
		{name: "price below range", args: []any{sellerID, categoryID, "Valid", "", int64(0), int32(1), "new", "active", "visible"}},
		{name: "quantity below range", args: []any{sellerID, categoryID, "Valid", "", int64(1000), int32(0), "new", "active", "visible"}},
		{name: "condition invalid", args: []any{sellerID, categoryID, "Valid", "", int64(1000), int32(1), "broken", "active", "visible"}},
		{name: "seller status invalid", args: []any{sellerID, categoryID, "Valid", "", int64(1000), int32(1), "new", "broken", "visible"}},
		{name: "moderation status invalid", args: []any{sellerID, categoryID, "Valid", "", int64(1000), int32(1), "new", "active", "hidden"}},
	}
	for _, test := range invalid {
		t.Run(test.name, func(t *testing.T) {
			_, err := pool.Exec(context.Background(), `
				INSERT INTO listings (seller_id, category_id, title, description, price_idr, quantity, condition, status, moderation_status)
				VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)`, test.args...)
			if err == nil {
				t.Fatal("invalid listing was accepted")
			}
		})
	}
	_, err := pool.Exec(context.Background(), `
		INSERT INTO listings (seller_id, category_id, title, description, price_idr, quantity, condition, status, moderation_status, created_at, updated_at)
		VALUES ($1, $2, 'Valid', '', 1000, 1, 'new', 'active', 'visible', now(), now() - interval '1 second')`, sellerID, categoryID)
	if err == nil {
		t.Error("updated_at before created_at was accepted")
	}
	_, err = pool.Exec(context.Background(), `
		INSERT INTO listings (seller_id, category_id, title, description, price_idr, quantity, condition, status, moderation_status)
		VALUES ($1, 999999, 'Valid', '', 1000, 1, 'new', 'active', 'visible')`, sellerID)
	if err == nil {
		t.Error("invalid category foreign key was accepted")
	}
}

func seedCatalogUser(t *testing.T, pool *pgxpool.Pool, discordID, handle, displayName string) int64 {
	t.Helper()
	var id int64
	if err := pool.QueryRow(context.Background(), `
		INSERT INTO users (discord_id, discord_username, display_name, handle)
		VALUES ($1, $2, $3, $4)
		RETURNING id`, discordID, handle, displayName, handle).Scan(&id); err != nil {
		t.Fatalf("seed user: %v", err)
	}
	return id
}

func createForSeller(t *testing.T, service *app.CatalogService, ctx context.Context, sellerID int64, input app.CreateListingInput) domain.Listing {
	t.Helper()
	listing, err := service.CreateListing(ctx, sellerID, input)
	if err != nil {
		t.Fatalf("create other seller listing: %v", err)
	}
	return listing
}

func ptrCondition(value domain.ListingCondition) *domain.ListingCondition {
	return &value
}

func ptrInt64(value int64) *int64 {
	return &value
}
