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
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

func TestCatalogSchemaSeedsAndConstraints(t *testing.T) {
	database := testdatabase.Open(t)
	if database.MigrationVersion != 3 {
		t.Fatalf("migration version = %d, want 3", database.MigrationVersion)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	store := postgresadapter.NewCatalogStore(database.Pool)
	service := app.NewCatalogService(store, store, time.Now)
	categories, err := service.ListCategories(ctx)
	if err != nil {
		t.Fatalf("list categories: %v", err)
	}
	wantCategories := []string{"keyboard", "keycaps", "switches", "parts", "accessories", "other"}
	if len(categories) != len(wantCategories) {
		t.Fatalf("category count = %d, want %d", len(categories), len(wantCategories))
	}
	for index, slug := range wantCategories {
		if categories[index].Slug != slug {
			t.Errorf("category %d slug = %q, want %q", index, categories[index].Slug, slug)
		}
	}

	assertCatalogDatabaseError(t, execCatalogSQL(ctx, database.Pool,
		`INSERT INTO categories (slug, name, sort_order) VALUES ('Uppercase', 'Uppercase', 70)`), "23514")
	assertCatalogDatabaseError(t, execCatalogSQL(ctx, database.Pool,
		`INSERT INTO categories (slug, name, sort_order) VALUES ('blank-name', ' ', 70)`), "23514")
	assertCatalogDatabaseError(t, execCatalogSQL(ctx, database.Pool,
		`INSERT INTO categories (slug, name, sort_order) VALUES ('negative-order', 'Negative order', -1)`), "23514")

	seller := insertCatalogUser(t, ctx, database.Pool, "700000000000000001", "constraint-seller")
	categoryID := categories[0].ID
	valid := catalogRawListing{
		sellerID:         seller.ID,
		categoryID:       categoryID,
		title:            "Valid listing",
		description:      "Description",
		priceIDR:         1,
		quantity:         1,
		condition:        "new",
		status:           "active",
		moderationStatus: "visible",
		createdAt:        time.Date(2026, 8, 8, 12, 0, 0, 0, time.UTC),
		updatedAt:        time.Date(2026, 8, 8, 12, 0, 0, 0, time.UTC),
	}
	invalidListings := []struct {
		name    string
		listing catalogRawListing
	}{
		{name: "blank title", listing: withCatalogListing(valid, func(listing *catalogRawListing) { listing.title = " " })},
		{name: "long description", listing: withCatalogListing(valid, func(listing *catalogRawListing) { listing.description = strings.Repeat("a", 5001) })},
		{name: "zero price", listing: withCatalogListing(valid, func(listing *catalogRawListing) { listing.priceIDR = 0 })},
		{name: "zero quantity", listing: withCatalogListing(valid, func(listing *catalogRawListing) { listing.quantity = 0 })},
		{name: "unknown condition", listing: withCatalogListing(valid, func(listing *catalogRawListing) { listing.condition = "old" })},
		{name: "unknown status", listing: withCatalogListing(valid, func(listing *catalogRawListing) { listing.status = "hidden" })},
		{name: "unknown moderation status", listing: withCatalogListing(valid, func(listing *catalogRawListing) { listing.moderationStatus = "pending" })},
		{name: "updated before created", listing: withCatalogListing(valid, func(listing *catalogRawListing) { listing.updatedAt = listing.createdAt.Add(-time.Second) })},
	}
	for _, test := range invalidListings {
		t.Run(test.name, func(t *testing.T) {
			assertCatalogDatabaseError(t, test.listing.insert(ctx, database.Pool), "23514")
		})
	}

	missingSeller := withCatalogListing(valid, func(listing *catalogRawListing) { listing.sellerID = 9_999_999 })
	assertCatalogDatabaseError(t, missingSeller.insert(ctx, database.Pool), "23503")
	missingCategory := withCatalogListing(valid, func(listing *catalogRawListing) { listing.categoryID = 9_999_999 })
	assertCatalogDatabaseError(t, missingCategory.insert(ctx, database.Pool), "23503")

	var compositeConstraintExists bool
	if err := database.Pool.QueryRow(ctx, `
		SELECT EXISTS (
			SELECT 1
			FROM pg_constraint
			WHERE conrelid = 'listings'::regclass
			  AND conname = 'listings_id_seller_id_key'
		)
	`).Scan(&compositeConstraintExists); err != nil {
		t.Fatalf("read composite constraint: %v", err)
	}
	if !compositeConstraintExists {
		t.Error("listings_id_seller_id_key constraint is missing")
	}

	rows, err := database.Pool.Query(ctx, `
		SELECT indexname
		FROM pg_indexes
		WHERE schemaname = current_schema()
		  AND tablename = 'listings'
	`)
	if err != nil {
		t.Fatalf("list listing indexes: %v", err)
	}
	defer rows.Close()
	indexes := map[string]bool{}
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			t.Fatalf("scan listing index: %v", err)
		}
		indexes[name] = true
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("iterate listing indexes: %v", err)
	}
	for _, name := range []string{
		"listings_status_created_at_id_idx",
		"listings_category_id_status_created_at_idx",
		"listings_seller_id_status_updated_at_idx",
		"listings_price_id_idx",
	} {
		if !indexes[name] {
			t.Errorf("listing index %q is missing", name)
		}
	}
}

