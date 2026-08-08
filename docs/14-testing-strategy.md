# Testing Strategy

## 1. Testing priorities

Highest-risk behavior:

1. authorization;
2. listing state transitions;
3. conversation uniqueness;
4. message participant validation;
5. Discord OAuth state handling;
6. session handling;
7. reconnect-safe chat persistence.

Visual component coverage is secondary to these domain boundaries.

## 2. Backend unit tests

Test domain/application logic without HTTP or real PostgreSQL where practical.

### Listing

- valid create;
- invalid price;
- invalid quantity;
- invalid condition;
- valid state transitions;
- invalid status;
- only owner may mutate.

### Conversation

- buyer can start conversation;
- seller cannot contact self;
- sold listing rejects new conversation;
- archived listing rejects new conversation;
- repeated creation returns same conversation.

### Message

- seller sends;
- buyer sends;
- third user rejected;
- empty body rejected;
- body too long rejected.

### Read pointer

- advances;
- cannot move backwards;
- message must belong to conversation.

## 3. Database integration tests

Use real PostgreSQL, preferably disposable per CI job.

Test:

- migrations apply;
- sqlc queries;
- constraints;
- unique conversation;
- indexes where query correctness depends on ordering;
- transaction behavior.

Avoid SQLite as a PostgreSQL substitute.

## 4. HTTP tests

Using Echo test facilities or `httptest`:

- request validation;
- status mapping;
- authentication middleware;
- authorization;
- JSON shapes;
- pagination parameters;
- rate-limit behavior where implemented.

## 5. OAuth tests

Do not depend on real Discord in the normal test suite.

Abstract external OAuth client behavior.

Test:

- state generated;
- mismatched state rejected;
- valid code exchange path;
- Discord API failure;
- local user create;
- local user update;
- return URL validation;
- session cookie attributes.

A small manual smoke test against a real Discord application is still required before production.

## 6. SSE tests

Test:

- authenticated connection;
- unauthenticated rejection;
- event format;
- two participants receive notification;
- unrelated user does not;
- multiple subscriptions for one user;
- slow subscriber cannot block send;
- disconnect removes subscription;
- keepalive emitted.

## 7. Persistence-before-publish test

Important scenario:

1. message insert commits;
2. broker publish fails/drops;
3. API still returns committed message correctly;
4. later HTTP catch-up returns message.

This verifies the intended reliability model.

## 8. Frontend tests

Use component and route tests for:

- listing card states;
- listing form validation;
- seller catalog;
- login redirect;
- inbox unread state;
- chat merge/deduplication;
- sold/reserved badge;
- catalog export copy flow.

## 9. End-to-end tests

Critical E2E scenarios:

### Scenario A - seller flow

```text
login stub
-> create listing
-> listing appears in my listings
-> public catalog shows listing
-> mark reserved
-> mark sold
```

### Scenario B - buyer flow

```text
browse anonymously
-> open listing
-> contact seller
-> authenticate
-> conversation created
-> send message
```

### Scenario C - two-party chat

```text
buyer + seller sessions
-> buyer sends
-> seller receives event
-> seller reloads
-> message remains
-> seller responds
```

### Scenario D - Discord export

```text
seller has active/reserved/sold listings
-> generate post
-> output includes active/reserved
-> output excludes sold
-> catalog URL included
```

## 10. Migration tests

CI should start from an empty database and apply every migration in sequence.

When down migrations are maintained, test them selectively. Production rollback strategy should not assume destructive down migrations are always safe.

## 11. CI quality gate

Suggested minimum:

Backend:

```text
go fmt check
go vet
golangci-lint run
go test -race ./...
go mod verify
sqlc generation/check
migration test
```

Frontend:

```text
dependency install from lockfile
typecheck
lint
unit/component tests
build
```

Then build Docker image.

Documentation:

```text
relative Markdown link check
forbidden legacy-name check
OpenAPI lint
Markdown/OpenAPI route agreement check
```

## 12. Manual release checklist

Before production release:

- Discord OAuth callback configured correctly;
- Secure cookie works behind production TLS/proxy;
- public browsing works logged out;
- create/edit listing works;
- seller catalog share URL works;
- catalog export copies correctly;
- two-account chat tested;
- SSE reconnect tested;
- database backup verified.
