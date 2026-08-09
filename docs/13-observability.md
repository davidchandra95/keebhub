# Observability

## 1. v1 objective

Observability should answer:

1. Is the application healthy?
2. Are HTTP requests failing?
3. Is PostgreSQL reachable?
4. Is Discord login failing?
5. Are message sends failing?
6. Are SSE clients connecting and disconnecting abnormally?

Do not build an enterprise telemetry stack before deployment needs it.

## 2. Structured logging

Use zap JSON logs in production.

Common fields:

```text
timestamp
level
message
request_id
method
path
status
duration_ms
user_id
error
```

Feature-specific fields when relevant:

```text
listing_id
conversation_id
message_id
oauth_stage
```

## 3. HTTP access logs

Log:

- method;
- normalized route;
- status;
- latency;
- response size if useful;
- request ID.

Do not log full query strings if they may contain sensitive data.

## 4. Authentication logs

Useful events:

```text
oauth_started
oauth_callback_success
oauth_callback_failed
session_created
session_invalid
logout
```

Never include token values.

## 5. Chat logs

Useful operational events:

```text
message_persist_failed
message_created
sse_connected
sse_disconnected
sse_publish_dropped
```

Do not log message body.

## 6. Health checks

### `/healthz`

Process is alive.

Must not depend on external services.

### `/readyz`

Process can serve traffic.

Check database reachability with a short timeout.

Discord availability does not need to block readiness because browsing can still work.

## 7. Metrics

If Prometheus is introduced, minimum metrics:

```text
http_requests_total
http_request_duration_seconds
db_query_errors_total

oauth_attempts_total
oauth_failures_total

listings_created_total
messages_created_total

sse_connections_current
sse_connections_total
sse_publish_dropped_total
```

Avoid labels with user IDs, listing IDs, or conversation IDs because they create unbounded cardinality.

## 8. Alerting

Early deployment may operate without automated alerts.

When traffic becomes meaningful, alert on:

- sustained 5xx rate;
- readiness failure;
- database unavailable;
- OAuth failure rate spike;
- SSE connection failure spike.

## 9. Tracing

Distributed tracing is not necessary for v1 because there is one application process and one database.

If OpenTelemetry is already operationally available, it can be added, but it should not block release.

## 10. Audit needs

Post-v1 moderation actions should have durable audit records. v1 does not have an operator moderation workflow, so ordinary application logs are not a substitute for that future audit trail.

Do not confuse application observability logs with a durable security audit log.
