# ADR-0006: Use a Versioned Modular Go Backend Foundation

- Status: Accepted
- Date: 2026-08-08

## Context

KeebHub needs one small deployable application with explicit domain, application, transport, persistence, and platform boundaries. It also needs a machine-readable HTTP contract and migrations that can run separately from application startup.

## Decision

Use:

- Go 1.26.5;
- Echo v5;
- pgx v5 and sqlc 1.31.1;
- Goose v3 migrations;
- PostgreSQL 18.4;
- zap structured logging;
- OpenAPI 3.1 as the public HTTP contract;
- one server binary and one migration binary in the same application image.

Application packages define the small interfaces they consume. Domain rules do not import Echo, PostgreSQL, sqlc, or zap. All I/O accepts `context.Context` and startup wiring passes dependencies explicitly.

Product schema and queries are introduced with their vertical slices instead of creating unused tables during foundation work.

## Consequences

### Positive

- current supported runtime and framework versions;
- clear testing and ownership boundaries;
- reproducible API, migration, and dependency contracts;
- no application-startup migration race;
- one production release artifact.

### Negative

- generated sqlc files must remain synchronized with schema and queries;
- Echo v5 has fewer years of third-party examples than v4;
- frontend and backend still deploy together.

## Operator boundary

The future moderation tool is a local audited CLI in the application image. It is not a public HTTP API and is implemented only with the trust vertical slice.

## Revisit when

Multiple application replicas, independent frontend/backend release ownership, or a proven new infrastructure need changes the operational model.
