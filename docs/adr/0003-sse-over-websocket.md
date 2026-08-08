# ADR-0003: Use SSE Instead of WebSocket for v1 Chat

- Status: Accepted
- Date: 2026-08-08

## Context

Chat requires:

Client to server:

```text
send message
mark read
```

Server to client:

```text
new message notification
```

Client writes already map cleanly to HTTP.

## Decision

Use:

- REST for client writes and history;
- Server-Sent Events for server push.

One SSE stream exists per authenticated user.

PostgreSQL is authoritative.

## Consequences

### Positive

- simpler protocol;
- works naturally with same-origin session cookies;
- native browser reconnect behavior;
- fewer moving parts than a bidirectional socket protocol.

### Negative

- still requires proxy configuration for long-lived responses;
- event delivery is not durable;
- multi-replica deployments later need cross-instance publication.

## Reliability rule

A message is committed to PostgreSQL before SSE publication.

Missing SSE events are recovered through HTTP history.

## Revisit when

The application develops genuine bidirectional realtime requirements that no longer fit ordinary HTTP writes.
