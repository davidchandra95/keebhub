# Implementation Plan

## 1. Strategy

Build vertical slices that produce usable product behavior early.

Do not implement all infrastructure first and integrate at the end.

## Phase 0 - Repository and foundations

### Deliverables

- Go module;
- OpenAPI 3.1 contract;
- React/Vite frontend;
- Astryx installed with exact pinned version;
- Echo server;
- zap configuration;
- PostgreSQL Compose service;
- migration tooling;
- sqlc;
- `/healthz`;
- `/readyz`;
- Docker multi-stage build;
- basic CI.

### Gate

```text
docker compose up
```

starts a working app and database.

Frontend can call `/api/v1/...` from the same origin.

## Phase 1 - Discord authentication

### Deliverables

- Discord developer application configuration;
- OAuth start;
- OAuth callback;
- state validation;
- user upsert;
- local session table;
- session middleware;
- logout;
- `/api/v1/me`.

### Gate

A user can:

```text
browse logged out
-> login with Discord
-> refresh
-> remain authenticated
-> logout
```

No Discord access token is needed by the frontend.

## Phase 2 - Catalog foundation

### Deliverables

- categories migration and seed;
- listing table;
- create listing;
- edit listing;
- status transition;
- my listings;
- public listing detail.
- public marketplace search, filters, sorting, and cursor pagination.

### Gate

Seller can create:

```text
Neo 98
Rp3.000.000
Used
Active
```

and another logged-out browser can open it.

## Phase 3 - Marketplace discovery

The API portion was completed with the Phase 2 catalog foundation. A later phase
will add the marketplace frontend experience.

### Deliverables

- marketplace list;
- text search;
- category filter;
- condition filter;
- price range;
- sorting;
- pagination.

### Gate

A buyer can find a listing without Discord login.

## Phase 4 - Seller catalog

### Deliverables

- local stable handle;
- seller public profile;
- public `/u/{handle}`;
- seller listing sections;
- profile location/bio.

### Gate

Seller can share one stable catalog URL.

## Phase 5 - Discord catalog export

### Deliverables

- catalog formatter;
- export API;
- owner UI;
- copy-to-clipboard;
- active/reserved filtering;
- catalog link appended.

### Gate

Seller can update inventory once and create a ready-to-paste WTS post.

This phase is considered a core product milestone, not a cosmetic extra.

## Phase 6 - Conversations and persistent messages

### Deliverables

- conversations;
- unique buyer/listing/seller rule;
- inbox;
- message table;
- send message;
- fetch history;
- read pointer.

### Gate

Buyer and seller can communicate by manual refresh even before realtime is enabled.

Build persistent correctness before SSE.

## Phase 7 - SSE realtime

### Deliverables

- in-memory broker;
- one stream per authenticated user;
- `message.created`;
- keepalive;
- reconnect behavior;
- frontend reconciliation;
- multi-tab support.

### Gate

Two browsers exchange messages without manual refresh, and a server restart does not lose committed messages.

## Phase 8 - Reports and security hardening

### Deliverables

- reports;
- disabled user enforcement;
- local audited operator CLI;
- separate listing moderation state;
- immutable moderation action records;
- input limits;
- rate limits;
- origin/CSRF checks;
- safe link rendering;
- sensitive-log review.

### Gate

Public launch threat checklist passes.

## Phase 9 - Production readiness

### Deliverables

- production Compose/env documentation;
- reverse proxy/TLS;
- SSE proxy verification;
- backup;
- restore test;
- production Discord callback;
- smoke test checklist.

### Gate

A fresh server can be provisioned using documented steps.

## 2. Suggested issue breakdown

### Foundation

- [ ] Initialize Go server
- [ ] Initialize Vite/React/Astryx
- [ ] Add Compose PostgreSQL
- [ ] Add migration tool
- [ ] Add sqlc
- [ ] Add zap
- [ ] Add health endpoints
- [ ] Build single app image

### Auth

- [ ] Create Discord OAuth app config
- [x] Implement OAuth state
- [x] Implement callback
- [x] Add users table
- [x] Add sessions table
- [x] Add session middleware
- [x] Add `/me`
- [x] Add logout

### Catalog

- [x] Add categories
- [x] Add listings
- [x] Create listing
- [x] Edit listing
- [x] Change status
- [x] My listings
- [x] Public listing
- [x] Search/filter/sort
- [ ] Seller profile
- [ ] Seller catalog

### Export

- [ ] Define Discord export format
- [ ] Build formatter
- [ ] Add export endpoint
- [ ] Add copy UI

### Chat

- [ ] Add conversations
- [ ] Start/get conversation
- [ ] Add messages
- [ ] Send message
- [ ] Message history
- [ ] Inbox
- [ ] Read pointer
- [ ] SSE broker
- [ ] SSE endpoint
- [ ] Frontend EventSource
- [ ] Reconnect reconciliation

### Trust

- [ ] Add reports
- [ ] Add disabled users
- [ ] Add limits/rate limits
- [ ] Security review

### Operations

- [ ] Production Docker image
- [ ] Production Compose
- [ ] TLS/reverse proxy
- [ ] Backup
- [ ] Restore test
- [ ] Release smoke test

## 3. Definition of done

A task is not done merely because its handler exists.

For user-facing functionality, done means:

- domain rule implemented;
- persistence implemented;
- HTTP contract implemented;
- authorization implemented;
- validation implemented;
- relevant test added;
- frontend state handled;
- error state handled;
- documentation updated if behavior changed.

## 4. Avoid during implementation

Do not add while implementing v1:

- image upload "because it is easy";
- Redis "for future scale";
- WebSocket "because chat normally uses it";
- payment schema;
- order table;
- generic notification service;
- microservices;
- message broker abstraction with multiple unused implementations;
- WTB/WTT complexity;
- seller rating.

Each one changes the product boundary or infrastructure surface.
