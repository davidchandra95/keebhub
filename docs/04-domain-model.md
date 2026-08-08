# Domain Model

## 1. Domain language

Use these terms consistently in code and documentation.

### User

A local application identity associated with a Discord account.

### Seller

A user viewed in the context of listings they own.

No separate seller account type is required.

### Buyer

A user viewed in the context of contacting another user's listing.

No separate buyer account type is required.

A user can be both buyer and seller.

### Listing

A seller-owned classified advertisement for an item.

Use `listing`, not `product`, as the primary domain term.

### Catalog

The public collection of listings owned by a seller.

A catalog is a projection, not a separate persisted aggregate in v1.

### Conversation

A private communication context between one buyer and one seller about one listing.

### Message

An immutable plain-text message inside a conversation.

## 2. User entity

Core properties:

```text
id
discord_id
discord_username
display_name
avatar_url
handle
location
bio
status
created_at
updated_at
```

### User status

- `active`
- `disabled`

Disabled users may still have public historical listings, but cannot perform authenticated mutations.

## 3. Listing entity

```text
id
seller_id
category_id
title
description
price_idr
quantity
condition
status
moderation_status
negotiable
created_at
updated_at
```

### Invariants

- seller must exist;
- title must not be blank;
- price must be greater than zero;
- quantity must be at least one;
- condition must be `new` or `used`;
- status must be a valid listing state;
- moderation status must be `visible` or `removed`;
- category must exist.

## 4. Listing state machine

```text
                 +----------+
                 | archived |
                 +----------+
                   ^      |
                   |      v
+--------+      +--------+      +------+
| active | <--> |reserved| ---> | sold |
+--------+      +--------+      +------+
    ^                |              |
    |                |              |
    +----------------+--------------+
            reactivation
```

The application may implement transition validation centrally.

### Why allow sold to active?

Real community sales can fail after a seller prematurely marks an item sold. Reactivation avoids destructive data edits.

## 5. Category

```text
id
slug
name
sort_order
active
```

Initial seed:

| Slug | Name |
|---|---|
| keyboard | Keyboard |
| keycaps | Keycaps |
| switches | Switches |
| parts | Parts |
| accessories | Accessories |
| other | Other |

Categories are rows rather than a PostgreSQL enum so taxonomy can change without enum migrations.

## 6. Conversation aggregate

```text
id
listing_id
seller_id
buyer_id
seller_last_read_message_id
buyer_last_read_message_id
created_at
last_message_at
```

### Invariants

- seller must own listing;
- buyer must not equal seller;
- one `(listing_id, seller_id, buyer_id)` tuple only;
- both participants must exist;
- conversation continues to exist after listing status changes.

## 7. Message entity

```text
id
conversation_id
sender_id
body
created_at
```

### Invariants

- sender must be one of conversation participants;
- body cannot be empty after trimming;
- body length must be bounded;
- messages are immutable for normal product operations.

## 8. Read model

Read state is deliberately simple.

For each conversation store:

```text
seller_last_read_message_id
buyer_last_read_message_id
```

Unread for a participant is any message:

```text
message.id > participant_last_read_message_id
AND message.sender_id != participant_id
```

Do not expose read receipts to the counterpart in v1.

## 9. Report entity

```text
id
reporter_id
target_type
target_id
reason
details
status
created_at
reviewed_at
```

Target types:

- `listing`
- `user`

Report status:

- `open`
- `reviewed`
- `dismissed`
- `actioned`

## 10. Moderation action

Seller-controlled listing status and operator moderation are separate.

```text
id
actor
action
target_type
target_id
report_id
reason
created_at
```

Initial actions:

- `listing_removed`
- `listing_restored`
- `user_disabled`
- `user_enabled`
- `report_reviewed`

Every operator mutation records an immutable moderation action in the same transaction. A moderation-removed listing is hidden from normal application APIs even if its seller status is `active`.

## 11. Session

Authentication session:

```text
id
user_id
token_hash
expires_at
created_at
last_seen_at
```

Store only a cryptographic hash of the bearer session token in PostgreSQL.

The raw token is kept only in the browser cookie.

## 12. Aggregate ownership

| Data | Owner |
|---|---|
| User identity/profile | User domain |
| Listing | Catalog domain |
| Category | Catalog domain |
| Conversation | Chat domain |
| Message | Chat domain |
| Session | Auth domain |
| Report | Trust domain |
| Moderation action | Trust domain |

## 13. Domain events

Avoid building a generalized event bus in v1.

The only realtime application event required is:

```text
message.created
```

It is emitted after the message transaction commits and is used to notify connected SSE clients.

Do not introduce event sourcing.
