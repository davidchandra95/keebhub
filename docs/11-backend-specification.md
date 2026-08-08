# Backend Specification

## 1. Stack

- Go 1.26.5
- Echo v5
- sqlc 1.31.1
- pgx v5
- PostgreSQL 18.4
- Goose v3
- zap
- standard `net/http` compatible SSE implementation
- Docker

## 2. Architectural goals

- one deployable service;
- explicit modules;
- domain rules independent from transport;
- sqlc restricted to database adapter;
- no ORM;
- no service-to-service network calls;
- no distributed event broker in v1.

## 3. Package layout

```text
cmd/
├── server/
├── migrate/
└── admin/                 # trust phase, not foundation

internal/
├── domain/                # entities and domain rules by vertical slice
├── app/                   # use cases and consumer-owned interfaces
├── adapter/
│   ├── httpapi/           # Echo routes, middleware, JSON, and SPA serving
│   └── postgres/          # repositories added by vertical slice
├── generated/
│   └── db/
└── platform/
    ├── config/
    ├── database/
    ├── logging/
    ├── migrations/
    └── server/
```

The Go module is:

```text
github.com/davidchandra95/keebhub
```

Interfaces are defined by the application package that consumes them. Functions that perform I/O accept `context.Context` as their first parameter. Constructors receive concrete dependencies, and package-level mutable state is not used.

## 4. Domain layer

Contains:

- entities/value definitions;
- state transition validation;
- domain-specific errors.

Must not import:

- Echo;
- sqlc;
- PostgreSQL driver;
- zap.

Logging is not a domain responsibility.

## 5. Application layer

Coordinates use cases.

Examples:

```text
CreateListing
UpdateListing
ChangeListingStatus
SearchListings
GetSellerCatalog

StartConversation
ListConversations
SendMessage
MarkConversationRead

LoginWithDiscord
Logout

CreateReport
```

Application layer uses repository interfaces.

## 6. Adapter layer

### HTTP

Responsible for:

- parse;
- authenticate;
- validate transport shape;
- invoke application use case;
- map errors to HTTP;
- serialize response.

Do not put SQL queries or business state transitions in handlers.

### PostgreSQL

Responsible for:

- sqlc calls;
- row mapping;
- transaction implementation;
- persistence-specific errors.

### SSE

Responsible for:

- subscription lifecycle;
- keepalive;
- delivery to connected user streams.

## 7. sqlc organization

Suggested:

```text
db/
├── migrations/
│   └── 00001_foundation.sql
└── queries/
    └── health.sql
```

Keep generated package under:

```text
internal/generated/db
```

Do not let generated database structs become public API models.

## 8. Error taxonomy

Application/domain errors should be identifiable:

```text
ErrNotFound
ErrUnauthorized
ErrForbidden
ErrValidation
ErrConflict
ErrListingUnavailable
ErrSelfConversation
ErrUserDisabled
```

HTTP adapter maps them consistently.

Unexpected errors:

- log full internal context;
- return generic server error;
- attach request ID.

## 9. Request IDs

Middleware:

1. accept trusted request ID only if deployment policy allows;
2. otherwise generate one;
3. add to context;
4. add response header;
5. include in zap log fields.

Suggested header:

```text
X-Request-ID
```

## 10. Logging

Use structured zap fields.

Example logical fields:

```text
request_id
method
path
status
duration_ms
user_id
listing_id
conversation_id
message_id
error
```

Do not log:

- session tokens;
- Discord client secret;
- OAuth access tokens;
- full cookie headers.

Avoid logging full private chat message bodies by default.

## 11. Timeouts

Configure server:

- read header timeout;
- ordinary request timeout where appropriate;
- graceful shutdown timeout.

SSE endpoint is long-lived and must not inherit a normal short request timeout.

## 12. Graceful shutdown

On SIGTERM:

1. stop accepting new ordinary requests;
2. stop/close SSE subscriptions;
3. allow in-flight requests to complete within timeout;
4. close database pool;
5. exit.

Because messages are persisted before realtime publish, shutdown should not lose committed messages.

## 13. Configuration

Environment-driven configuration:

```text
APP_ENV
APP_BASE_URL
HTTP_ADDR
DATABASE_URL
LOG_LEVEL

DISCORD_CLIENT_ID
DISCORD_CLIENT_SECRET
DISCORD_REDIRECT_URI

SESSION_COOKIE_NAME

DB_MAX_CONNS
DB_MIN_CONNS
DB_MAX_CONN_LIFETIME
DB_MAX_CONN_IDLE_TIME

STATIC_DIR
MIGRATIONS_DIR
READINESS_TIMEOUT
SHUTDOWN_TIMEOUT
HTTP_BODY_LIMIT_BYTES
```

Parse once at startup.

Fail fast on missing required production configuration.

Development defaults are safe for local Docker Compose. Production rejects non-HTTPS base URLs, missing Discord credentials once auth is enabled, weak or inconsistent cookie settings, invalid durations, and invalid database pool sizes.

## 14. Static SPA

Production Echo server:

- serves versioned assets;
- sends `index.html` for known frontend routes;
- does not swallow `/api/*`, `/auth/*`, health, or unknown asset paths.

Cache:

- hashed frontend assets can be long-cache;
- `index.html` should use shorter/no immutable caching.

## 15. Rate limits

Minimum application-level limits should cover:

- OAuth/login starts;
- listing creation;
- chat sends;
- reports.

A simple in-memory limiter is acceptable for one-process v1.

Do not design a distributed rate-limit service before multiple replicas exist.

## 16. Operator CLI

The trust phase adds `cmd/admin`, not a public admin HTTP API.

Commands cover:

```text
reports list
reports review
listings remove
listings restore
users disable
users enable
```

Every mutation requires `--actor` and `--reason`. The target change and immutable `moderation_actions` insert share one transaction.
