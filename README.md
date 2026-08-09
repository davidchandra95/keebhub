# KeebHub Project Documentation

KeebHub is a small C2C classifieds marketplace for mechanical-keyboard enthusiasts in Indonesia. It is intentionally not a full e-commerce platform.

The core loop is:

1. Seller maintains a structured catalog.
2. Seller shares that catalog into existing communities such as Discord.
3. Buyer discovers a listing or seller catalog.
4. Buyer contacts the seller through a listing-scoped chat.
5. Buyer and seller arrange payment, shipping, or COD outside the platform.

## Product principles

1. **Classifieds, not e-commerce**
   - No cart.
   - No checkout.
   - No payment gateway.
   - No shipping integration.
   - No escrow.
   - No platform-managed order lifecycle.

2. **Structured inventory, existing-community distribution**
   - Sellers maintain one source of truth.
   - Discord remains a distribution and community channel.
   - KeebHub generates a clean WTS post that links back to the seller catalog.

3. **Text-first v1**
   - No listing photos.
   - No chat attachments.
   - Keep implementation and moderation surface small.

4. **Discord-native identity**
   - Public browsing does not require login.
   - Discord OAuth2 is the only authentication mechanism in v1.
   - Login is required for selling, chat, account settings, and reporting.

5. **Simple realtime**
   - REST for writes.
   - Server-Sent Events for server-to-client chat updates.
   - PostgreSQL is the source of truth.
   - No Redis, Kafka, NATS, or WebSocket in v1.

## Technology stack

### Backend

- Go 1.26.5
- Echo v5
- sqlc 1.31.1
- pgx v5
- PostgreSQL 18.4
- Goose
- zap
- Server-Sent Events
- Docker

### Frontend

- React 19
- TypeScript
- Vite 8
- Astryx 0.3.0 beta, isolated behind local wrappers
- pnpm 10.34.1
- Docker multi-stage build

### Deployment

- Docker Compose
- One application container
- One PostgreSQL container
- One-shot migration container during deployment
- Application container serves both API and compiled frontend assets

## Documentation index

| Document | Purpose |
|---|---|
| [01 Product Vision and Scope](docs/01-product-vision-and-scope.md) | Product definition, goals, exclusions, success criteria |
| [02 Functional Requirements](docs/02-functional-requirements.md) | Complete v1 feature requirements |
| [03 User Flows](docs/03-user-flows.md) | Seller, buyer, auth, listing, and chat flows |
| [04 Domain Model](docs/04-domain-model.md) | Core domain entities, rules, state transitions |
| [05 System Architecture](docs/05-system-architecture.md) | Runtime architecture and dependency boundaries |
| [06 Database Design](docs/06-database-design.md) | Tables, constraints, indexes, data ownership |
| [07 API Contract](docs/07-api-contract.md) | HTTP behavior and JSON conventions |
| [OpenAPI 3.1 Contract](api/openapi.yaml) | Machine-readable source of truth for public HTTP interfaces |
| [08 Discord Authentication](docs/08-discord-authentication.md) | Discord OAuth2 and application session design |
| [09 Chat and SSE](docs/09-chat-and-sse.md) | Messaging model, SSE protocol, reconnect behavior |
| [10 Frontend Specification](docs/10-frontend-specification.md) | Routes, screens, state, Astryx usage |
| [11 Backend Specification](docs/11-backend-specification.md) | Go package layout, use cases, adapters |
| [12 Security and Trust](docs/12-security-and-trust.md) | Security controls, abuse handling, marketplace trust |
| [13 Observability](docs/13-observability.md) | Logging, metrics, health checks |
| [14 Testing Strategy](docs/14-testing-strategy.md) | Backend, frontend, DB, auth, SSE testing |
| [15 Deployment and Operations](docs/15-deployment-and-operations.md) | Docker Compose, configuration, migration, backup |
| [16 Implementation Plan](docs/16-implementation-plan.md) | Recommended implementation order and acceptance gates |
| [17 Roadmap](docs/17-roadmap.md) | Explicit post-v1 possibilities |
| [18 Discord Catalog Export](docs/18-discord-catalog-export.md) | Copy-as-Discord-post feature |
| [19 Non-functional Requirements](docs/19-non-functional-requirements.md) | Performance, availability, accessibility, compatibility |
| [20 References](docs/20-references.md) | Primary technical references |

### Architecture Decision Records

| ADR | Decision |
|---|---|
| [ADR-0001](docs/adr/0001-classifieds-not-ecommerce.md) | Build classifieds, not full e-commerce |
| [ADR-0002](docs/adr/0002-discord-auth-only-v1.md) | Discord-only authentication in v1 |
| [ADR-0003](docs/adr/0003-sse-over-websocket.md) | SSE instead of WebSocket |
| [ADR-0004](docs/adr/0004-no-images-v1.md) | No listing images in v1 |
| [ADR-0005](docs/adr/0005-single-app-container.md) | Serve SPA and API from one application container |
| [ADR-0006](docs/adr/0006-backend-foundation.md) | Backend runtime, data, migration, and contract baseline |

## Development status

The repository currently contains the backend foundation, Discord authentication, database-backed sessions, and the catalog API foundation: categories, listing management, public detail, and marketplace search. The React client remains at its minimal login/health baseline. Seller profiles, chat, trust, and moderation operations remain planned vertical slices.

## Local development

Requirements:

- Go 1.26.5;
- Node.js 22.22.2 or newer in the Node 22 line;
- pnpm 10.34.1;
- Docker with Compose for PostgreSQL and full-stack checks.

Copy `.env.example` to `.env` when you need to change the safe development defaults. Then use one of these paths:

```text
make web-install
make web-dev
make server
```

Or start the complete container stack:

```text
make docker-up
```

The shell is served at `http://localhost:8080`, liveness at `/healthz`, and database readiness at `/readyz`. PostgreSQL is exposed on local port `54329` by default.

Run the repeatable quality gates with:

```text
make docs-check
make vet lint test-race mod-verify
make web-check
```

Database integration tests require `TEST_DATABASE_URL`. The Compose default is:

```text
postgres://keebhub:keebhub@localhost:54329/keebhub?sslmode=disable
```

See `make help` for focused commands. Generated web output, local environment files, coverage, and build artifacts are ignored.

## Recommended implementation rule

Before adding a feature, ask:

> Does this improve listing inventory, discovery, seller distribution, or buyer-seller connection?

If not, it probably does not belong in v1.
