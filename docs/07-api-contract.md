# API Contract

The machine-readable source of truth is [`api/openapi.yaml`](../api/openapi.yaml). This document explains behavior and examples. A contract change is complete only when both files agree.

## 1. Conventions

Base path:

```text
/api/v1
```

Content type:

```text
application/json
```

JSON request bodies are decoded strictly:

- unknown fields are rejected;
- more than one JSON value is rejected;
- endpoint-specific body-size limits are enforced before decoding.

Authentication:

- secure HttpOnly session cookie;
- no application bearer token exposed to frontend JavaScript.

IDs:

- serialize all database IDs as JSON strings to avoid JavaScript integer precision issues.

Money:

```json
{
  "price_idr": 3000000
}
```

No floating point.

`price_idr` must be between `1` and `10,000,000,000`. Quantity must be between `1` and `1,000,000`.

Timestamps:

- serialize in UTC using RFC 3339;
- clients must not depend on sub-second precision.

Pagination:

- default limit is `20`;
- maximum limit is `100`;
- cursors are opaque, versioned server values;
- a cursor is bound to its filters and sort and is rejected if reused with different query options;
- `newest` uses `(created_at DESC, id DESC)`;
- `price_asc` uses `(price_idr ASC, id ASC)`;
- `price_desc` uses `(price_idr DESC, id DESC)`.

## 2. Error shape

```json
{
  "error": {
    "code": "listing_not_found",
    "message": "Listing was not found.",
    "request_id": "01J4..."
  }
}
```

Validation error:

```json
{
  "error": {
    "code": "validation_failed",
    "message": "Request validation failed.",
    "request_id": "01J4...",
    "fields": {
      "title": "Title is required."
    }
  }
}
```

Do not expose internal database errors.

Every response includes `X-Request-ID`. The same value appears in error responses so users can report a failure without receiving internal details.

## 3. Health

### GET `/healthz`

Liveness.

Response:

```json
{
  "status": "ok"
}
```

### GET `/readyz`

Readiness, including database reachability.

Returns `503 Service Unavailable` with a safe JSON error while PostgreSQL cannot be reached within the readiness timeout.

## 4. Authentication

### GET `/auth/discord`

Starts Discord OAuth2 login.

Query parameter:

```text
return_to=/listings/1001
```

`return_to` must be a relative internal application route. The endpoint creates a 10-minute HttpOnly OAuth-state cookie and redirects to Discord.

### GET `/auth/discord/callback`

Validates `code`, `state`, and the OAuth-state cookie, creates or updates the local user, creates a fixed 30-day local session, clears the OAuth-state cookie, sets `keebhub_session`, and redirects to the validated internal route. A denied or failed callback redirects to `/login` with a safe error code and request ID.

### POST `/auth/logout`

Requires the same-origin policy used by other unsafe requests. It invalidates the server-side session, clears the cookie, and returns `204 No Content`. Repeating logout succeeds.

## 5. Current user

### GET `/api/v1/me`

Authenticated.

Returns `401 Unauthorized` when the session is absent, invalid, expired, or belongs to a disabled user.

Response:

```json
{
  "user": {
    "id": "42",
    "handle": "gunawan",
    "discord_username": "gunawan",
    "display_name": "Gunawan",
    "avatar_url": "https://...",
    "location": "Jakarta Barat",
    "bio": "Keyboard enthusiast",
    "created_at": "2026-08-09T10:00:00Z"
  }
}
```

### PATCH `/api/v1/me`

```json
{
  "location": "Jakarta Barat",
  "bio": "Keyboard enthusiast"
}
```

This is a strict partial update. Omitting a field leaves it unchanged. `null` or an empty trimmed value clears optional `location` or `bio`. Outer whitespace is trimmed, while internal spaces and bio line breaks are preserved. Location is limited to 100 Unicode characters and bio is limited to 500 Unicode characters. An empty object is rejected with `400 Bad Request`, unchanged normalized values return the current user without updating `updated_at`, and malformed, oversized, or non-JSON bodies return `400`, `413`, or `415`.

## 6. Categories

### GET `/api/v1/categories`

Public.

```json
{
  "items": [
    {
      "id": "1",
      "slug": "keyboard",
      "name": "Keyboard"
    }
  ]
}
```

## 7. Listings

### GET `/api/v1/listings`

Public.

Query parameters:

```text
q
category
condition
min_price
max_price
sort=newest|price_asc|price_desc
cursor
limit
```

Default public states:

```text
active,reserved
```

`q` and `category` are trimmed. An empty normalized value is treated as absent.
`q` is a literal case-insensitive substring search, so `%`, `_`, and backslash do
not act as PostgreSQL wildcard characters. An unknown category returns an empty
successful page. Cursors are bound to every search filter and sort; changing
`limit` between pages is allowed.

Response:

```json
{
  "items": [
    {
      "id": "1001",
      "title": "Neo 98",
      "description": "Anodized silver...",
      "price_idr": 3000000,
      "quantity": 1,
      "category": {
        "slug": "keyboard",
        "name": "Keyboard"
      },
      "condition": "used",
      "status": "active",
      "negotiable": true,
      "seller": {
        "id": "42",
        "handle": "gunawan",
        "display_name": "Gunawan",
        "avatar_url": "https://...",
        "location": "Jakarta Barat"
      },
      "created_at": "2026-08-08T12:00:00Z",
      "updated_at": "2026-08-08T12:00:00Z"
    }
  ],
  "next_cursor": "opaque-value"
}
```

### GET `/api/v1/listings/{listing_id}`

Public.

Sold listings remain accessible.

Archived listings are returned only to the listing owner. Other callers receive `404 Not Found`.

Listings removed by moderation are unavailable through normal application endpoints, including to the owner.

### POST `/api/v1/listings`

