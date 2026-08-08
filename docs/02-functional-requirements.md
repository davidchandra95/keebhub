# Functional Requirements

## 1. Authentication

### AUTH-001 Public browsing

A visitor must be able to:

- open marketplace pages;
- search listings;
- filter listings;
- open listing details;
- open seller catalogs;

without authentication.

### AUTH-002 Discord login

Authenticated features require Discord OAuth2.

v1 requests only the minimum Discord identity scope required to identify the user.

### AUTH-003 Authenticated actions

Authentication is required to:

- create or edit listings;
- manage listing status;
- manage seller profile;
- contact a seller;
- view inbox;
- send messages;
- mark conversations read;
- report listings or users;
- generate seller-specific catalog exports.

### AUTH-004 Logout

Users can terminate their application session.

## 2. Seller profile

### PROFILE-001 Profile fields

A seller profile contains:

- local user ID;
- Discord ID;
- Discord username snapshot;
- display name;
- Discord avatar URL;
- local unique handle;
- optional free-form location;
- optional short bio;
- created timestamp.

### PROFILE-002 Public catalog URL

Each seller receives a stable local URL:

`/u/{handle}`

The handle is local to KeebHub and must not depend permanently on a mutable Discord username.

### PROFILE-003 Handle behavior

- generated from Discord username on first login when possible;
- normalized to a URL-safe form;
- collision-safe;
- unique case-insensitively;
- can be made editable later, but changing handles is not required in the first implementation.

## 3. Listings

### LIST-001 Create listing

Required fields:

- title;
- price;
- quantity;
- category;
- condition.

Optional fields:

- description;
- negotiable flag.

Defaults:

- status = `active`;
- quantity = 1 when omitted by UI.

### LIST-002 Edit listing

Only the seller who owns the listing can edit it.

Editable fields:

- title;
- description;
- price;
- quantity;
- category;
- condition;
- negotiable.

### LIST-003 Status management

Seller can transition a listing among:

- active;
- reserved;
- sold;
- archived.

Recommended transitions:

| From | To |
|---|---|
| active | reserved, sold, archived |
| reserved | active, sold, archived |
| sold | active, archived |
| archived | active |

Reactivation is allowed because a sale can fail or the seller can relist an item.

### LIST-004 Price

- currency is always IDR in v1;
- price is stored as an integer amount of rupiah;
- decimal prices are not supported;
- minimum is `1`;
- maximum is `10,000,000,000`;
- `0` is not a valid listing price in v1.

### LIST-005 Quantity

- integer;
- minimum 1;
- maximum 1,000,000;
- represents identical units available within the listing;
- package contents such as "120 switches" may still be written in title or description.

Do not attempt SKU or unit-of-measure modeling in v1.

### LIST-006 Category

Initial categories:

- keyboard;
- keycaps;
- switches;
- parts;
- accessories;
- other.

### LIST-007 Condition

Allowed values:

- new;
- used.

### LIST-008 Public visibility

Normal search results include:

- active;
- reserved.

Sold and archived listings are excluded from normal discovery by default.

A sold listing remains accessible by direct URL unless removed for moderation.

### LIST-009 Seller catalog

Seller catalog shows:

- seller identity;
- location;
- bio;
- active listings;
- reserved listings;
- optional sold history section.

Default sort:

1. active before reserved;
2. newest updated listing first.

## 4. Marketplace discovery

### SEARCH-001 Text search

Search over:

- listing title;
- listing description.

v1 implementation can use PostgreSQL `ILIKE` and appropriate indexes if needed. Do not add a separate search service.

### SEARCH-002 Filters

Supported filters:

- category;
- condition;
- minimum price;
- maximum price;
- status, restricted to public states;
- seller handle when browsing a catalog.

### SEARCH-003 Sort

Supported:

- newest;
- price ascending;
- price descending.

### SEARCH-004 Pagination

Use opaque cursor pagination with a default limit of 20 and maximum of 100.

For listing discovery, use these stable combinations:

- `(created_at DESC, id DESC)` for newest;
- `(price_idr ASC, id ASC)` for ascending price;
- `(price_idr DESC, id DESC)` for descending price.

The cursor is versioned and bound to its filters and sort. Reusing it with different query options is rejected.

## 5. Conversations

### CHAT-001 Listing-scoped chat

A buyer initiates a conversation from a listing.

Generic user-to-user DM is not supported.

### CHAT-002 Conversation uniqueness

There can be at most one conversation for:

- listing;
- buyer;
- seller.

Repeated use of "Contact Seller" opens the existing conversation.

### CHAT-003 Self-contact

Seller cannot create a buyer conversation against their own listing.

### CHAT-004 Sold listings

Existing conversations remain usable when a listing becomes sold or archived.

Creating a new conversation for a sold or archived listing should be rejected.

### CHAT-005 Inbox

Inbox shows:

- listing title;
- counterpart;
- latest message preview;
- latest message timestamp;
- unread indicator;
- listing status.

### CHAT-006 Messages

v1 messages are:

- plain UTF-8 text;
- immutable;
- non-editable;
- non-deletable by normal users.

No:

- attachments;
- reactions;
- replies;
- markdown-specific rendering requirements;
- voice;
- media.

URLs may be linkified by the frontend.

### CHAT-007 Read state

Track read state at conversation level using the last-read message ID per participant.

Read receipts are not shown to the other party in v1.

The read position exists only to calculate the local unread state.

## 6. SSE

### SSE-001 Stream

Authenticated users can open one SSE connection:

`GET /api/v1/events`

### SSE-002 Initial event type

v1 requires:

- `message.created`

Additional event types should not be added until there is a concrete requirement.

### SSE-003 Reconnect

The client reconnects automatically.

After reconnect, it must reconcile messages from persistent storage rather than assuming the event stream is lossless.

## 7. Discord catalog export

### EXPORT-001 Generate post

A seller can generate a Discord-friendly text representation of their current catalog.

### EXPORT-002 Default contents

Include:

- seller header;
- active listings;
- reserved listings, clearly marked;
- price;
- negotiable marker when useful;
- seller catalog URL.

Exclude:

- sold listings;
- archived listings.

### EXPORT-003 Copy

Frontend exposes a one-click copy-to-clipboard action.

The platform does not automatically post into Discord in v1.

No bot is required.

## 8. Reports

### REPORT-001 Report target

Authenticated users can report:

- listing;
- user.

### REPORT-002 Reason

Initial reasons:

- scam suspicion;
- prohibited or unrelated item;
- harassment;
- spam;
- misleading listing;
- other.

### REPORT-003 Report storage

Reports are stored for operator review.

No complex moderation dashboard is required initially. A database/admin query workflow is acceptable for the first operational release.

## 9. Admin minimum

The application needs an operational mechanism to:

- remove or restore listings through the separate moderation state;
- disable a user from authenticated actions;
- list and review reports.

Implement this as a local audited operator CLI. Every mutation requires a non-empty operator identity and reason and records the state change in the same database transaction. A full admin UI and admin HTTP API are not required for v1.