func TestCatalogStoreLifecycleSearchAndVisibility(t *testing.T) {
	database := testdatabase.Open(t)
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	store := postgresadapter.NewCatalogStore(database.Pool)
	now := time.Date(2026, 8, 8, 12, 0, 0, 0, time.UTC)
	service := app.NewCatalogService(store, store, func() time.Time { return now })
	seller := insertCatalogUser(t, ctx, database.Pool, "700000000000000002", "catalog-seller")
	otherSeller := insertCatalogUser(t, ctx, database.Pool, "700000000000000003", "other-seller")
	if _, err := database.Pool.Exec(ctx, `UPDATE categories SET active = FALSE WHERE slug = 'other'`); err != nil {
		t.Fatalf("deactivate category: %v", err)
	}
	if _, err := service.CreateListing(ctx, seller, app.CreateListingInput{
		Title:        "Inactive category",
		PriceIDR:     1_000_000,
		Quantity:     1,
		CategorySlug: "other",
		Condition:    domain.ListingConditionNew,
	}); err == nil {
		t.Error("creating a listing in an inactive category unexpectedly succeeded")
	} else {
		var validationError *domain.ValidationError
		if !errors.As(err, &validationError) || validationError.Field != "category_slug" {
			t.Errorf("inactive category error = %v, want category_slug validation error", err)
		}
	}

	create := func(title string, priceIDR int64, condition domain.ListingCondition) domain.Listing {
		t.Helper()
		listing, err := service.CreateListing(ctx, seller, app.CreateListingInput{
			Title:        title,
			Description:  "Catalog description",
			PriceIDR:     priceIDR,
			Quantity:     1,
			CategorySlug: "keyboard",
			Condition:    condition,
		})
		if err != nil {
			t.Fatalf("create %q: %v", title, err)
		}
		return listing
	}

	neo := create("  Neo 98  ", 3_000_000, domain.ListingConditionUsed)
	if neo.Title != "Neo 98" || neo.Category.Slug != "keyboard" || neo.Seller.ID != seller.ID || !neo.CreatedAt.Equal(now) {
		t.Fatalf("created listing = %+v", neo)
	}
	loaded, err := store.GetByID(ctx, neo.ID)
	if err != nil || loaded.ID != neo.ID || loaded.Seller.Handle != "catalog-seller" {
		t.Fatalf("load created listing = %+v, error = %v", loaded, err)
	}

	now = now.Add(time.Minute)
	description := "Edited description\nwith a line break"
	priceIDR := int64(2_900_000)
	updated, err := service.UpdateListing(ctx, seller, neo.ID, app.UpdateListingInput{
		Description: &description,
		PriceIDR:    &priceIDR,
	})
	if err != nil {
		t.Fatalf("update listing: %v", err)
	}
	if updated.Title != "Neo 98" || updated.Description != description || updated.PriceIDR != priceIDR || !updated.UpdatedAt.Equal(now) {
		t.Errorf("updated listing = %+v", updated)
	}
	if _, err := service.UpdateListing(ctx, otherSeller, neo.ID, app.UpdateListingInput{Description: &description}); !errors.Is(err, domain.ErrForbidden) {
		t.Errorf("cross-seller update error = %v, want forbidden", err)
	}

	now = now.Add(time.Minute)
	change, err := service.ChangeListingStatus(ctx, seller, neo.ID, domain.ListingStatusReserved)
	if err != nil || !change.Changed || change.Listing.Status != domain.ListingStatusReserved || !change.Listing.UpdatedAt.Equal(now) {
		t.Fatalf("reserve listing = %+v, error = %v", change, err)
	}
	sameStatus, err := service.ChangeListingStatus(ctx, seller, neo.ID, domain.ListingStatusReserved)
	if err != nil || sameStatus.Changed || !sameStatus.Listing.UpdatedAt.Equal(change.Listing.UpdatedAt) {
		t.Fatalf("same status change = %+v, error = %v", sameStatus, err)
	}

	now = now.Add(time.Minute)
	alpha := create("Alpha", 1_000_000, domain.ListingConditionUsed)
	now = now.Add(time.Minute)
	bravo := create("Bravo", 1_000_000, domain.ListingConditionNew)
	now = now.Add(time.Minute)
	zulu := create("Zulu", 5_000_000, domain.ListingConditionUsed)
	now = now.Add(time.Minute)
	literal := create("Literal %_\\ Match", 4_000_000, domain.ListingConditionUsed)

	assertListingIDs(t, allPublicListingIDs(t, ctx, service, app.SearchListingsInput{Sort: "newest", Limit: 2}), []int64{
		literal.ID, zulu.ID, bravo.ID, alpha.ID, neo.ID,
	})
	assertListingIDs(t, allPublicListingIDs(t, ctx, service, app.SearchListingsInput{Sort: "price_asc", Limit: 2}), []int64{
		alpha.ID, bravo.ID, neo.ID, literal.ID, zulu.ID,
	})
	assertListingIDs(t, allPublicListingIDs(t, ctx, service, app.SearchListingsInput{Sort: "price_desc", Limit: 2}), []int64{
		zulu.ID, literal.ID, neo.ID, bravo.ID, alpha.ID,
	})

	queryOnly, err := service.SearchListings(ctx, app.SearchListingsInput{Query: "Neo"})
	if err != nil {
		t.Fatalf("text-only public search: %v", err)
	}
	assertListingIDs(t, listingIDs(queryOnly.Items), []int64{neo.ID})
	categoryOnly, err := service.SearchListings(ctx, app.SearchListingsInput{Category: "keyboard"})
	if err != nil {
		t.Fatalf("category-only public search: %v", err)
	}
	assertListingIDs(t, listingIDs(categoryOnly.Items), []int64{literal.ID, zulu.ID, bravo.ID, alpha.ID, neo.ID})
	conditionOnly, err := service.SearchListings(ctx, app.SearchListingsInput{Condition: "new"})
	if err != nil {
		t.Fatalf("condition-only public search: %v", err)
	}
	assertListingIDs(t, listingIDs(conditionOnly.Items), []int64{bravo.ID})
	minimumOnly := int64(4_000_000)
	minimumPage, err := service.SearchListings(ctx, app.SearchListingsInput{MinPrice: &minimumOnly})
	if err != nil {
		t.Fatalf("minimum-price public search: %v", err)
	}
	assertListingIDs(t, listingIDs(minimumPage.Items), []int64{literal.ID, zulu.ID})
	maximumOnly := int64(1_000_000)
	maximumPage, err := service.SearchListings(ctx, app.SearchListingsInput{MaxPrice: &maximumOnly})
	if err != nil {
		t.Fatalf("maximum-price public search: %v", err)
	}
	assertListingIDs(t, listingIDs(maximumPage.Items), []int64{bravo.ID, alpha.ID})

	minimumPrice, maximumPrice := int64(2_000_000), int64(3_000_000)
	filtered, err := service.SearchListings(ctx, app.SearchListingsInput{
		Query:     " neo ",
		Category:  " keyboard ",
		Condition: "used",
		MinPrice:  &minimumPrice,
		MaxPrice:  &maximumPrice,
		Sort:      "newest",
	})
	if err != nil {
		t.Fatalf("combined public search: %v", err)
	}
	assertListingIDs(t, listingIDs(filtered.Items), []int64{neo.ID})
	literalSearch, err := service.SearchListings(ctx, app.SearchListingsInput{Query: "%_\\"})
	if err != nil {
		t.Fatalf("literal wildcard search: %v", err)
	}
	assertListingIDs(t, listingIDs(literalSearch.Items), []int64{literal.ID})
	unknownCategory, err := service.SearchListings(ctx, app.SearchListingsInput{Category: "missing"})
	if err != nil {
		t.Fatalf("unknown category search: %v", err)
	}
	if len(unknownCategory.Items) != 0 || unknownCategory.NextCursor != nil {
		t.Errorf("unknown category page = %+v", unknownCategory)
	}

	ownedReserved, err := service.ListOwnedListings(ctx, seller, app.OwnedListingsInput{Status: "reserved"})
	if err != nil {
		t.Fatalf("list reserved listings: %v", err)
	}
	assertListingIDs(t, listingIDs(ownedReserved.Items), []int64{neo.ID})
	ownedIDs := allOwnedListingIDs(t, ctx, service, seller, app.OwnedListingsInput{Limit: 2})
	if len(ownedIDs) != 5 {
		t.Fatalf("owned listing count = %d, want 5 (%v)", len(ownedIDs), ownedIDs)
	}

	now = now.Add(time.Minute)
	archived := create("Archived listing", 2_000_000, domain.ListingConditionUsed)
	now = now.Add(time.Minute)
	if _, err := service.ChangeListingStatus(ctx, seller, archived.ID, domain.ListingStatusArchived); err != nil {
		t.Fatalf("archive listing: %v", err)
	}
	if _, err := service.GetListing(ctx, archived.ID, nil); !errors.Is(err, domain.ErrListingNotFound) {
		t.Errorf("anonymous archived detail error = %v, want not found", err)
	}
	if _, err := service.GetListing(ctx, archived.ID, &seller); err != nil {
		t.Errorf("owner archived detail: %v", err)
	}

	if _, err := database.Pool.Exec(ctx, `UPDATE listings SET moderation_status = 'removed' WHERE id = $1`, literal.ID); err != nil {
		t.Fatalf("remove listing for moderation: %v", err)
	}
	if _, err := service.GetListing(ctx, literal.ID, &seller); !errors.Is(err, domain.ErrListingNotFound) {
		t.Errorf("removed owner detail error = %v, want not found", err)
	}
	removedSearch, err := service.SearchListings(ctx, app.SearchListingsInput{Query: "Literal"})
	if err != nil {
		t.Fatalf("search removed listing: %v", err)
	}
	if len(removedSearch.Items) != 0 {
		t.Errorf("removed listing appeared in search: %+v", removedSearch.Items)
	}
	ownedAfterRemoval := allOwnedListingIDs(t, ctx, service, seller, app.OwnedListingsInput{Limit: 100})
	for _, listingID := range ownedAfterRemoval {
		if listingID == literal.ID {
			t.Error("removed listing appeared in owner page")
		}
	}
}

