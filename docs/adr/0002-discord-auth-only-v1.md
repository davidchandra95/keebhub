# ADR-0002: Discord-only Authentication in v1

- Status: Accepted
- Date: 2026-08-08

## Context

Initial users already participate in Discord mechanical-keyboard communities.

Adding:

- password login;
- password reset;
- email verification;
- Google login;

would increase authentication scope without solving the main product problem.

## Decision

Use Discord OAuth2 as the only login mechanism for v1.

Anonymous browsing remains available.

Request only minimal identity permission.

After OAuth succeeds, create a local KeebHub session.

## Consequences

### Positive

- no password storage;
- no reset flow;
- familiar identity for target community;
- some continuity with existing community persona.

### Negative

- users without Discord cannot sell or chat;
- authentication depends on Discord availability;
- Discord identity alone is not proof that a seller is trustworthy.

## Revisit when

A meaningful number of target users cannot or do not want to use Discord.
