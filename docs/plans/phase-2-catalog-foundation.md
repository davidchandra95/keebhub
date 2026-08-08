# Phase 2 Catalog Foundation

Status: Implemented

## Summary

Build the first complete catalog slice:

- Seed categories.
- Create, edit, manage, and read listings.
- Add public marketplace search, filters, selectable sorting, and cursor pagination.
- Keep the existing `status`, `cursor`, and `limit` parameters for `GET /api/v1/me/listings`.
- Keep this phase API-only. Do not add or change React pages, routes, components, or client state.

No new dependencies are needed.

## Data Model

Add `db/migrations/00003_catalog_foundation.sql`.

### Categories

Create `categories` with:

- Identity `BIGINT` primary key.
- Unique lowercase slug, maximum 50 characters.
- Nonblank name, maximum 100 characters.
- Nonnegative `sort_order`.
- `active BOOLEAN NOT NULL DEFAULT TRUE`.

Seed active rows in this order, using sort values `10` through `60`:

1. `keyboard` - Keyboard
2. `keycaps` - Keycaps
3. `switches` - Switches
4. `parts` - Parts
5. `accessories` - Accessories
6. `other` - Other

### Listings

Create `listings` with the documented columns and these database rules:

- `seller_id` references `users(id)`.
- `category_id` references `categories(id)`.
- Title is nonblank and at most 120 Unicode characters.
- Description is at most 5,000 Unicode characters.
- `price_idr` is between 1 and 10,000,000,000.
- `quantity` is between 1 and 1,000,000.
- Condition is `new` or `used`.
- Seller status is `active`, `reserved`, `sold`, or `archived`.
- `moderation_status` is separate and limited to `visible` or `removed`.
- `updated_at >= created_at`.
- Add the required unique constraint on `(id, seller_id)`.
- Add the catalog indexes already specified in `docs/06-database-design.md`.
- The down migration drops listings before categories.

`moderation_status` is internal. Sellers cannot set it, and HTTP responses do not expose it.

## Domain and Application Design

Add typed category, condition, seller status, and moderation status values under `internal/domain`.

Listing validation must:

- Trim outer whitespace from title and category slug.
- Reject a blank title.
- Count Unicode characters, not bytes.
- Preserve description text and line breaks.
- Validate every price, quantity, condition, status, and moderation boundary.

Status rules:

| From | Allowed targets |
|---|---|
| active | reserved, sold, archived |
| reserved | active, sold, archived |
| sold | active, archived |
| archived | active |
| any status | the same status as a successful no-op |

A same-status request returns the existing listing with `200 OK` and does not change `updated_at`. Other disallowed transitions return a typed conflict error.

Create `CatalogService` in the application layer. Define small, consumer-owned `CategoryRepository` and `ListingRepository` interfaces there. Required use cases are:

- `ListCategories`
- `CreateListing`
- `UpdateListing`
- `ChangeListingStatus`
- `ListOwnedListings`
- `GetListing`
- `SearchListings`

Application behavior:

- Resolve category slugs only against active categories.
- Apply create defaults: empty description, quantity 1, negotiable false, status active, moderation visible.
- Keep status changes out of generic listing updates.
- Check ownership in the application layer, then also include `(id, seller_id)` in mutation queries.
- Use partial SQL updates so concurrent edits to different fields do not overwrite each other.
- Use an injected UTC clock for mutation timestamps.
- Return typed validation, not-found, forbidden, and conflict errors.

Visibility rules:

| Listing state | Anonymous | Owner |
|---|---:|---:|
| active | visible | visible |
| reserved | visible | visible |
| sold | visible | visible |
| archived | 404 | visible |
| moderation removed | 404 | 404 |

Disabled sellers may retain visible historical listings but cannot perform authenticated mutations.

## PostgreSQL and sqlc

Add category and listing query files and regenerate sqlc output with `make sqlc`. Never hand-edit generated files.

Queries must cover:

- Active categories ordered by `sort_order, id`.
- Active category lookup by slug.
- Listing insert with returned category and public seller details.
- Listing lookup by ID.
- Partial owner update.
- Owner status update.
- Owner listing page ordered by `updated_at DESC, id DESC`.
- Public listing search for each supported sort order.

`GET /api/v1/me/listings` supports:

- Optional status filter across all four seller statuses.
- Default limit 20, maximum 100.
- Stable cursor pagination based on `(updated_at, id)`.
- A versioned base64url JSON cursor bound to the selected status.
- `400 Bad Request` for malformed cursors or filter mismatch.
- `next_cursor: null` when no following page exists.

Fetch `limit + 1` rows to determine whether another page exists.

### Public marketplace search

`GET /api/v1/listings` is public and supports:

- `q` for a case-insensitive literal substring match against title or description.
- `category` for an exact category slug match.
- `condition` with `new` or `used`.
- `min_price` and `max_price`, both inclusive.
- `sort` with `newest`, `price_asc`, or `price_desc`.
- `cursor` for the next result page.
- `limit` with default 20 and maximum 100.

Normalize `q` and `category` by trimming outer whitespace. Treat an empty normalized value as absent. Escape PostgreSQL `ILIKE` wildcard characters in `q` so `%`, `_`, and backslash are searched as literal characters. Reject `q` longer than 100 Unicode characters, invalid conditions or sort values, prices outside listing bounds, and `min_price > max_price` with `400 Bad Request`. An unknown category slug returns an empty successful page.

