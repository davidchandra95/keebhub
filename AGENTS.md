# KeebHub Agent Guide

## Purpose

Use this file as the starting context for every task in this repository. Do not scan the whole repository to rediscover the architecture. Read only the task-specific source files and the documents named below.

Keep this file current when a change updates the project structure, main commands, architecture rules, or implementation baseline.

## Project Summary

KeebHub is a small C2C classifieds marketplace for mechanical-keyboard enthusiasts in Indonesia. It connects buyers and sellers, but it is not an e-commerce platform.

The core user loop is:

1. A seller maintains structured inventory in KeebHub.
2. The seller shares a catalog or listing link in an existing community such as Discord.
3. A buyer discovers a listing or seller catalog.
4. The buyer contacts the seller through listing-scoped chat.
5. Both parties arrange payment, shipping, or COD outside KeebHub.

Hard v1 boundaries:

- No cart, checkout, payment gateway, escrow, shipping integration, or platform-managed order lifecycle.
- No listing photos or chat attachments.
- Public browsing works without login.
- Discord OAuth2 is the only v1 login method.
- REST handles writes. Server-Sent Events, or SSE, handles server-to-client chat updates.
- PostgreSQL is the only durable state store.
- No Redis, Kafka, NATS, WebSocket, or microservices in v1.
- One application instance is the planned v1 scale boundary.

Before adding a feature, ask whether it improves listing inventory, discovery, seller distribution, or buyer-seller connection. If it does not, it probably does not belong in v1.

## Current Implementation Baseline

The repository currently contains:

- backend, frontend, database, Docker, documentation, and CI foundations;
- Discord OAuth start and callback handling;
- database-backed hashed sessions, session middleware, logout, and `GET /api/v1/me`;
- liveness and database readiness endpoints;
- a minimal React application with health, login, and not-found pages.
- catalog and marketplace-discovery APIs for categories and listings;
- seller profile and public seller-catalog APIs;
- backend-only listing-scoped chat: persistent conversations, inboxes, messages, read pointers, and authenticated SSE notifications.

The catalog API foundation covers categories, listing management, public listing detail, marketplace discovery, seller profiles, and public seller catalogs. The React client remains at its minimal login and health baseline. Discord catalog export, chat frontend screens, and reconnect handling are documented but not yet implemented. Reporting, blocking, and operator moderation are deliberately deferred until after the first release. The OpenAPI contract includes planned v1 endpoints, so a documented endpoint is not proof that its code exists.

When a vertical slice becomes complete, update this section, `README.md`, and the checklist in `docs/16-implementation-plan.md` as part of the same change.

## Source of Truth

Use this order when deciding intended behavior:

1. `api/openapi.yaml` for the public HTTP contract.
2. Accepted decisions in `docs/adr/` and the product boundaries in `docs/01-product-vision-and-scope.md`.
3. The focused specification in `docs/` for the feature area.
4. Current source and tests for what is implemented now.

If these sources disagree, do not silently choose one. Explain the mismatch and fix all affected sources when the task allows it.

Useful focused documents:

| Task | Read first |
|---|---|
| Product scope or requirements | `docs/01-product-vision-and-scope.md`, `docs/02-functional-requirements.md` |
| Domain rules or state changes | `docs/04-domain-model.md` |
| Architecture or package boundaries | `docs/05-system-architecture.md`, `docs/11-backend-specification.md` |
| Database change | `docs/06-database-design.md` |
| HTTP API change | `api/openapi.yaml`, `docs/07-api-contract.md` |
| Discord authentication | `docs/08-discord-authentication.md` |
| Chat or SSE | `docs/09-chat-and-sse.md` |
| Frontend or UX | `docs/10-frontend-specification.md` |
| Security, authorization, or trust | `docs/12-security-and-trust.md` |
| Tests | `docs/14-testing-strategy.md` |
| Docker, configuration, or deployment | `docs/15-deployment-and-operations.md` |
| Work order or current scope | `docs/16-implementation-plan.md` |

