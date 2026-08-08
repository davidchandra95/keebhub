# Discord Catalog Export

## 1. Purpose

This feature connects structured inventory back to the community behavior that already exists.

The product should not ask sellers to abandon Discord.

Instead:

> Maintain catalog in KeebHub, then use Discord as distribution.

## 2. User flow

```text
Seller edits listings
    |
    v
Seller catalog is current
    |
    v
Click "Copy Discord Post"
    |
    v
Generated WTS text
    |
    v
Clipboard
    |
    v
Paste into Discord marketplace channel
```

## 3. Default format

Example:

```text
WTS Mechanical Keyboard Stuff

Keyboard
- Matrix Lab Hiya HHKB - Rp12.000.000
- Matrix Lab Magic3 HHKB - Rp12.000.000
- Leopold FC660M - Rp500.000
- Neo 98 - Rp3.000.000 [Nego]

Keycaps
- SA Maestro - Rp2.000.000
- SA Oblivion - Rp2.000.000

Switches
- Cherry MX Blue 100 pcs - Rp350.000
- Gateron RAW 120 pcs - Rp400.000

Reserved
- Realforce R3 Fullsize - Rp5.000.000

Full catalog:
https://market.example/u/gunawan
```

## 4. Included states

Default:

- active;
- reserved.

Excluded:

- sold;
- archived.

Reserved listings should be separated or clearly tagged so buyers do not assume normal availability.

## 5. Categories

Group by category only when it improves readability.

If a seller has only a few listings, a flat list is acceptable.

Formatter should be deterministic so the same inventory produces predictable output.

## 6. Price formatting

Store:

```text
3000000
```

Render:

```text
Rp3.000.000
```

Formatting belongs to presentation/export, not the database representation.

## 7. Negotiable flag

Possible rendering:

```text
[Nego]
```

Keep it compact.

Do not generate verbose prose per listing.

## 8. Seller terms

v1 does not need structured shipping/payment terms.

If seller has relevant text, allow it in profile `bio` or a later `seller_terms` field.

Do not infer JNE, COD, direct transfer, or insurance policy from listing content.

## 9. Export API

Recommended:

```text
GET /api/v1/me/catalog-export?format=discord
```

Return generated text and public catalog URL.

The frontend owns clipboard interaction.

## 10. No automatic Discord posting

v1 does not:

- require a bot;
- select channels;
- request guild/channel permissions;
- post on user's behalf;
- schedule daily reposting.

Reasons:

- lower OAuth permission surface;
- simpler implementation;
- less spam risk;
- maintains user control.

## 11. Future possibility

If manual copy becomes a demonstrated pain point, consider optional Discord bot or webhook integration later.

That requires a separate trust/spam and permission design.