Only listings with seller status `active` or `reserved` and moderation status `visible` appear in search. Sold and archived listings remain available only through the detail visibility rules.

Use these stable orders and cursor keys:

| Sort | Database order | Cursor position |
|---|---|---|
| `newest` | `created_at DESC, id DESC` | `created_at`, `id` |
| `price_asc` | `price_idr ASC, id ASC` | `price_idr`, `id` |
| `price_desc` | `price_idr DESC, id DESC` | `price_idr`, `id` |

Use separate sqlc queries for the three orders instead of dynamic SQL or `CASE` sorting. Each query applies all optional filters and fetches `limit + 1` rows.

Public search cursors are versioned base64url JSON. They include a cursor kind, normalized `q`, category, condition, minimum price, maximum price, sort, and the last row's position values. Reject malformed cursors, unknown cursor versions, a non-public cursor kind, invalid position values, or any mismatch between cursor-bound options and the current request with `400 Bad Request`. Changing only `limit` between pages is allowed. Return `next_cursor: null` when there is no following page.

## HTTP Contract

Implement these routes and mark them implemented in `api/openapi.yaml`:

- `GET /api/v1/categories`
- `GET /api/v1/listings`
- `POST /api/v1/listings`
- `PATCH /api/v1/listings/{listing_id}`
- `POST /api/v1/listings/{listing_id}/status`
- `GET /api/v1/me/listings`
- `GET /api/v1/listings/{listing_id}`

Preserve the current request and response shapes. IDs remain JSON strings, money remains integer rupiah, and timestamps use UTC RFC 3339.

Strict JSON requirements for all listing writes:

- Require `application/json`, allowing standard parameters such as `charset`.
- Reject unknown fields.
- Reject multiple JSON values.
- Reject empty bodies.
- Reject explicit `null` for every listing field.
- Use presence-aware request fields so PATCH can distinguish omitted values from `null`.
- Reject an empty PATCH object.
- Apply a 32 KiB endpoint body limit.
- Return field-specific `validation_failed` errors.

Error mapping:

- Missing session: `401`.
- Authenticated non-owner: `403`.
- Missing, hidden, malformed-ID, or moderation-removed listing: `404`.
- Invalid cursor or malformed JSON: `400`.
- Invalid transition: `409`.
- Domain field failure or inactive category: `422`.
- Oversized body: `413`.
- Unsupported content type: `415`.
- Unexpected database failure: safe `500` with full internal logging.

Log successful mutations using request ID, seller ID, listing ID, and old/new status where relevant. Do not log listing descriptions or cookies.

## API-only Boundary

This phase stops at the HTTP API. Do not add or change frontend routes, forms, listing cards, browser state, API clients, or frontend tests. The public listing detail requirement is satisfied by an unauthenticated `GET /api/v1/listings/{listing_id}` JSON response. A later frontend phase will consume these stable API contracts.

## Tests

### Domain and application

Use table-driven tests for:

- All status pairs, including same-status no-op.
- Unknown status and condition values.
- Unicode title and description boundaries.
- Price and quantity boundaries.
- Create defaults.
- Inactive or missing category.
- Partial update behavior.
- Ownership rejection.
- Archived and moderation visibility.
- Repository failures and same-status no-write behavior.
- Search option validation and normalization.

### PostgreSQL integration

Using real PostgreSQL:

- Migration version becomes 3.
- Seed rows and ordering are correct.
- Every database check constraint rejects invalid data.
- Foreign keys and `(id, seller_id)` uniqueness exist.
- Create, joined detail read, partial update, status update, visibility, status filtering, ordering, and cursor boundaries work.
- Public search covers every individual filter, combined filters, literal wildcard characters, all three sort orders, equal-value ID tie-breakers, page boundaries, and empty results.
- Removed listings are excluded from owner lists and normal detail access.

### HTTP and authorization

Cover:

- Every successful response shape and ID-as-string rule.
- Authentication and cross-seller authorization.
- Origin checks for unsafe requests.
- Unknown fields, multiple JSON values, nulls, empty objects, bad content types, and oversized bodies.
- Invalid path IDs and cursors.
- Invalid search parameters, `min_price > max_price`, and cursors reused with changed filters or sorting.
- Public search visibility, combined filters, all sort orders, stable next-page behavior, and `next_cursor: null`.
- Same-status `200` behavior.
- Archived and removed visibility.
- Full real-PostgreSQL flow: fake Discord login, create, edit, change status, list as owner, then fetch the detail and find the listing through search without a session.

## Verification and Acceptance

Run:

```text
make sqlc
make docs-check
make vet lint test-race mod-verify
make web-check
TEST_DATABASE_URL='postgres://keebhub:keebhub@localhost:54329/keebhub?sslmode=disable' make test-integration
make docker-build
```

Integration acceptance is not complete if PostgreSQL tests were skipped.

API acceptance gate:

1. Authenticate a seller and create a used `Neo 98` listing for `Rp3.000.000` through the API.
2. Edit its price or description through the API.
3. Change it to reserved through the API.
4. Confirm it appears in `GET /api/v1/me/listings`.
5. Without a session cookie, fetch `GET /api/v1/listings/{listing_id}` and confirm it returns the edited reserved listing.
6. Without a session cookie, find it through `GET /api/v1/listings` using text, category, condition, and price filters.
7. Confirm `newest`, `price_asc`, and `price_desc` return stable orders across cursor pages without duplicates or missing rows.

Seller catalogs, Discord export, chat, reports, moderation operations, images, and all frontend work remain outside Phase 2.