type catalogRawListing struct {
	sellerID         int64
	categoryID       int64
	title            string
	description      string
	priceIDR         int64
	quantity         int
	condition        string
	status           string
	moderationStatus string
	createdAt        time.Time
	updatedAt        time.Time
}

func (listing catalogRawListing) insert(ctx context.Context, pool *pgxpool.Pool) error {
	_, err := pool.Exec(ctx, `
		INSERT INTO listings (
			seller_id, category_id, title, description, price_idr, quantity,
			condition, status, moderation_status, created_at, updated_at
		)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)
	`, listing.sellerID, listing.categoryID, listing.title, listing.description, listing.priceIDR,
		listing.quantity, listing.condition, listing.status, listing.moderationStatus, listing.createdAt, listing.updatedAt)
	return err
}

func withCatalogListing(listing catalogRawListing, update func(*catalogRawListing)) catalogRawListing {
	update(&listing)
	return listing
}

func execCatalogSQL(ctx context.Context, pool *pgxpool.Pool, statement string) error {
	_, err := pool.Exec(ctx, statement)
	return err
}

func assertCatalogDatabaseError(t *testing.T, err error, wantCode string) {
	t.Helper()
	if err == nil {
		t.Fatal("database operation unexpectedly succeeded")
	}
	var databaseError *pgconn.PgError
	if !errors.As(err, &databaseError) {
		t.Fatalf("error = %T %v, want PostgreSQL error", err, err)
	}
	if databaseError.Code != wantCode {
		t.Errorf("PostgreSQL code = %s, want %s (%v)", databaseError.Code, wantCode, err)
	}
}

