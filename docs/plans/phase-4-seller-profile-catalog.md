# Phase 4 Seller Profile and Catalog

Status: Ready for implementation

## Summary

Build the complete backend slice for seller profiles and public seller catalogs:

- Allow an authenticated seller to update optional location and bio fields.
- Return the account creation timestamp from the current-user API.
- Provide a public seller profile with a visible active-listing count.
- Provide stable, paginated active, reserved, and sold seller catalog reads.
- Keep seller handles stable and read-only.

This phase is backend-only. Do not add or change React pages, routes, components, browser state, API clients, or frontend tests. Listing detail, listing creation, and seller management remain separate work. Reports, blocking, moderation operations, and trust UI are post-v1 work.

No new dependency, database migration, or `catalogs` table is needed. The existing `users`, `listings`, and `categories` tables already contain the required durable state. Preserve all existing chat and SSE behavior while adding this slice.

## Public Contract

Implement and mark these operations as implemented in `api/openapi.yaml`:

- `PATCH /api/v1/me`
- `GET /api/v1/users/{handle}`
- `GET /api/v1/users/{handle}/listings`

Keep IDs as JSON strings and timestamps as UTC RFC 3339.

### Current user

Add `created_at` to the required authenticated `User` response returned by both `GET /api/v1/me` and `PATCH /api/v1/me`.

`PATCH /api/v1/me` accepts a strict partial request:

```json
{
  "location": "Jakarta Barat",
  "bio": "Keyboard enthusiast"
}
```

Both fields are optional and nullable. Apply these rules independently:

- An omitted field remains unchanged.
- Explicit `null` clears the field to database `NULL`.
- A string is trimmed at its outer edges.
- A string that is empty after trimming clears the field to database `NULL`.
- Internal spaces and bio line breaks are preserved.
- Location is limited to 100 Unicode characters.
- Bio is limited to 500 Unicode characters.
- An empty object is rejected with `400 Bad Request`.
- A request whose normalized values equal the current values returns `200 OK` without a database write or `updated_at` change.

The seller handle, Discord identity, display name, and avatar remain controlled by the existing login flow and cannot be edited through this endpoint.

### Public seller profile

`GET /api/v1/users/{handle}` is public and returns:

```json
{
  "user": {
    "id": "42",
    "handle": "gunawan",
    "display_name": "Gunawan",
    "avatar_url": "https://...",
    "location": "Jakarta Barat",
    "bio": "Keyboard enthusiast",
    "created_at": "2026-08-09T10:00:00Z",
    "active_listing_count": 3
  }
}
```

Add a dedicated `SellerProfile` schema for this response. Keep the existing `PublicUser` schema unchanged because listing and chat responses do not need joined date or listing count.

The active listing count includes only listings whose seller status is `active` and moderation status is `visible`. Reserved, sold, archived, and moderation-removed listings do not contribute to the count. This preserves the existing visibility safeguard; it does not add a v1 report or operator workflow.

A seller with no listings still returns `200 OK` with `active_listing_count: 0`. Disabled sellers remain publicly readable so their existing public history and links do not disappear. A malformed or unknown handle returns `404 Not Found` without revealing any additional account state.

Handles are canonical lowercase values using the existing 3 to 40 character grammar. Do not normalize a malformed public path into a different seller or add handle redirects in this phase.

### Public seller catalog

`GET /api/v1/users/{handle}/listings` is public and accepts:

```text
status
category
cursor
limit
```

`status` accepts one value:

- `active`
- `reserved`
- `sold`

When `status` is omitted, return active listings first and reserved listings second. Sold listings appear only when the caller explicitly requests `status=sold`. Archived and moderation-removed listings never appear.

Normalize `category` by trimming outer whitespace. Treat an empty normalized value as absent. A nonempty category is an exact slug filter. An unknown category returns a successful empty page.

Use a default limit of 20 and a maximum of 100. Fetch `limit + 1` rows to determine whether another page exists. Return `next_cursor: null` when no following page exists.

Use these stable orders:

| Request | Database order |
|---|---|
| status omitted | active before reserved, then `updated_at DESC, id DESC` |
| one status selected | `updated_at DESC, id DESC` |

The response remains the existing `ListingPage` shape. Each listing continues to include its existing seller summary.

## Cursor Contract

Seller catalog cursors are opaque, versioned, base64url-encoded JSON. Add a distinct cursor kind such as `seller_listings` rather than reusing owned or marketplace cursors.

Bind each cursor to:

- seller ID resolved from the requested handle;
- selected status, including the distinction between omitted status and a specific status;
- normalized category;
- the last row's status rank, `updated_at`, and listing ID.

