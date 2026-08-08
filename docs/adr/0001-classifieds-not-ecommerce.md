# ADR-0001: Build a Classifieds Marketplace, Not Full E-commerce

- Status: Accepted
- Date: 2026-08-08

## Context

The user problem is repeated unstructured WTS posting in enthusiast Discord channels.

The platform needs to improve:

- inventory maintenance;
- discovery;
- seller catalog sharing;
- buyer-seller connection.

It does not need to own the financial transaction.

## Decision

KeebHub is a C2C classifieds platform.

Transaction flow is:

```text
List
-> Discover
-> Contact
-> Deal externally
```

The platform will not implement v1 cart, checkout, payment, shipping, escrow, refunds, or order management.

## Consequences

### Positive

- much smaller domain;
- lower security and compliance scope;
- lower infrastructure cost;
- faster path to validating product need;
- preserves community behavior.

### Negative

- platform cannot verify completed sales;
- platform cannot provide strong buyer protection;
- platform cannot reliably derive ratings from transaction truth;
- transaction analytics are limited.

## Revisit when

A large share of users explicitly demand platform-managed transaction protection and there is evidence they would adopt it.
