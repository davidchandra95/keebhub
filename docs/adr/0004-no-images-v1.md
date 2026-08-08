# ADR-0004: No Listing Images in v1

- Status: Accepted
- Date: 2026-08-08

## Context

Most C2C marketplaces depend heavily on photos.

However, the initial target community already uses text-heavy Discord WTS lists and frequently asks for detailed condition/photos through DM.

Image support would add:

- upload API;
- object storage;
- file validation;
- resizing;
- moderation;
- CDN/storage costs;
- deletion lifecycle.

## Decision

v1 listing data is text-only.

Chat also has no attachment upload.

## Consequences

### Positive

- significantly smaller implementation;
- no media infrastructure;
- faster validation of catalog/discovery hypothesis.

### Negative

- lower trust and browsing quality for some users;
- some sellers may refuse to use the platform without photos;
- buyers may still need external links or direct follow-up.

## Future design

When introduced, images use a one-to-many `listing_images` table.

Do not add a single image URL field to the `listings` table.

## Revisit when

Real-user adoption data shows that lack of photos materially blocks listing creation or buyer engagement.
