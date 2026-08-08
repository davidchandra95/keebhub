# ADR-0005: Serve Frontend and API from One Application Container

- Status: Accepted
- Date: 2026-08-08

## Context

The project uses:

- React/Vite/Astryx frontend;
- Go/Echo backend;
- cookie authentication;
- SSE;
- Docker Compose.

A separate frontend service would introduce additional routing and cross-origin considerations without a clear v1 benefit.

## Decision

Use a multi-stage Docker build.

The final Go application image contains and serves the compiled frontend assets.

Docker Compose runs:

```text
app
postgres
```

## Consequences

### Positive

- same-origin API;
- simpler cookie auth;
- simpler SSE;
- no CORS for normal deployment;
- fewer containers;
- simpler release artifact.

### Negative

- frontend and backend deploy together;
- a frontend-only release still rebuilds app image.

For the expected project size, this tradeoff is desirable.

## Revisit when

Frontend and backend require independent release ownership or delivery topology.
