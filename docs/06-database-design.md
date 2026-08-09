# Database Design

## 1. General rules

- PostgreSQL is the only durable database.
- Use `BIGINT GENERATED ALWAYS AS IDENTITY` for internal IDs.
- Treat Discord snowflakes as strings at API boundaries.
- Store IDR prices as integer rupiah in `BIGINT`.
- Use `TIMESTAMPTZ`.
- Prefer text plus `CHECK` constraints over PostgreSQL enums for statuses that may evolve.
- Use migrations for all schema changes.
- sqlc queries are the application database contract.

## 2. Tables

### 2.1 users

```text
users
-----
id                  BIGINT PK
discord_id          TEXT NOT NULL UNIQUE
discord_username    TEXT NOT NULL
display_name        TEXT NOT NULL
avatar_url          TEXT NULL
handle              TEXT NOT NULL
location            TEXT NULL
bio                 TEXT NULL
status              TEXT NOT NULL DEFAULT 'active'
created_at          TIMESTAMPTZ NOT NULL
updated_at          TIMESTAMPTZ NOT NULL
```

Constraints:

```text
status IN ('active', 'disabled')
lower(handle) unique
```

Suggested limits:

- handle: 3 to 40 characters;
- location: 100 characters;
- bio: 500 characters.

The application fails closed when it encounters a `disabled` user. v1 has no user-management workflow that changes this status.

### 2.2 sessions

```text
sessions
--------
id                  BIGINT PK
user_id             BIGINT NOT NULL FK users(id)
token_hash          BYTEA NOT NULL UNIQUE
expires_at          TIMESTAMPTZ NOT NULL
created_at          TIMESTAMPTZ NOT NULL
last_seen_at        TIMESTAMPTZ NOT NULL
```

Indexes:

```text
(user_id)
(expires_at)
```

Expired sessions may be periodically deleted.

### 2.3 categories

```text
categories
----------
id                  BIGINT PK
slug                TEXT NOT NULL UNIQUE
name                TEXT NOT NULL
sort_order          INTEGER NOT NULL
active              BOOLEAN NOT NULL DEFAULT TRUE
```

Initial rows:

```text
keyboard
keycaps
switches
parts
accessories
other
```

### 2.4 listings

```text
listings
--------
id                  BIGINT PK
seller_id           BIGINT NOT NULL FK users(id)
category_id         BIGINT NOT NULL FK categories(id)
title               TEXT NOT NULL
description         TEXT NOT NULL DEFAULT ''
price_idr           BIGINT NOT NULL
quantity            INTEGER NOT NULL DEFAULT 1
condition           TEXT NOT NULL
status              TEXT NOT NULL DEFAULT 'active'
moderation_status   TEXT NOT NULL DEFAULT 'visible'
negotiable          BOOLEAN NOT NULL DEFAULT FALSE
created_at          TIMESTAMPTZ NOT NULL
updated_at          TIMESTAMPTZ NOT NULL
```

Constraints:

```text
price_idr BETWEEN 1 AND 10000000000
quantity BETWEEN 1 AND 1000000
condition IN ('new', 'used')
status IN ('active', 'reserved', 'sold', 'archived')
moderation_status IN ('visible', 'removed')
```

`moderation_status` is present as an internal visibility safeguard, but v1 has no report, operator, or administration workflow that changes it.

Add a unique constraint on `(id, seller_id)`. Conversations use a composite foreign key to this pair so the database, not only application code, enforces that the conversation seller owns the listing.

Recommended indexes:

```text
(status, created_at DESC, id DESC)
(category_id, status, created_at DESC)
(seller_id, status, updated_at DESC)
(price_idr, id)
```

Search can begin with `ILIKE`.

If data volume later requires it, add PostgreSQL full-text or trigram indexing without changing the API.

### 2.5 conversations

```text
conversations
-------------
id                          BIGINT PK
listing_id                  BIGINT NOT NULL FK listings(id)
seller_id                   BIGINT NOT NULL FK users(id)
buyer_id                    BIGINT NOT NULL FK users(id)
seller_last_read_message_id BIGINT NULL
buyer_last_read_message_id  BIGINT NULL
created_at                  TIMESTAMPTZ NOT NULL
last_message_at             TIMESTAMPTZ NULL
```

