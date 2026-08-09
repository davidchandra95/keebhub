# Phase 6-7 Backend Chat and SSE

Status: Ready for implementation

## Summary

Build the complete backend slice for listing-scoped buyer and seller chat:

- Create or return one conversation for each listing, seller, and buyer tuple.
- Persist immutable text messages and participant read positions in PostgreSQL.
- Provide an inbox, message history, catch-up reads, message sends, and monotonic read updates through REST.
- Notify every connected tab for both participants through one authenticated SSE stream per user.
- Keep PostgreSQL authoritative. SSE is a best-effort update signal published only after a message commits.

This plan combines the backend work from Phases 6 and 7 while keeping two acceptance gates. Persistent chat must work through manual refresh before SSE is enabled.

Keep this phase backend-only. Do not add or change React pages, routes, components, browser state, or `EventSource` client code. Message rate limiting remains part of the later security-hardening phase. Reports, moderation, and blocking are post-v1 work; attachments, general direct messages, group chat, WebSocket, Redis, NATS, and multi-process event delivery are outside this phase.

No new external dependencies are needed.

## Contract Clarifications

Use the existing OpenAPI paths and response models, with these decisions made explicit before implementation:

- A new conversation can start only when the listing is `active` or `reserved` and its moderation status is `visible`.
- A conversation remains available after its listing becomes sold, archived, or removed. A listing state change does not delete or disable existing chat history.
- Inbox order is `COALESCE(last_message_at, created_at) DESC, id DESC`.
- Inbox pagination uses an opaque, versioned cursor containing the activity timestamp and conversation ID. Return `next_cursor: null` when there is no next page.
- Message responses are always ordered by `id ASC`.
- A message request without `before_id` or `after_id` returns the latest page, reordered into ascending ID order for the response.
- `before_id` returns the closest older page and `after_id` returns the earliest newer page. Clients can repeat catch-up using the last returned ID.
- Supplying both `before_id` and `after_id` returns `400 Bad Request`.
- Message bodies are trimmed at the outer edges before validation and storage. Internal spaces and line breaks are preserved.
- A read pointer may target only a message from the same conversation and may move forward or remain unchanged, never backward.
- Disabled users may read existing conversations but cannot start a conversation or send a message.

Update `api/openapi.yaml`, `docs/07-api-contract.md`, and `docs/09-chat-and-sse.md` with these rules during implementation. Also correct the API contract example so `ConversationSummary.last_message` includes the required `sender_id` field.

## Data Model

Add `db/migrations/00004_chat_and_sse.sql`.

### Conversations

Create `conversations` with:

- Identity `BIGINT` primary key.
- `listing_id BIGINT NOT NULL`.
- `seller_id BIGINT NOT NULL`.
- `buyer_id BIGINT NOT NULL`.
- Nullable `seller_last_read_message_id` and `buyer_last_read_message_id`.
- `created_at TIMESTAMPTZ NOT NULL DEFAULT now()`.
- Nullable `last_message_at TIMESTAMPTZ`.

Enforce:

- Both participants reference `users(id)`.
- `(listing_id, seller_id)` references `listings(id, seller_id)` so the recorded seller owns the listing.
- `seller_id <> buyer_id`.
- `UNIQUE (listing_id, seller_id, buyer_id)`.
- `last_message_at IS NULL OR last_message_at >= created_at`.

Add indexes for:

- Seller inbox order using `seller_id`, `last_message_at`, `created_at`, and `id`.
- Buyer inbox order using `buyer_id`, `last_message_at`, `created_at`, and `id`.
- Listing lookup.

### Messages

Create `messages` with:

- Identity `BIGINT` primary key.
- `conversation_id BIGINT NOT NULL` referencing `conversations(id)`.
- `sender_id BIGINT NOT NULL` referencing `users(id)`.
- `body TEXT NOT NULL`.
- `created_at TIMESTAMPTZ NOT NULL DEFAULT now()`.

Enforce `char_length(body) BETWEEN 1 AND 2000`. Add a unique constraint on `(id, conversation_id)` so conversation read pointers can use composite foreign keys. After both tables exist, add nullable composite foreign keys that require each read-pointer message to belong to its conversation.

Index messages by `(conversation_id, id)` for history, catch-up, unread counts, and read validation.

The down migration removes read-pointer foreign keys before dropping messages and conversations. It must not remove listings or users.

## Domain and Application Design

Add chat-specific domain values under `internal/domain` for:

- conversation identity and participant checks;
- message body normalization and Unicode-length validation;
- inbox and message-page options;
- message-created event data.

Add `ChatService` under `internal/app`. Define small, consumer-owned interfaces for the persistence operations it needs and a realtime publisher interface implemented by the SSE broker.

Required use cases are:

- `StartConversation`
- `ListConversations`
- `ListMessages`
- `SendMessage`
- `MarkConversationRead`

Application behavior:

- Load the listing before starting a conversation and apply visibility, status, self-contact, and disabled-user rules.
- Treat database uniqueness as the concurrency authority. Concurrent start requests return the same conversation, with exactly one response reporting `created: true`.
- Use the authenticated user ID to select the participant role. Never accept participant IDs from request JSON.
- Return not found for conversation reads or writes by a non-participant so private resource existence is not exposed.
- Allow both participants to continue messaging after the listing status changes.
- Insert a message and update `conversations.last_message_at` in one repository transaction.
- Publish `message.created` only after the repository transaction returns successfully.
- Send the event to both the buyer and seller so the sender's other tabs also update.
- Do not fail or delay a successful message response when an event is dropped.
- Advance only the authenticated participant's read pointer.
- Use an injected UTC clock for deterministic mutation timestamps.

Return identifiable validation, not-found, conflict, unavailable-listing, self-conversation, and disabled-user errors so the HTTP adapter can map them without inspecting persistence errors.

## PostgreSQL and sqlc

Add chat queries under `db/queries/` and regenerate `internal/generated/db/` with `make sqlc`. Never edit generated code manually.

Persistence operations must cover:

- Atomic insert-or-select conversation creation.
- Participant-scoped conversation lookup.
- Seller and buyer inbox pages with the same stable activity ordering.
- Latest-message selection and unread counts without a separate inbox table.
- Latest message page, older history, and forward catch-up.
- Transactional message insert plus `last_message_at` update.
- Participant-specific, monotonic read-pointer update after validating the target message belongs to the conversation.

Use default limit 20 and maximum limit 100 for inbox and message reads. Fetch `limit + 1` inbox rows to produce `next_cursor`.

For message history:

- Query the initial and older pages in descending order for efficient limiting, then reverse them before mapping the response.
- Query catch-up pages in ascending order.
- Do not load all messages to calculate a page.

Keep sqlc rows inside the PostgreSQL adapter and map them to application/domain values. Wrap multi-statement message creation in a pgx transaction and roll it back on any insert or conversation-update failure.

## SSE Broker

Add one process-local broker owned by the server process.

Broker behavior:

- Store a set of subscriptions per authenticated user ID.
- Give each subscription a bounded channel with capacity 16.
- Support multiple subscriptions for the same user for multi-tab behavior.
- Publish without blocking. If a subscription channel is full, remove and close that subscription so the browser reconnects and reconciles from PostgreSQL.
- Make unsubscribe idempotent so request cancellation and broker shutdown can race safely.
- Close all subscriptions when the broker closes and reject later subscriptions.
- Protect subscription state with one clear synchronization strategy and never send on a channel after it closes.

The event written to the stream is:

```text
id: <message_id>
event: message.created
data: {"conversation_id":"<conversation_id>","message_id":"<message_id>"}
```

The broker owns operational logs for `sse_connected`, `sse_disconnected`, and `sse_publish_dropped`. Include user ID and relevant message or conversation IDs, but never include message bodies.

## HTTP and SSE Endpoints

Implement and mark these operations as implemented in `api/openapi.yaml`:

- `POST /api/v1/listings/{listing_id}/conversation`
- `GET /api/v1/conversations`
- `GET /api/v1/conversations/{conversation_id}/messages`
- `POST /api/v1/conversations/{conversation_id}/messages`
- `POST /api/v1/conversations/{conversation_id}/read`
- `GET /api/v1/events`

The first five are the chat REST routes. `/api/v1/events` is the separate SSE stream route.

Preserve these response rules:

- IDs are JSON strings.
- Timestamps are UTC RFC 3339.
- Conversation creation returns `201` when created and `200` when an existing row is returned.
- Message send returns `201` with the committed message.
- Read update returns `204`, including when the pointer is already equal to or ahead of the requested message.

Message-send and read requests use the existing strict JSON decoder:

- Require `application/json`, including support for standard parameters such as `charset`.
- Reject unknown fields, empty bodies, multiple JSON values, and explicit `null`.
- Apply the existing 32 KiB endpoint body limit.
- Return field-specific `validation_failed` errors for invalid bodies.

HTTP error mapping:

- Missing session: `401`.
- Missing, malformed, hidden, or non-participant resource: `404`.
- Self-conversation or unavailable listing state: `409`.
- Invalid cursor, incompatible message parameters, or malformed JSON: `400`.
- Invalid message body or read target: `422`.
- Oversized body: `413`.
- Unsupported content type: `415`.
- Unexpected persistence failure: safe `500` with full internal logging.

The SSE handler must:

- Require an authenticated session.
- Set `Content-Type: text/event-stream`, `Cache-Control: no-cache`, and `X-Accel-Buffering: no`.
- Flush headers immediately after subscribing.
- Write one `: keepalive` comment every 20 seconds while idle.
- Flush every event and keepalive.
- Stop on request cancellation, subscription closure, write failure, or broker shutdown.
- Avoid ordinary short request timeouts.

## Server Wiring and Shutdown

Construct one broker in `cmd/server` and inject it as both the application event publisher and HTTP SSE subscription source.

Extend the server lifecycle with an explicit pre-shutdown hook. On process cancellation:

