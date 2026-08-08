# Chat and SSE

## 1. Scope

Chat exists only to connect a buyer and a seller regarding one listing.

It is not a general messaging product.

v1 deliberately excludes:

- generic DMs;
- group chat;
- attachments;
- images;
- voice;
- reactions;
- replies;
- message editing;
- message deletion;
- typing indicators;
- presence;
- counterpart-facing read receipts.

## 2. Conversation model

A conversation is identified by:

```text
listing_id
seller_id
buyer_id
```

Unique constraint prevents duplicates.

The seller is always the listing owner.

## 3. Creating a conversation

Endpoint:

```text
POST /api/v1/listings/{listing_id}/conversation
```

Algorithm:

```text
1. Require authenticated user.
2. Load listing.
3. Reject if listing is sold or archived.
4. Reject if current user is seller.
5. INSERT conversation with ON CONFLICT against unique tuple.
6. Return resulting conversation.
```

This makes the operation safe under concurrent button presses.

## 4. Sending a message

Endpoint:

```text
POST /api/v1/conversations/{conversation_id}/messages
```

Transaction:

```text
BEGIN

verify sender belongs to conversation
INSERT message
UPDATE conversations.last_message_at

COMMIT
```

After commit:

```text
publish message.created to in-memory SSE broker
```

Never publish before commit.

## 5. Event design

One authenticated SSE connection per browser/user:

```text
GET /api/v1/events
```

Do not open one connection per conversation.

### v1 event

```text
event: message.created
id: 5004
data: {"conversation_id":"9001","message_id":"5004"}
```

Keep event payload minimal.

The durable message is retrieved from PostgreSQL.

## 6. Why SSE fits

Client-to-server operations are ordinary HTTP requests:

```text
send message -> POST
mark read -> POST
```

Server-to-client requirement is mainly:

```text
new message available
```

This is a one-way server push problem, which SSE handles directly.

## 7. Browser connection

Use native `EventSource` where practical.

Because authentication uses same-origin HttpOnly cookies:

- JavaScript does not need access to credentials;
- the browser sends the cookie;
- no bearer token needs to be embedded into an SSE URL.

## 8. Keepalive

The server should periodically send an SSE comment to prevent idle intermediaries from closing the connection:

```text
: keepalive
```

Suggested interval:

```text
15 to 30 seconds
```

Exact value can be operationally tuned.

## 9. Reconnect correctness

SSE may disconnect.

Therefore:

> Event delivery is best-effort. Message persistence is authoritative.

Client strategy:

1. keep last known message ID per open conversation;
2. when stream reconnects or page resumes, fetch messages with `after_id`;
3. merge by message ID;
4. update UI;
5. update read pointer when appropriate.

## 10. Last-Event-ID

The server may set SSE `id` to the created message ID.

The browser can send `Last-Event-ID` during automatic reconnect.

Do not make correctness depend solely on it. A message ID is useful for hints, but the application should still reconcile persistent conversation state.

## 11. In-memory broker

Conceptual structure:

```text
Broker
  subscribersByUserID map[userID]set[subscription]
```

Subscribe:

```text
user opens /events
-> broker.add(userID, channel)
```

Unsubscribe:

```text
request context ends
-> broker.remove(userID, channel)
```

Publish:

```text
broker.publish(sellerID, event)
broker.publish(buyerID, event)
```

A full broker queue is unnecessary.

If a subscriber is slow, do not allow it to block message sends indefinitely.

Use:

- bounded per-subscriber channel;
- non-blocking publish or short timeout;
- disconnect/reconcile rather than unbounded memory growth.

## 12. Message pagination

### Older messages

```text
GET /api/v1/conversations/{conversation_id}/messages?before_id=5000&limit=50
```

### Catch-up

```text
GET /api/v1/conversations/{conversation_id}/messages?after_id=5000&limit=100
```

IDs are monotonic.

## 13. Read pointer

When conversation is visible and messages are rendered:

```text
POST /api/v1/conversations/{conversation_id}/read
{
  "last_read_message_id": "5004"
}
```

Server only advances the pointer.

## 14. Unread calculation

For buyer:

```text
message.sender_id != buyer_id
AND message.id > buyer_last_read_message_id
```

Equivalent logic for seller.

## 15. Multi-tab behavior

One user may have multiple tabs and therefore multiple SSE connections.

That is acceptable.

Broker stores multiple subscribers per user.

Frontend must deduplicate messages by ID.

## 16. Scaling beyond one app process

One-process in-memory broker is correct for v1.

When deploying multiple app replicas, an event created on replica A must reach a user connected to replica B.

At that point add a small cross-instance mechanism.

Preferred evaluation order:

1. PostgreSQL `LISTEN/NOTIFY` if requirements remain simple.
2. Redis Pub/Sub if Redis is otherwise justified.
3. NATS if broader messaging requirements develop.

Do not introduce Kafka for this use case.