func insertCatalogUser(t *testing.T, ctx context.Context, pool *pgxpool.Pool, discordID, handle string) domain.User {
	t.Helper()
	var user domain.User
	err := pool.QueryRow(ctx, `
		INSERT INTO users (discord_id, discord_username, display_name, handle)
		VALUES ($1, $2, $3, $4)
		RETURNING id, status
	`, discordID, handle, handle, handle).Scan(&user.ID, &user.Status)
	if err != nil {
		t.Fatalf("insert catalog user: %v", err)
	}
	user.Handle = handle
	user.DisplayName = handle
	return user
}

func allPublicListingIDs(t *testing.T, ctx context.Context, service *app.CatalogService, input app.SearchListingsInput) []int64 {
	t.Helper()
	var ids []int64
	seen := map[int64]bool{}
	for {
		page, err := service.SearchListings(ctx, input)
		if err != nil {
			t.Fatalf("search page: %v", err)
		}
		for _, listingID := range listingIDs(page.Items) {
			if seen[listingID] {
				t.Fatalf("listing %d appeared more than once across pages", listingID)
			}
			seen[listingID] = true
			ids = append(ids, listingID)
		}
		if page.NextCursor == nil {
			return ids
		}
		input.Cursor = *page.NextCursor
	}
}

func allOwnedListingIDs(t *testing.T, ctx context.Context, service *app.CatalogService, seller domain.User, input app.OwnedListingsInput) []int64 {
	t.Helper()
	var ids []int64
	seen := map[int64]bool{}
	for {
		page, err := service.ListOwnedListings(ctx, seller, input)
		if err != nil {
			t.Fatalf("list owned page: %v", err)
		}
		for _, listingID := range listingIDs(page.Items) {
			if seen[listingID] {
				t.Fatalf("listing %d appeared more than once across owner pages", listingID)
			}
			seen[listingID] = true
			ids = append(ids, listingID)
		}
		if page.NextCursor == nil {
			return ids
		}
		input.Cursor = *page.NextCursor
	}
}

func listingIDs(listings []domain.Listing) []int64 {
	ids := make([]int64, 0, len(listings))
	for _, listing := range listings {
		ids = append(ids, listing.ID)
	}
	return ids
}

func assertListingIDs(t *testing.T, got, want []int64) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("listing IDs = %v, want %v", got, want)
	}
	for index := range want {
		if got[index] != want[index] {
			t.Errorf("listing ID %d = %d, want %d (all: %v)", index, got[index], want[index], got)
		}
	}
}
