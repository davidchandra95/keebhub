# Security and Trust

## 1. Threat model

The system is a public classifieds site with:

- user-generated listing text;
- user-generated private chat;
- Discord authentication;
- public seller profiles;
- direct buyer-seller contact.

Likely threats:

- spam;
- phishing links;
- scam listings;
- harassment;
- impersonation;
- session theft;
- CSRF;
- XSS;
- OAuth state attacks;
- abusive chat traffic;
- enumeration;
- oversized input;
- credential leakage from logs.

## 2. Authentication security

- authorization code exchange occurs server-side;
- validate OAuth `state`;
- keep Discord client secret server-side;
- use local application sessions;
- session cookie is HttpOnly;
- session cookie is Secure in production;
- use SameSite=Lax by default;
- logout invalidates server-side session;
- disabled local accounts cannot create content or send chat.

The v1 service enforces the existing disabled state but provides no reporting or operator workflow that sets it.

## 3. Session storage

Use opaque random tokens.

Store only token hashes.

If database leaks, stored session hashes should not immediately act as bearer credentials.

## 4. XSS

Listings and chat are plain text.

Frontend must render user content as text, not raw HTML.

Do not support arbitrary HTML.

If links are detected:

- create links from parsed text;
- apply safe URL scheme rules;
- use appropriate `rel` behavior for external links.

## 5. Input limits

Suggested initial limits:

| Field | Limit |
|---|---:|
| Listing title | 120 Unicode characters |
| Listing description | 5,000 Unicode characters |
| Location | 100 chars |
| Bio | 500 chars |
| Chat message | 2,000 chars |

Reject unreasonable request body sizes at HTTP layer.

## 6. Authorization

Every owner/participant endpoint performs server-side authorization.

Never trust frontend visibility.

Examples:

- only listing seller can edit status;
- only conversation participants can read messages;
- seller cannot create buyer conversation against own listing;
- arbitrary user cannot mark another participant's read state.

## 7. CSRF

State-changing endpoints use non-GET methods.

With same-origin cookie authentication:

- keep SameSite protection;
- require unsafe requests to carry a matching `Origin`, or a matching `Referer` only when `Origin` is absent;
- do not implement sensitive GET mutations.

If deployment later splits frontend and API across sites, reassess CSRF controls.

## 8. SQL injection

Use sqlc and parameterized SQL.

Do not concatenate user-provided search strings into SQL.

Dynamic sort must be selected from an allowlist, not inserted directly from arbitrary query parameter values.

## 9. Public IDs and enumeration

Sequential IDs are acceptable for v1 if authorization is correct.

Do not mistake unguessable IDs for access control.

If vanity/privacy concerns later justify opaque external identifiers, introduce them deliberately.

## 10. Marketplace trust

KeebHub does not verify off-platform transactions.

Product copy should state:

- payment and delivery are arranged directly;
- platform does not hold funds;
- platform cannot guarantee item condition;
- users should use reasonable precautions for direct transfer/COD.

Do not imply buyer protection that does not exist.

## 11. Ratings

Do not implement transaction ratings in v1.

The platform cannot reliably prove:

- payment;
- shipment;
- delivery;
- completed transaction.

A rating system without a transaction truth source is easy to manipulate.

## 12. Deferred post-v1 trust and moderation

v1 does not include report submission, user blocking, moderation review, or an operator administration tool. These are product workflows, not prerequisites for shipping the catalog and buyer-seller chat loop.

If a later trust slice is approved, it must define report targets and reasons, rate limits, operator authorization, listing and account actions, private-message access policy, and immutable audit records together. The marketplace remains limited to mechanical-keyboard-related goods, but no in-product v1 workflow handles prohibited-content reports.

## 13. Secrets

Never commit:

- Discord client secret;
- production database credentials;
- session-signing/encryption secrets;
- infrastructure credentials.

Use environment variables or deployment secret mechanisms.

## 14. Logs

Never log:

- raw session cookie;
- OAuth code;
- Discord access token;
- Discord client secret;
- database password.

Avoid logging message bodies unless temporarily required for a specific investigation with proper access controls.

## 15. Backups

Database backups contain:

- identities;
- private chats;

Treat backups as sensitive.

Restrict access and define retention.
