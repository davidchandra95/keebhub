# User Flows

## 1. Anonymous buyer discovery

```text
Shared catalog or listing link
    |
    v
Seller catalog or listing URL
    |
    v
Browse without login
    |
    +--> Search
    +--> Filter
    +--> Open seller
    +--> Open listing
    |
    v
Contact Seller
    |
    v
Discord login if unauthenticated
    |
    v
Conversation
```

### Acceptance rules

- Redirect back to the intended listing after login.
- Do not require login before the buyer indicates intent to contact.

## 2. First login

```text
User selects "Continue with Discord"
    |
    v
Backend creates OAuth state
    |
    v
Redirect to Discord OAuth2
    |
    v
User authorizes
    |
    v
Callback with code + state
    |
    v
Backend validates state
    |
    v
Backend exchanges code for Discord access token
    |
    v
Backend fetches Discord user identity
    |
    v
Upsert local user
    |
    v
Create local session
    |
    v
Set secure session cookie
    |
    v
Redirect to original route or marketplace
```

## 3. Seller creates first listing

```text
Login
  |
  v
Sell
  |
  v
Create Listing
  |
  +--> Title
  +--> Description
  +--> Price
  +--> Quantity
  +--> Category
  +--> Condition
  +--> Negotiable
  |
  v
Submit
  |
  v
Listing becomes ACTIVE
  |
  v
Seller catalog updates
```

## 4. Seller inventory management

```text
My Listings
    |
    +--> Active
    +--> Reserved
    +--> Sold
    +--> Archived
```

Each listing supports:

```text
Edit
Reserve
Mark Sold
Archive
Reactivate
```

State changes must be explicit. Do not infer "sold" from chat messages.

## 5. Buyer contacts seller

```text
Listing Detail
    |
    v
Contact Seller
    |
    +--> not logged in -> Discord login -> return
    |
    v
POST create/open conversation
    |
    v
Conversation page
```

Backend behavior:

1. Validate listing exists.
2. Validate listing is active or reserved.
3. Validate buyer is not seller.
4. Get or create unique conversation.
5. Return conversation ID.

## 6. Chat send flow

```text
Buyer UI
   |
   | POST /messages
   v
Go API
   |
   | INSERT message
   v
PostgreSQL
   |
   | commit
   v
In-memory SSE broker
   |
   +----------> buyer SSE
   |
   +----------> seller SSE
```

Important rule:

> Database commit happens before realtime publication.

The message exists even if SSE delivery fails.

## 7. Chat reconnect flow

```text
Browser loses connection
    |
    v
EventSource reconnects
    |
    v
Conversation UI fetches messages after last known ID
    |
    v
Local view is reconciled
```

SSE is an update signal, not the authoritative message store.

## 8. Reserve and sale flow

```text
ACTIVE
   |
   | buyer commits informally
   v
RESERVED
   |
   +--> deal fails -> ACTIVE
   |
   +--> deal succeeds -> SOLD
```

No payment state is modeled.

## 9. Post-v1 report flow (deferred)

This flow is intentionally outside the v1 release and must not block shipping the catalog and buyer-seller chat loop.

```text
Listing or seller
    |
    v
Report
    |
    v
Login if required
    |
    v
Select reason
    |
    v
Optional explanation
    |
    v
Submit
    |
    v
Stored for operator review
```