Status rank is `0` for active and `1` for reserved. A status-specific page may use a constant rank but must keep the same cursor shape and validation path.

Reject with `400 Bad Request`:

- malformed base64url or JSON;
- unknown cursor versions or kinds;
- missing or invalid position values;
- cursors created for a different seller;
- status or category changes between pages.

Changing only `limit` between pages is allowed.

## Domain and Application Design

Add seller-profile domain values under `internal/domain` for:

- maximum location and bio lengths;
- optional profile normalization and validation;
- a public seller profile projection containing `domain.PublicUser`, creation time, and active listing count.

Count characters with Unicode rune counts rather than bytes. Keep transport-specific presence tracking outside the domain.

Add `SellerService` under `internal/app`. Define small, consumer-owned interfaces:

- `SellerRepository` for profile update and public profile lookup;
- `SellerCatalogRepository` for seller-scoped listing pages.

Required use cases are:

- `UpdateProfile`
- `GetSellerProfile`
- `ListSellerListings`

Represent each update field with explicit presence and nullable value information so omitted, null, and non-null values remain distinct after HTTP decoding.

Application behavior:

- Accept the authenticated `domain.User` for profile comparison and authorization context.
- Normalize and validate every present field before persistence.
- Return the current user without writing when no effective value changes.
- Use an injected UTC clock for real profile updates.
- Resolve the seller by handle before listing lookup so a real seller with zero listings differs from an unknown seller.
- Allow disabled sellers to be returned by public profile and catalog reads.
- Reject profile mutation when the account is no longer active, including a disable that races with session authentication.
- Validate status, category, limit, and cursor in the application layer.
- Keep public visibility and cursor semantics out of the HTTP and PostgreSQL adapters.
- Return typed validation, query, not-found, and disabled-user errors.

## PostgreSQL and sqlc

Add profile queries under `db/queries/` and seller catalog queries beside the existing catalog queries. Run `make sqlc` and include regenerated output. Never edit `internal/generated/db/` manually.

Persistence operations must cover:

- Partial location and bio update by user ID, with explicit set flags and nullable values.
- An update guard requiring `users.status = 'active'`.
- Public seller lookup by canonical handle.
- Visible active-listing count using a left join or equivalent query so a seller with zero listings is returned.
- Seller listing pages filtered by seller ID, optional status, optional category, moderation visibility, cursor position, and page limit.

The profile update returns the full authenticated user model. Map no-row results from an active-only update to the disabled-user error rather than exposing a persistence error.

The public profile query must not expose Discord ID, Discord username, user status, or any session data.

The seller catalog query must:

- include only `moderation_status = 'visible'`;
- default to `status IN ('active', 'reserved')`;
- allow exact active, reserved, or sold status filtering;
- never return archived listings;
- join categories and seller data using the existing listing mapping shape;
- use keyset pagination rather than offset pagination;
- apply an explicit status-rank and timestamp/ID cursor condition matching its order.

Keep sqlc row types inside the PostgreSQL adapter and map them to domain or application values.

## HTTP and Server Wiring

Add a seller service dependency to the HTTP adapter configuration and construct it in `cmd/server` from the PostgreSQL seller/profile store and existing catalog store.

Keep `GET /api/v1/me` in the existing authenticated flow, but share its user response mapping with `PATCH /api/v1/me` so both endpoints return the same `User` contract.

Promote the existing listing/chat strict JSON request decoder into a transport-neutral shared helper. Use that same helper for profile writes without changing existing listing or chat behavior.

The shared decoder must:

- require `application/json`, allowing standard parameters such as `charset`;
- enforce the existing 32 KiB endpoint body limit;
- reject unknown fields;
- reject empty bodies and top-level `null`;
- reject more than one JSON value;
- preserve explicit field nulls for the profile request type;
- return consistent safe transport errors.

HTTP error mapping:

- malformed JSON, an empty update object, or an invalid cursor: `400`;
- missing or invalid active session: `401`;
- disabled profile mutation: `403`;
- malformed or unknown seller handle: `404`;
- oversized request body: `413`;
- unsupported content type: `415`;
- invalid location or bio: `422`;
- unexpected persistence failure: safe `500` with the internal cause logged.

Log successful profile updates with request ID, user ID, and changed field names. Do not log location or bio values, Discord identifiers, cookies, session tokens, or other sensitive profile data.

Keep the existing request ID, same-origin protection for unsafe requests, security headers, recovery, and safe error envelope.

## API and Documentation Updates

Update `api/openapi.yaml` to:

- mark all three operations implemented;
- add `created_at` to the required `User` fields;
- add the dedicated `SellerProfile` schema;
- allow `sold` in the seller catalog status parameter;
- document profile body and response errors, including `400`, `413`, and `415`;
- preserve `PublicUser` unchanged.

