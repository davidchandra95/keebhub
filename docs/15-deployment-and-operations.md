# Deployment and Operations

## 1. Deployment topology

```text
docker compose
|
+-- app
|   +-- Go/Echo
|   +-- React build
|   +-- REST
|   +-- SSE
|
+-- postgres
+-- migrate, one-shot before app startup
```

A reverse proxy may be placed in front if needed for TLS and domain routing.

## 2. Dockerfile strategy

Multi-stage image:

### Stage 1 - frontend

- Node;
- install dependencies from lockfile;
- build React/Vite;
- output `dist`.

### Stage 2 - backend build

- compile server and migration binaries with Go 1.26.5.

### Stage 3 - runtime

- minimal runtime image;
- copy Go binary;
- copy frontend `dist`;
- run as non-root where practical.

## 3. Docker Compose services

### app

Depends on PostgreSQL readiness.

Environment:

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
```

### postgres

- persistent volume;
- health check;
- production credentials supplied outside Git.

PostgreSQL is pinned to 18.4 in deployment configuration and is updated through normal reviewed dependency maintenance.

### Discord OAuth configuration

Local development uses the untracked `.env` file. `make server` reads it only when `APP_ENV` is unset or `development`, and Docker Compose uses it for variable substitution. Neither path overrides variables injected by the process or deployment platform. `.env` is excluded from Git and Docker build context.

Before a production deployment, choose the final HTTPS application origin and configure all of the following together:

```text
APP_ENV=production
APP_BASE_URL=https://<production-origin>
DISCORD_REDIRECT_URI=https://<production-origin>/auth/discord/callback
```

Inject `DISCORD_CLIENT_ID` and `DISCORD_CLIENT_SECRET` through the deployment secret store. Then register that exact redirect URL in the existing Discord Developer Portal application. Do not register a placeholder or wildcard callback URL. This production registration remains pending until the production origin is chosen.

## 4. Migrations

Choose one explicit migration execution strategy:

### Recommended simple approach

A dedicated migration command runs during deployment before starting the new app image.

Do not let every application replica race to apply schema changes when multi-replica deployment eventually exists.

For local Compose, a one-shot migration service is acceptable.

The migration service uses the same application image as the server and must finish successfully before the app starts.

## 5. Same-origin deployment

Recommended:

```text
https://market.example/
https://market.example/api/v1/*
https://market.example/auth/*
https://market.example/api/v1/events
```

Benefits:

- simpler session cookies;
- native EventSource auth;
- no CORS configuration;
- simpler OAuth return flow.

## 6. Reverse proxy requirements for SSE

If using Nginx, Caddy, Traefik, or another proxy:

- allow long-lived HTTP responses;
- avoid response buffering for SSE;
- configure sensible idle timeout;
- pass through streaming promptly.

Test SSE through the real production proxy, not only localhost.

## 7. TLS

Production authentication cookie uses `Secure`, therefore production deployment requires HTTPS.

## 8. Database backup

At minimum:

- scheduled PostgreSQL backup;
- retention policy;
- restore test.

The database contains private chats and should be protected accordingly.

## 9. Restore procedure

Document operator steps:

1. stop writes if necessary;
2. provision compatible PostgreSQL;
3. restore backup;
4. run expected migrations;
5. start application;
6. verify readiness;
7. test login, catalog, and message history.

## 10. Application rollback

Prefer backward-compatible migrations.

Release process:

```text
migration
-> deploy app
-> health/readiness
-> smoke test
```

If application rollback is needed, previous binary/image should still work against the migrated schema whenever practical.

## 11. Log collection

v1 may use Docker logs.

Production should eventually forward logs to a searchable system if operational needs justify it.

## 12. Resource expectations

Initial system should be inexpensive.

Primary resource consumption:

- PostgreSQL;
- Go process;
- long-lived SSE connections.

No image storage and no media processing significantly reduce infrastructure requirements.

## 13. SSE restart behavior

Application restart disconnects all clients.

This is acceptable.

Browsers reconnect and messages remain in PostgreSQL.

Do not preserve in-memory SSE broker state across deployments.

## 14. Scale trigger

Move beyond one app replica only when metrics or reliability requirements justify it.

At that time review:

- cross-instance SSE publication;
- shared rate limiting if required;
- load balancer SSE behavior;
- graceful connection draining.

Do not prebuild this infrastructure in v1.
