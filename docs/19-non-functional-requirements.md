# Non-functional Requirements

## 1. Simplicity

The v1 system must remain deployable with:

```text
one application container
one PostgreSQL container
```

A new infrastructure dependency requires a concrete problem statement.

## 2. Performance

Initial targets are pragmatic, not contractual SLAs.

For normal database-backed endpoints under expected low load:

- p50 should feel immediate to users;
- p95 should generally remain below 500 ms excluding external Discord OAuth calls.

Marketplace queries should use pagination.

Avoid returning entire seller inventory histories without bounds.

## 3. Realtime latency

After a message commit, a connected counterpart should normally receive an SSE notification within roughly one second under normal deployment conditions.

Correctness is more important than realtime speed.

## 4. Availability

SSE disconnection must not make stored chat unavailable.

Core browsing and API operations must not depend on the in-memory broker being historically complete.

## 5. Durability

Once the message API reports a successful committed creation, the message must be recoverable from PostgreSQL even if:

- SSE publication fails;
- counterpart is offline;
- application restarts immediately afterward.

## 6. Security

- HTTPS in production;
- secure server-side sessions;
- authorization on every private resource;
- OAuth state validation;
- no raw HTML UGC;
- parameterized SQL;
- request limits;
- secret hygiene.

## 7. Privacy

Private conversations may be read through v1 application endpoints only by:

- buyer;
- seller.

Any future operator access for abuse or security work requires a separately approved policy and authorization model.

Do not expose chat through public endpoints.

## 8. Accessibility

Primary flows should work with keyboard navigation and screen-reader-compatible semantic controls.

Status must not rely solely on color.

## 9. Responsive support

Web application should support:

- modern desktop browsers;
- modern mobile browsers.

No native mobile application is part of v1.

## 10. Browser support

Target current mainstream Chromium, Safari, and Firefox versions that support EventSource and modern React output.

If specific browser support becomes contractual, define an explicit compatibility matrix later.

## 11. Localization

v1 can launch with one interface language.

The data model should not bake UI labels into persistent records.

Code values remain language-neutral:

```text
active
reserved
sold
used
new
```

Frontend maps those values to display copy.

## 12. Maintainability

- migrations checked into Git;
- sqlc queries checked into Git;
- generated code reproducible;
- exact dependency lockfile committed;
- Astryx version pinned;
- public API models separated from DB models;
- architecture decisions recorded as ADRs.

## 13. Operability

Production must have:

- liveness;
- readiness;
- structured logs;
- backup procedure;
- restore procedure;
- graceful shutdown.

## 14. Cost

v1 intentionally avoids:

- object storage;
- image CDN;
- message broker;
- managed search engine;
- media processing;
- SMS;
- email provider.

The main recurring infrastructure cost should be application compute, PostgreSQL, domain/TLS infrastructure, and backups.