Do not read every document for a small task.

## Technology and Runtime

Backend:

- Go 1.26.5
- Echo v5
- pgx v5 and sqlc 1.31.1
- PostgreSQL 18.4
- Goose migrations
- zap structured logging

Frontend:

- React 19 and strict TypeScript
- Vite 8
- React Router
- Astryx 0.3.0 beta with StyleX, isolated behind local wrappers
- pnpm 10.34.1 on Node.js 22.22.2 or newer in the Node 22 line
- Vitest and Testing Library

Deployment:

- The Go application serves both `/api` and the compiled SPA from one origin.
- Docker Compose runs `app` and `postgres`, plus a one-shot `migrate` service before `app` starts.
- PostgreSQL is exposed on local port `54329` by default.
- The application is served at `http://localhost:8080` by default.

Use pinned versions from `go.mod`, `web/package.json`, `web/pnpm-lock.yaml`, `Makefile`, and `.github/workflows/ci.yml`. If this summary becomes stale, update it instead of adding another version source.

## Repository Map

```text
cmd/server/                 process wiring and graceful HTTP server startup
cmd/migrate/                explicit Goose migration command
internal/domain/            entities, domain rules, and domain errors
internal/app/               use cases and consumer-owned interfaces
internal/adapter/httpapi/   Echo routes, middleware, JSON, auth, and SPA serving
internal/adapter/postgres/  pgx and sqlc-backed persistence adapters
internal/adapter/discord/   Discord OAuth provider adapter
internal/generated/db/      generated sqlc code, never edit by hand
internal/platform/          config, database, logging, migrations, and server lifecycle
internal/testutil/          shared test infrastructure
db/migrations/              versioned PostgreSQL schema changes
db/queries/                 hand-written SQL used by sqlc
api/openapi.yaml            machine-readable public HTTP contract
web/src/                    React application
docs/                       product and engineering specifications
docs/adr/                   accepted architecture decisions
scripts/check-docs.mjs      documentation and API agreement checks
```

## Architecture Rules

Backend dependency direction is:

```text
HTTP or provider adapter -> application use case -> domain
PostgreSQL adapter ------> application-owned interface
```

Follow these rules:

- `internal/domain` must not import Echo, pgx, sqlc-generated types, zap, or other infrastructure.
- `internal/app` coordinates use cases and defines the small interfaces it consumes.
- HTTP handlers parse, authenticate, validate transport data, call application code, map errors, and serialize responses. They do not contain SQL or business state transitions.
- PostgreSQL adapters own sqlc calls, row mapping, transactions, and persistence-specific error handling.
- Do not expose generated database structs as domain or public API models.
- Pass dependencies through constructors. Avoid package-level mutable state.
- I/O functions accept `context.Context` first and respect cancellation and timeouts.
- Persist durable changes before publishing disposable realtime events.
- Keep one clear implementation path. Do not add a second wrapper or parallel flow to avoid fixing the real flow.
- Keep functions small, names clear, errors explicit, and failure behavior observable.

## HTTP, Security, and Data Rules

- API paths use `/api/v1`. Auth paths use `/auth`. Health paths are `/healthz` and `/readyz`.
- JSON decoding is strict. Reject unknown fields, extra JSON values, and oversized bodies.
- Serialize database IDs as JSON strings because JavaScript cannot safely represent every `BIGINT`.
- Store and send IDR prices as integer rupiah. Never use floating point for money.
- Serialize timestamps in UTC using RFC 3339.
- Use the common safe error shape with an error code, message, and request ID. Never return internal database errors.
- Every response carries `X-Request-ID`. Logs use the same request ID.
- Authentication uses a secure HttpOnly application cookie. Do not add frontend bearer-token storage.
- Unsafe cookie-authenticated requests must keep the same-origin checks.
- Validate authorization on the server even when the frontend hides an action.
- Render listing and chat content as text, never raw HTML.
- Use parameterized SQL. Dynamic sort choices must come from an allowlist.
- Never log or commit session tokens, cookies, OAuth codes, Discord access tokens, Discord client secrets, database passwords, private chat bodies, or production credentials.
- Treat `.env` as local and sensitive. Use `.env.example` for documented defaults and examples.