Authenticated.

```json
{
  "title": "Neo 98",
  "description": "Anodized silver. DM for full condition.",
  "price_idr": 3000000,
  "quantity": 1,
  "category_slug": "keyboard",
  "condition": "used",
  "negotiable": true
}
```

Response: `201 Created`.

The category must exist and be active. Title, description, price, and quantity use the limits in the security and OpenAPI contracts. Omitted `description`, `quantity`, and `negotiable` use `""`, `1`, and `false` respectively.

All listing writes require `application/json`, allow standard content-type parameters, and are limited to 32 KiB. They reject empty bodies, unknown fields, multiple JSON values, and `null` for every listing field. Invalid request syntax returns `400`; field validation returns `422` with a field map.

### PATCH `/api/v1/listings/{listing_id}`

Owner only.

```json
{
  "title": "Neo 98",
  "price_idr": 2900000,
  "negotiable": false
}
```

Partial update.

Omitted fields are unchanged. A present listing field cannot be `null`; `description` can be the empty string.

### POST `/api/v1/listings/{listing_id}/status`

Owner only.

```json
{
  "status": "reserved"
}
```

Recommended instead of permitting arbitrary status changes inside the generic edit endpoint.

## 8. Seller catalog

### GET `/api/v1/users/{handle}`

Public profile. Handles are canonical lowercase values and malformed or unknown handles return `404 Not Found`.

The response contains the public seller identity, location, bio, account creation timestamp, and `active_listing_count`. The count includes only visible active listings. Sellers with no active listings, including disabled sellers, remain publicly readable and return a count of zero.

### GET `/api/v1/users/{handle}/listings`

Public.

Parameters:

```text
status
category
cursor
limit
```

Default:

```text
active,reserved
```

Active listings appear before reserved listings. Each status group is sorted by `updated_at DESC, id DESC`. Sold listings are returned only with `status=sold`; archived and moderation-removed listings never appear. Category is trimmed before exact-slug filtering, and an unknown category returns an empty page.

Results use an opaque, versioned cursor bound to the seller, selected status, and normalized category. Changing any of those values between pages returns `400 Bad Request`; changing `limit` is allowed.

## 9. Seller management

### GET `/api/v1/me/listings`

Authenticated.

Supports all statuses.

Optional `status` filters one seller-controlled status. Results use stable `(updated_at, id)` descending cursor pagination. Removed listings are excluded.

## 10. Discord catalog export

### GET `/api/v1/me/catalog-export?format=discord`

Authenticated.

Response:

```json
{
  "format": "discord",
  "text": "WTS Mechanical Keyboard Stuff\n\nNeo 98 - Rp3.000.000\n...",
  "catalog_url": "https://example.com/u/gunawan"
}
```

Do not make the server post to Discord.

## 11. Conversations

### POST `/api/v1/listings/{listing_id}/conversation`

Authenticated buyer.

Creates or returns an existing conversation.

Response:

```json
{
  "conversation": {
    "id": "9001"
  },
  "created": true
}
```

Repeated request:

```json
{
  "conversation": {
    "id": "9001"
  },
  "created": false
}
```

### GET `/api/v1/conversations`

Authenticated.

Response:

```json
{
  "items": [
    {
      "id": "9001",
      "listing": {
        "id": "1001",
        "title": "Neo 98",
        "status": "active"
      },
      "counterpart": {
        "id": "42",
        "display_name": "Gunawan",
        "avatar_url": "https://..."
      },
      "last_message": {
        "id": "5004",
        "body": "Boleh COD Jakarta Barat?",
        "created_at": "2026-08-08T12:10:00Z"
      },
      "unread_count": 2
    }
  ]
}
```

## 12. Messages

### GET `/api/v1/conversations/{conversation_id}/messages`

Authenticated participant only.

Parameters:

```text
before_id
after_id
limit
```

Use:

- `before_id` for scrolling older history;
- `after_id` for reconnect/catch-up.

Response:

```json
{
  "items": [
    {
      "id": "5004",
      "sender_id": "77",
      "body": "Boleh COD Jakarta Barat?",
      "created_at": "2026-08-08T12:10:00Z"
    }
  ]
}
```

### POST `/api/v1/conversations/{conversation_id}/messages`

Authenticated participant only.

```json
{
  "body": "Boleh COD Jakarta Barat?"
}
```

Response: `201 Created`.

### POST `/api/v1/conversations/{conversation_id}/read`

```json
{
  "last_read_message_id": "5004"
}
```

The backend validates that the message belongs to the conversation.

## 13. SSE

### GET `/api/v1/events`

Authenticated.

Content type:

```text
text/event-stream
```

Example event:

```text
id: 5004
event: message.created
data: {"conversation_id":"9001","message_id":"5004"}
```

Payload stays intentionally small.

On receipt, frontend can:

- append if it already has sufficient context;
- or fetch the message/conversation;
- update inbox unread state.

## 14. Reports

### POST `/api/v1/reports`

Authenticated.

```json
{
  "target_type": "listing",
  "target_id": "1001",
  "reason": "misleading_listing",
  "details": "Price/description appears intentionally misleading."
}
```

Response: `201 Created`.

## 15. HTTP status guidance

| Status | Meaning |
|---|---|
| 200 | Successful read/update |
| 201 | Resource created |
| 204 | Successful operation without body |
| 400 | Malformed request |
| 401 | Authentication required |
| 403 | Authenticated but not authorized |
| 404 | Resource unavailable/not found |
| 409 | State or uniqueness conflict |
| 422 | Domain validation failure |
| 429 | Rate limited |
| 500 | Unexpected server error |

## 16. Idempotency

No generalized idempotency-key infrastructure is necessary for v1.

Use database uniqueness where duplicate operations matter, especially conversation creation.
