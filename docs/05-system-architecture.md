# System Architecture

## 1. Runtime architecture

```text
                         Internet
                            |
                            v
                 +---------------------+
                 |     KeebHub App     |
                 |---------------------|
                 | Echo HTTP server    |
                 | REST API            |
                 | Discord OAuth       |
                 | SSE endpoint        |
                 | In-memory broker    |
                 | Static SPA files    |
                 +----------+----------+
                            |
                            | SQL
                            v
                 +---------------------+
                 |     PostgreSQL      |
                 +---------------------+
```

Docker Compose runs:

1. `app`
2. `postgres`

No other runtime dependency is required for v1.

## 2. Request classes

### Static frontend

```text
GET /
GET /assets/*
GET /u/*
GET /listings/*
```

Echo returns the compiled React application and static assets.

### API

```text
/api/v1/*
```

JSON REST endpoints.

### Authentication

```text
/auth/discord
/auth/discord/callback
/auth/logout
```

### SSE

```text
GET /api/v1/events
```

Uses the same secure application session cookie as REST endpoints.

## 3. Application layering

Recommended backend dependency direction:

```text
HTTP Adapter
    |
    v
Application Use Cases
    |
    v
Domain
    ^
    |
Repository Interfaces
    ^
    |
PostgreSQL Adapter / sqlc
```

Cross-cutting infrastructure:

```text
Config
Logging
Clock
SSE broker
```

### Rule

The domain and application layers must not depend on Echo, sqlc-generated concrete types, or PostgreSQL details.

## 4. Suggested repository structure

```text
.
├── cmd/
│   ├── server/
│   │   └── main.go
│   ├── migrate/
│   │   └── main.go
│   └── admin/                 # added with the trust vertical slice
├── internal/
│   ├── domain/
│   │   ├── user.go
│   │   ├── listing.go
│   │   ├── conversation.go
│   │   ├── message.go
│   │   └── report.go
│   ├── app/
│   │   ├── auth/
│   │   ├── catalog/
│   │   ├── chat/
│   │   └── trust/
│   ├── adapter/
│   │   ├── http/
│   │   ├── postgres/
│   │   └── sse/
│   ├── generated/
│   │   └── db/
│   └── platform/
│       ├── config/
│       └── logging/
├── db/
│   ├── migrations/
│   └── queries/
├── api/
│   └── openapi.yaml
├── web/
│   ├── src/
│   └── ...
├── docs/
├── Dockerfile
├── docker-compose.yml
├── scripts/
└── README.md
```

This structure preserves separation without creating microservices or unnecessary internal RPC boundaries.

## 5. Application modules

### Auth

Responsibilities:

- Discord OAuth redirect;
- callback validation;
- local user upsert;
- session creation;
- session lookup;
- logout;
- current user.

### Catalog

Responsibilities:

- categories;
- listings;
- seller catalogs;
- listing search;
- listing status transitions;
- Discord catalog export.

### Chat

Responsibilities:

- conversation creation;
- authorization;
- message persistence;
- read position;
- inbox queries;
- SSE publication.

### Trust

Responsibilities:

- reports;
- disabled-account enforcement;
- moderation state.

## 6. PostgreSQL as source of truth

All durable state lives in PostgreSQL.

SSE broker state is disposable.

If the application restarts:

- sessions remain valid;
- listings remain valid;
- messages remain valid;
- SSE clients reconnect;
- clients reconcile message history using HTTP.

## 7. In-memory SSE broker

v1 runs one application instance.

The broker can maintain:

```text
map[userID]set[subscriber]
```

When a message commits:

1. publish event to sender user stream;
2. publish event to receiver user stream;
3. if no subscriber exists, do nothing.

No event queue is required because missed updates are recovered from PostgreSQL.

## 8. Scale boundary

Do not add distributed messaging until multiple app replicas are required.

When scaling to multiple replicas, replace only the cross-instance publication mechanism.

Possible future options:

- PostgreSQL `LISTEN/NOTIFY`;
- Redis Pub/Sub;
- NATS.

The HTTP API, domain model, and persistent message model should remain unchanged.

## 9. Frontend delivery

Recommended Docker build:

```text
Stage 1: Node
  -> install pinned dependencies
  -> build React/Vite/Astryx
  -> produce dist/

Stage 2: Go
  -> compile server
  -> copy dist/
  -> serve API + SPA
```

Benefits:

- same origin;
- simpler cookies;
- simpler SSE;
- no CORS configuration for normal deployment;
- two-container Compose topology;
- easy local deployment.

## 10. Why no microservices

The system has:

- one small team;
- one database;
- one deployment unit;
- tightly related features;
- modest expected traffic.

Splitting auth, catalog, and chat into network services would add:

- deployment complexity;
- distributed failure modes;
- tracing requirements;
- authentication propagation;
- versioned service contracts;

without a demonstrated benefit.

Keep modular boundaries in code first.
