# Product Vision and Scope

## 1. Product definition

KeebHub is a specialized C2C classifieds platform for mechanical-keyboard enthusiasts, initially targeting Indonesia.

The platform converts unstructured marketplace posts into structured, searchable listings while preserving Discord as an existing community and distribution channel.

It is not intended to replace Tokopedia, Shopee, Carousell, or Discord as a general marketplace or community platform.

## 2. Problem statement

Mechanical-keyboard sellers commonly maintain inventory as repeated Discord messages.

Typical behavior:

1. Seller posts a long WTS message.
2. Some items sell.
3. Seller edits or recreates the message.
4. The next day the seller reposts the updated list.
5. Buyers search old messages and cannot easily determine whether an item remains available.
6. Product metadata such as price, condition, availability, and category is embedded inside free-form text.

This creates several problems:

- inventory is not a durable source of truth;
- search is weak;
- availability becomes stale;
- seller catalogs are hard to browse;
- buyers must inspect many unrelated messages;
- sellers repeatedly perform manual reposting work.

## 3. Product proposition

### Seller value

> Maintain your mechanical-keyboard inventory once and share one catalog everywhere.

### Buyer value

> Search listings that are actually available instead of digging through Discord history.

## 4. Target users

### Primary seller

An enthusiast who periodically sells:

- keyboards;
- keycaps;
- switches;
- keyboard parts;
- keyboard accessories.

They may be a hobbyist rotating their collection, not necessarily a professional merchant.

### Primary buyer

An enthusiast searching for specific used or niche keyboard products that may be difficult to find on mainstream marketplaces.

## 5. Initial geography

Indonesia.

v1 assumptions:

- prices use IDR;
- seller location is free-form text;
- COD and shipping arrangements occur off-platform;
- no map or geocoding functionality.

## 6. v1 goals

1. Allow sellers to maintain a public structured catalog.
2. Allow buyers to browse and search active listings without authentication.
3. Allow users to authenticate using Discord.
4. Allow sellers to create, edit, reserve, sell, and archive listings.
5. Allow buyers to initiate listing-scoped conversations with sellers.
6. Deliver new chat messages using SSE.

## 7. Explicit non-goals

The following are intentionally excluded from v1:

- cart;
- checkout;
- payment gateway;
- escrow;
- wallet;
- shipping integration;
- shipment tracking;
- order management;
- refunds;
- ratings;
- transaction reviews;
- dispute resolution;
- report submission and review;
- user blocking;
- listing or user moderation workflows;
- operator administration tools;
- listing photos;
- chat attachments;
- voice or video;
- online presence;
- typing indicators;
- read receipts;
- WebSocket;
- Redis;
- Kafka;
- NATS;
- mobile application;
- WTB listings;
- WTT listings;
- multi-currency;
- recommendation engine;
- sponsored listings;
- auctions.

## 8. Marketplace boundaries

The platform is a connector.

It stores:

- identity;
- listings;
- seller catalog data;
- conversations;
- messages.

It does not know or guarantee:

- whether payment occurred;
- whether an item was shipped;
- whether an item was received;
- whether a COD meeting occurred;
- whether the physical product matches its description.

This boundary must be visible in product copy and terms.

## 9. Initial categories

1. `keyboard`
2. `keycaps`
3. `switches`
4. `parts`
5. `accessories`
6. `other`

`other` means another mechanical-keyboard-related item. The marketplace must not intentionally evolve into a generic second-hand marketplace during v1.

## 10. Listing condition

- `new`
- `used`

Avoid expanding condition grading until users demonstrate a need.

## 11. Listing states

- `active`
- `reserved`
- `sold`
- `archived`

### State meaning

`active`
: Visible in normal marketplace search and available for buyer inquiries.

`reserved`
: Temporarily committed to a potential buyer. Still visible, but clearly marked reserved.

`sold`
: No longer available. Remains accessible by direct URL and seller history.

`archived`
: Removed from normal public discovery without claiming a sale occurred.

## 12. Success criteria for the first release

The initial release is successful if real users can complete this loop without operator assistance:

1. Login through Discord.
2. Create several listings.
3. Open a public seller catalog.
4. Share a seller catalog or listing link.
5. Another user follows the link.
6. Buyer searches or opens a listing.
7. Buyer contacts seller.
8. Both users exchange messages.
9. Seller marks the item reserved or sold.

Do not use GMV or platform revenue as a v1 success criterion because transactions occur off-platform.

## 13. Product metrics

Useful metrics:

- active sellers;
- active listings;
- listing creation rate;
- listing status transition rate;
- public catalog views;
- listing views;
- searches;
- contact-seller conversions;
- conversations created;
- messages sent;
- weekly returning sellers.

Avoid vanity metrics that do not connect to the core loop.