Update `docs/07-api-contract.md` with the normalization, clearing, public profile, sold history, ordering, pagination, and cursor rules.

Update focused product and engineering documents only where they conflict with this plan. Do not rewrite unrelated sections.

When the implementation is complete, update:

- the current implementation baseline in `AGENTS.md`;
- the repository status in `README.md`;
- the Seller profile and Seller catalog checkboxes in `docs/16-implementation-plan.md`.

No ADR is required because this work follows the accepted architecture and existing catalog model.

## Tests

Write tests before or alongside each behavior and keep them focused on public behavior.

### Domain and application

Use table-driven tests for:

- omitted, null, empty, whitespace-only, and nonempty location and bio values;
- outer trimming with internal spaces and multiline bio preserved;
- exact 100/500-character boundaries and one-character-over failures;
- Unicode character limits;
- partial updates that leave omitted fields unchanged;
- effective no-op updates that do not call persistence or change `updated_at`;
- profile repository failure and disabled-user mapping;
- public sellers with zero listings;
- active count mapping;
- disabled seller public reads;
- malformed and unknown handles;
- active, reserved, sold, archived, and moderation-removed visibility;
- default and status-specific listing options;
- category normalization;
- limit validation;
- cursor version, kind, seller, status, category, and position validation;
- page construction and `next_cursor` behavior;
- repository failures.

### PostgreSQL integration

Using real PostgreSQL, verify:

- profile location and bio can be set, partially changed, and cleared to `NULL`;
- profile updates change `updated_at` only for effective writes;
- disabled users cannot be updated;
- a seller with no listings is returned with count zero;
- active count includes only visible active listings and changes after status or moderation changes;
- disabled sellers remain publicly readable;
- default catalog order is active before reserved, then newest update and highest ID;
- active, reserved, and sold filters return only the selected status;
- archived and moderation-removed listings never appear;
- category filtering and unknown categories behave correctly;
- equal timestamps use ID as a stable tie-breaker;
- page boundaries have no duplicates or gaps;
- cursors cannot cross sellers or filters.

Do not add a seller migration assertion. This phase must work on the existing schema and must not change the schema version.

### HTTP and authorization

Cover:

- successful `GET /api/v1/me` and `PATCH /api/v1/me` response shapes, including string ID and `created_at`;
- successful public seller profile shape and active count;
- successful default, status-specific, category-filtered, and paginated catalog responses;
- missing authentication and same-origin rejection for profile writes;
- disabled-user update rejection;
- explicit null clearing and whitespace clearing;
- unknown fields, empty objects, empty bodies, top-level null, multiple JSON values, invalid JSON, bad content types, and oversized bodies;
- Unicode validation errors with field-specific messages;
- malformed and unknown handles;
- invalid status, category length, limit, and cursor values;
- cursor reuse with a changed seller, status, or category;
- disabled seller public reads;
- safe `500` responses without private profile values in logs.

Add a full real-PostgreSQL HTTP flow using the existing fake Discord login and session test infrastructure.

## Verification and Acceptance

Run:

```text
make sqlc
make docs-check
make vet lint test-race mod-verify
make web-check
TEST_DATABASE_URL='postgres://keebhub:keebhub@localhost:54329/keebhub?sslmode=disable' make test-integration
make docker-build
git diff --check
```

Integration acceptance is not complete if PostgreSQL tests were skipped. `make web-check` is a regression gate only. This phase must not add or modify frontend source or tests.

API acceptance gate:

1. Authenticate a seller and update location and bio through `PATCH /api/v1/me`.
2. Read `GET /api/v1/me` and verify the updated fields and joined date.
3. Without a session, read the seller profile by handle and verify the visible active count.
4. Create active, reserved, sold, archived, and moderation-removed test listings.
5. Verify the default seller catalog contains only active and reserved listings in the required order.
6. Verify sold history is returned only with `status=sold`.
7. Verify category filters and multiple cursor pages have no missing or duplicate rows.
8. Clear profile fields with explicit `null` and whitespace strings and verify they become `null`.
9. Disable the seller, verify profile mutation is rejected, and verify the public profile and allowed catalog history remain readable.

## Out of Scope

Do not implement:

- handle editing or redirects;
- React profile settings or seller catalog pages;
- listing detail, listing creation, or seller management pages;
- profile or listing images beyond the existing Discord avatar URL;
- follower, rating, review, social-feed, or presence features;
- reports or moderation operations;
- new search infrastructure;
- Redis, caching, background jobs, or a separate catalog table;
- database schema changes.