1. close the broker so existing SSE handlers exit and new subscriptions are rejected;
2. call normal HTTP shutdown to stop accepting new work;
3. wait for remaining ordinary in-flight requests within the existing shutdown timeout;
4. close the database pool after the runner returns.

The hook must be safe when no broker is configured and safe if called more than once. Add lifecycle tests proving a connected SSE request does not hold shutdown open until timeout.

## Observability and Security

Log successful message creation with request ID, conversation ID, message ID, and sender ID. Log persistence failures with the same identifiers when available.

Never log:

- message bodies;
- session cookies or token values;
- full private-chat query data.

Keep the current server-side authorization, same-origin checks for unsafe methods, request IDs, recovery middleware, and safe error envelope. Rendered chat safety belongs to the later frontend phase, but the API must continue treating message bodies as plain text.

Do not implement message rate limiting in this phase. The existing OpenAPI `429` response may remain documented for the later security-hardening work.

## Tests

### Domain and application

Use table-driven tests for:

- Active and reserved listing conversation creation.
- Sold, archived, removed, missing, self-owned, and disabled-user rejection.
- Existing conversation continuity after every listing status change.
- Concurrent uniqueness result mapping.
- Seller, buyer, and unrelated-user participant checks.
- Empty, whitespace-only, 2,000-character, 2,001-character, Unicode, and multiline bodies.
- Outer trimming with internal whitespace preservation.
- Persistence failure before publish.
- Successful persistence followed by dropped publication.
- Both participant IDs included in publication.
- Read pointer advance, same-value no-op, backward no-op, wrong-conversation message, and unrelated-user rejection.
- Inbox and message option validation.

### PostgreSQL integration

Using real PostgreSQL, verify:

- Migration version becomes 4 and applies from an empty database.
- Every foreign key, check constraint, uniqueness rule, and index exists and rejects invalid data.
- Concurrent conversation starts create exactly one row.
- Seller and buyer inboxes return correct counterpart, listing, last message, unread count, and stable pagination.
- Empty conversations and equal activity timestamps use deterministic ordering.
- Initial, older, and catch-up message pages have no duplicates, gaps, or wrong ordering at boundaries.
- Participant isolation is enforced by repository queries.
- Message insert and `last_message_at` update commit or roll back together.
- Read pointers cannot reference another conversation and never move backward.

### HTTP

Cover:

- Every successful status and JSON shape.
- ID-as-string and UTC timestamp serialization.
- Missing sessions, disabled-user writes, and cross-participant access.
- Enumeration-safe `404` behavior.
- Cursor boundaries and malformed cursors.
- `before_id`, `after_id`, both together, invalid IDs, default limits, and maximum limits.
- Unknown fields, multiple JSON values, nulls, empty bodies, unsupported content type, and oversized bodies.
- Same-origin enforcement for all chat writes.
- Safe `500` responses that do not expose database errors or message bodies.

### Broker and SSE

Cover:

- Authenticated and unauthenticated connections.
- Exact `message.created` event format.
- Buyer and seller delivery with unrelated-user isolation.
- Multiple subscriptions for one user.
- Non-blocking publication and full-channel disconnection.
- Idempotent unsubscribe and no send-on-closed-channel race.
- Keepalive output using an injected timer or test interval.
- Client cancellation cleanup.
- Broker close and graceful server shutdown.
- Race-detector coverage for subscribe, publish, unsubscribe, and close interactions.

## Verification and Acceptance

Run:

```text
make sqlc
make docs-check
make fmt
make vet
make lint
make test-race
make mod-verify
make web-check
TEST_DATABASE_URL='postgres://keebhub:keebhub@localhost:54329/keebhub?sslmode=disable' make test-integration
make docker-build
```

Integration acceptance is not complete if PostgreSQL tests were skipped.

### Persistent chat gate

1. Authenticate a seller and buyer and create an active listing owned by the seller.
2. Send concurrent start-conversation requests and confirm they return one conversation.
3. Send messages in both directions using only REST.
4. Confirm inbox data, unread counts, history, catch-up, and read pointers are correct after refresh.
5. Change the listing to sold or archived and confirm the existing conversation remains usable.
6. Confirm a third user cannot discover or access the conversation.

### SSE gate

1. Open SSE connections for the buyer and seller.
2. Send a message and confirm both streams receive the same minimal `message.created` event after the committed response exists in PostgreSQL.
3. Fill or disconnect one subscription and confirm message sending still succeeds.
4. Recover the missed message through `after_id`.
5. Restart the application, allow streams to reconnect, and confirm committed history remains complete.
6. Stop the server with active SSE connections and confirm graceful shutdown finishes within its timeout.

## Documentation Completion

When the implementation passes both gates:

- Mark all six OpenAPI operations as implemented.
- Update the matching behavior in `docs/07-api-contract.md` and `docs/09-chat-and-sse.md`.
- Update the implementation baseline in `README.md` and `AGENTS.md`.
- Check off the completed backend chat and SSE items in `docs/16-implementation-plan.md` while leaving frontend items unchecked.
- Keep the frontend inbox, conversation page, `EventSource`, and reconnect reconciliation work for a separate plan.
