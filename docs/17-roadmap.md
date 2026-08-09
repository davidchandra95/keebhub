# Roadmap

This document records possible future directions. It is not a commitment.

The default decision is to keep v1 small until usage proves a requirement.

## v1

Core:

- Discord login;
- public browse/search;
- structured WTS listings;
- seller catalog;
- listing-scoped chat;
- SSE;
- Docker Compose.

No photos.

No transaction infrastructure.

## Candidate v1.1

Only after observing real use:

### Better catalog management

- duplicate listing;
- bulk status update;
- category sections;
- seller-defined catalog ordering.

### Buyer convenience

- favorites/bookmarks;
- saved search;
- share individual listing;
- recently viewed.

### Trust

- report listing/user;
- block user;
- operator account and listing actions;
- seller account age on platform;
- moderation UI.

Do not equate platform account age with transaction trust.

## Candidate v1.2 - Images

Introduce images only if text-first listings meaningfully reduce adoption.

Requirements before implementation:

- object storage;
- image limits;
- upload validation;
- resizing;
- metadata stripping;
- deletion lifecycle;
- moderation;
- cost controls.

Database addition:

```text
listing_images
```

Do not place a single image field directly on `listings`.

## Candidate v1.3 - WTB

Add buyer-request listings:

```text
listing_type = sell | buy
```

Evaluate discovery and chat implications first.

A WTB listing reverses some seller/buyer language, so rename participant semantics if needed.

## Candidate v2 - WTT

Trade listings are materially more complex.

Need to model:

```text
I HAVE
I WANT
optional cash adjustment
```

Examples:

```text
KeyKobo COL
-> GMK Evil Dolch

Keyboard + cash
-> another keyboard
```

Do not force WTT into the simple WTS schema merely to ship it quickly.

## Candidate marketplace trust improvements

Possible:

- Discord guild membership verification;
- verified community role;
- transaction confirmation;
- reputation.

These require careful design.

If the platform still cannot verify a completed transaction, traditional star ratings remain weak evidence.

## Candidate realtime scaling

When multiple application replicas are needed:

1. add cross-instance event propagation;
2. keep PostgreSQL durable source;
3. preserve same HTTP/SSE contract.

Potential:

- PostgreSQL LISTEN/NOTIFY;
- Redis Pub/Sub;
- NATS.

## Candidate notifications

Only after inbox usage demonstrates value:

- web push;
- email, if email identity is later collected;
- Discord notification integration, if users explicitly authorize it.

Do not add notification infrastructure preemptively.

## Features that should require a separate product decision

The following change KeebHub from classifieds toward managed commerce:

- escrow;
- payment processing;
- shipping purchase;
- order lifecycle;
- refunds;
- buyer protection;
- seller fees;
- transaction commissions.

If those become desired, create a new product/architecture proposal instead of casually extending the current schema.