## Database and sqlc Workflow

- Make every schema change through a new file in `db/migrations/`.
- Prefer backward-compatible, additive migrations. Do not assume a destructive down migration is a safe production rollback.
- Put hand-written queries in `db/queries/`.
- After changing schema or queries, run `make sqlc` and include the regenerated output.
- Never edit files in `internal/generated/db/` by hand.
- Keep sqlc types inside the PostgreSQL adapter and map them to domain or application types.
- Use real PostgreSQL for database integration tests. Do not replace it with SQLite.
- Do not delete or reset the local PostgreSQL volume unless the user explicitly asks.

## API and Documentation Workflow

For an HTTP contract change:

1. Update `api/openapi.yaml`.
2. Update the matching explanation in `docs/07-api-contract.md`.
3. Update implementation and tests.
4. Run `make docs-check`.

The documentation check validates relative links, project naming, OpenAPI syntax, and agreement between documented and OpenAPI operations.

Create or update an ADR when changing an accepted architecture decision. Do not rewrite an important decision only in implementation code.

Do not manually edit generated artifacts or `CHANGELOG.md` files.

## Frontend Rules

- Keep TypeScript strict and define explicit request and response types at the HTTP boundary.
- Do not import Go or sqlc-generated types into the frontend.
- Keep Astryx behind small local wrappers such as `AppButton` and `AppCard` when a stable adaptation point is useful. Do not wrap every component.
- Do not couple domain or feature logic to Astryx beta APIs.
- Use React state and route-level data first. Do not add a global state framework without a clear need.
- Keep the interface mobile responsive, keyboard accessible, and clear without relying on color alone.
- Browsing remains public. Ask for login only when the user starts an authenticated action.
- Use native `EventSource` for future SSE support. Reconcile durable state from HTTP after reconnects.
- Do not add blank image areas because v1 listings are text-only.

## Testing and Commands

Start with focused tests for the changed package or component, then run all relevant gates.

```text
make help                 list supported commands
make server               run the Go server
make web-dev              run the Vite development server with API proxying
make docker-up            build and start the full local stack
make docker-down          stop the stack without deleting its named volume

make docs-check           check docs and OpenAPI agreement
make fmt                  format Go code, modifies files
make vet                  run go vet
make lint                 run pinned golangci-lint
make test                 run Go tests without the race detector
make test-race            run Go tests with the race detector
make mod-verify           verify Go module downloads
make web-check            typecheck, lint, test, and build the frontend
make ci                   run the main local documentation, Go, and web gates
```

Database integration tests require a real PostgreSQL database and `TEST_DATABASE_URL`. The local default is:

```text
postgres://keebhub:keebhub@localhost:54329/keebhub?sslmode=disable
```

Run them with `make test-integration` after PostgreSQL is available. Tests that require this variable may skip during ordinary `make test-race`, so a passing unit suite is not proof that database integration passed.

Use `make docker-build` or `make docker-up` when a change affects the Dockerfile, Compose, migrations, static asset serving, runtime configuration, or service startup.

Report each gate that ran and its result. Also report important gates that could not run and why.

## Working Rules

- Check `git status --short` before editing and preserve unrelated user changes.
- Keep changes focused on the requested task. Mention unrelated problems without fixing them unless the user asks.
- Do not stage, commit, push, create branches, or open pull requests unless the user explicitly asks.
- Do not use destructive Git commands or delete local data to make a test pass.
- Prefer correctness, simplicity, security, clear ownership, and long-term maintenance over short-term typing speed.
- For a read-only review or diagnosis, do not change files. Note that `make fmt` and some generation commands modify the working tree.
- Before calling a feature complete, verify its domain behavior, persistence, HTTP contract, authorization, validation, tests, frontend state, error state, and documentation where each part applies.