Constraints:

```text
seller_id <> buyer_id
UNIQUE(listing_id, seller_id, buyer_id)
FOREIGN KEY (listing_id, seller_id) REFERENCES listings(id, seller_id)
```

Indexes:

```text
(seller_id, last_message_at DESC)
(buyer_id, last_message_at DESC)
(listing_id)
```

### 2.6 messages

```text
messages
--------
id                  BIGINT PK
conversation_id     BIGINT NOT NULL FK conversations(id)
sender_id           BIGINT NOT NULL FK users(id)
body                TEXT NOT NULL
created_at          TIMESTAMPTZ NOT NULL
```

Recommended constraints:

```text
char_length(body) BETWEEN 1 AND 2000
```

Application should trim body before validation.

Indexes:

```text
(conversation_id, id DESC)
(sender_id, created_at DESC)
```

### 2.7 Deferred post-v1 reports

Do not add this table or a report API before the first release. The shape below is a future design starting point, not a v1 requirement.

```text
reports
-------
id                  BIGINT PK
reporter_id         BIGINT NOT NULL FK users(id)
target_type         TEXT NOT NULL
target_id           BIGINT NOT NULL
reason              TEXT NOT NULL
details             TEXT NOT NULL DEFAULT ''
status              TEXT NOT NULL DEFAULT 'open'
created_at          TIMESTAMPTZ NOT NULL
reviewed_at         TIMESTAMPTZ NULL
```

Constraints:

```text
target_type IN ('listing', 'user')

reason IN (
  'scam_suspicion',
  'unrelated_item',
  'harassment',
  'spam',
  'misleading_listing',
  'other'
)

status IN ('open', 'reviewed', 'dismissed', 'actioned')
```

A polymorphic `target_id` cannot use one ordinary foreign key. Application-level validation is acceptable for this small trust table.

### 2.8 Deferred post-v1 moderation actions

Do not add this table or an operator workflow before the first release. If it is introduced later, action and audit-row creation must share one transaction.

```text
moderation_actions
------------------
id                  BIGINT PK
actor               TEXT NOT NULL
action              TEXT NOT NULL
target_type         TEXT NOT NULL
target_id           BIGINT NOT NULL
report_id           BIGINT NULL FK reports(id)
reason              TEXT NOT NULL
created_at          TIMESTAMPTZ NOT NULL
```

Constraints:

```text
char_length(actor) BETWEEN 1 AND 200
char_length(reason) BETWEEN 1 AND 2000
target_type IN ('listing', 'user', 'report')
action IN (
  'listing_removed',
  'listing_restored',
  'user_disabled',
  'user_enabled',
  'report_reviewed'
)
```

Audit rows are immutable during normal operations.

## 3. Read position semantics

When participant opens conversation:

```text
latest visible message ID -> participant_last_read_message_id
```

Update must be monotonic:

```text
new_last_read_id >= existing_last_read_id
```

Never move a read cursor backwards.

## 4. Inbox query

Inbox is derived from:

- conversation;
- listing;
- counterpart user;
- latest message;
- participant read pointer.

Avoid a separate inbox table.

## 5. Catalog

Catalog is derived from:

```text
users
JOIN listings
JOIN categories
```

No `catalogs` table is required.

## 6. Deletion policy

Prefer logical product states rather than hard deletion.

### Listings

Use `archived`.

### Users

Use `disabled`.

v1 has no operator action that changes this state.

### Messages

Do not provide ordinary deletion in v1.

### Hard deletion

Reserved for:

- legal/privacy requirements;
- post-v1 moderation;
- operator maintenance.

## 7. Transaction boundaries

### Create message

One transaction should:

1. verify conversation membership as needed;
2. insert message;
3. update conversation `last_message_at`;
4. commit.

SSE publication occurs after commit.

### Create conversation

Use an atomic insert with uniqueness protection.

Concurrent `Contact Seller` requests must return one conversation, not duplicates.

## 8. Future image support

Do not add image columns now.

When photos are introduced later, add a separate table:

```text
listing_images
--------------
id
listing_id
storage_key
sort_order
created_at
```

This preserves one-to-many semantics without changing the `listings` record.
