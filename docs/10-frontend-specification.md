# Frontend Specification

## 1. Stack

- React 19
- TypeScript
- Vite 8
- Astryx 0.3.0 beta with the neutral theme
- native browser `EventSource`
- `fetch` or a minimal HTTP wrapper

Astryx is currently a React/StyleX design system and should be treated as an external UI dependency rather than the application architecture itself.

Because it is still beta, pin the exact package version in the lockfile and avoid tightly coupling domain logic to Astryx components.

The foundation provides `AppButton` and `AppCard` wrappers as the first controlled adaptation points. Add more wrappers only when a product slice needs them.

## 2. UX priorities

1. Product title, price, condition, and availability must be immediately scannable.
2. Seller catalog should feel like a maintained inventory list, not a social feed.
3. Text-first layout must still look intentional without images.
4. Mobile responsive web is required.
5. Browsing must work without authentication.
6. Login interruption should happen only when the user performs an authenticated action.

## 3. Routes

Recommended routes:

```text
/                         marketplace
/listings/:listingId      listing detail
/u/:handle                seller catalog
/sell                     create listing
/my/listings              seller listing manager
/my/profile               profile settings
/inbox                    conversations
/inbox/:conversationId    conversation
/login                     authentication entry/error page
```

## 4. Marketplace page

### Content

- top navigation;
- search field;
- category filter;
- condition filter;
- min/max price;
- sort;
- listing results.

### Listing row/card

Because there are no photos, prioritize text hierarchy.

Example:

```text
Neo 98
Keyboard · Used · Negotiable

Rp3.000.000

Gunawan · Jakarta Barat
Active · updated 2h ago
```

Do not reserve a blank image area.

## 5. Listing detail

Display:

- title;
- price;
- status;
- condition;
- category;
- quantity;
- negotiable;
- description;
- seller summary;
- seller location;
- "View Catalog";
- "Contact Seller".

If current user owns the listing, replace buyer CTA with management actions.

## 6. Seller catalog

Header:

- avatar;
- display name;
- handle;
- location;
- bio;
- active listing count;
- "Copy Discord Post" if owner.

Sections:

```text
Active
Reserved
Sold history, optional collapsed section
```

Allow category filtering inside seller catalog when inventory is large enough.

## 7. Seller listing manager

Tabs:

```text
Active
Reserved
Sold
Archived
```

Each item exposes concise actions.

Example:

```text
Neo 98
Rp3.000.000

[Edit] [Reserve] [Sold] [Archive]
```

## 8. Create/edit listing form

Fields:

1. title;
2. description;
3. price IDR;
4. quantity;
5. category;
6. condition;
7. negotiable.

Price input behavior:

- accept user-friendly formatting;
- submit integer rupiah;
- never send formatted strings to API.

## 9. Inbox

List:

- counterpart avatar/name;
- listing title;
- latest message preview;
- time;
- unread badge;
- listing state.

On desktop:

```text
inbox list | conversation
```

is optional.

For implementation simplicity, separate routes are acceptable.

## 10. Conversation page

Header includes listing context:

```text
Neo 98
Rp3.000.000 · Active
Seller: Gunawan
```

Messages below.

Input:

```text
[ Type a message...                     ] [Send]
```

No attachment button.

No typing indicator.

## 11. Authentication UX

Anonymous user presses:

```text
Contact Seller
```

Frontend redirects to:

```text
/auth/discord?return_to=/listings/1001
```

After successful login, user returns to the intended context.

`/login` is the authentication entry and error page. It preserves only a safe internal `return_to`, maps known callback error codes to user-facing text, and shows the request ID when support information is available.

## 12. SSE client behavior

At authenticated app shell:

1. create one `EventSource('/api/v1/events')`;
2. listen for `message.created`;
3. update inbox state;
4. if matching conversation is open, fetch/merge missing messages;
5. reconnect automatically;
6. on `visibilitychange` from hidden to visible, reconcile active conversation/inbox.

Keep SSE connection ownership high in the authenticated application tree rather than inside each chat component.

## 13. State management

Do not add a global state framework by default.

Start with:

- route loaders/hooks;
- React state;
- query/cache abstraction only if needed.

Persistent server state should not be duplicated into a complex frontend store without benefit.

## 14. API typing

Maintain explicit TypeScript response/request types.

Do not import Go-generated types into frontend.

The HTTP contract is the boundary.

## 15. Astryx isolation

Use small local wrappers for frequently used primitives when useful:

```text
AppButton
AppInput
AppDialog
AppBadge
AppCard
```

This gives a controlled adaptation layer if Astryx beta APIs change.

Do not wrap every component merely for abstraction.

## 16. Accessibility

At minimum:

- keyboard navigable controls;
- proper labels;
- focus indicators;
- semantic buttons and links;
- status not communicated using color alone;
- sufficient contrast;
- accessible modal/dialog behavior.

## 17. Empty states

### No marketplace result

```text
No matching listings.
Try changing the search or filters.
```

### New seller catalog

```text
No active listings yet.
```

Owner receives CTA:

```text
Create your first listing
```

### Empty inbox

```text
No conversations yet.
Contact a seller from a listing to start one.
```

## 18. Error states

Use specific user-facing errors for:

- listing sold before contact;
- listing unavailable;
- session expired;
- message send failure;
- temporary reconnecting state;
- OAuth failure.

SSE disconnect should not show a large blocking error. Chat remains readable and HTTP actions remain available.
