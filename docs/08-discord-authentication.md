# Discord Authentication

## 1. Decision

Use Discord OAuth2 authorization code flow as the only authentication method in v1.

Anonymous browsing remains available.

## 2. Requested Discord scope

Request only:

```text
identify
```

Do not request email unless a future product requirement genuinely needs it.

The local application needs basic identity such as:

- Discord user ID;
- username;
- display/global name where available;
- avatar.

## 3. No bot requirement

Authentication does not require a Discord bot.

v1 does not:

- post messages into Discord;
- inspect guild channels;
- manage roles;
- read marketplace channels.

The user manually copies the generated WTS text and pastes it into Discord.

## 4. OAuth flow

### Step 1

Browser:

```text
GET /auth/discord?return_to=/listings/1001
```

### Step 2

Backend:

1. validate `return_to` as an internal route;
2. generate cryptographically random OAuth `state`;
3. store the state and validated internal return route in a 10-minute HttpOnly OAuth cookie;
4. redirect to Discord authorization URL.

### Step 3

Discord redirects:

```text
GET /auth/discord/callback?code=...&state=...
```

### Step 4

Backend:

1. compare returned `state`;
2. reject mismatch or missing state;
3. exchange authorization code server-to-server;
4. fetch current Discord user;
5. upsert local `users` row;
6. create local application session;
7. clear OAuth state cookie;
8. set application session cookie;
9. redirect to `return_to`.

## 5. Discord token handling

After identity is fetched, the application does not need to use the Discord access token for routine requests.

Recommended v1 behavior:

- do not expose Discord token to browser;
- do not use it as KeebHub session credential;
- do not persist it unless a later Discord API feature requires persistence.

This keeps the application authentication model independent from Discord API token lifetime.

## 6. Local session design

Generate a 32-byte cryptographically random session token and encode it using unpadded base64url.

Browser receives:

```text
keebhub_session=<raw-token>
```

Database stores the 32-byte SHA-256 digest:

```text
hash(raw-token)
```

Lookup:

1. hash cookie token;
2. query session by hash;
3. verify not expired;
4. load user;
5. reject disabled user for authenticated mutations.

## 7. Cookie settings

Production:

```text
HttpOnly
Secure
SameSite=Lax
Path=/
```

Recommended:

- opaque cookie value;
- fixed 30-day session lifetime;
- rotate session token on login;
- invalidate on logout.

Because frontend and backend are same-origin, no CORS or cross-site token storage is needed.

## 8. CSRF

SameSite cookies reduce CSRF exposure but are not the only control.

For state-changing endpoints:

- require non-GET methods;
- require `Origin` to match `APP_BASE_URL`; if a supported browser omits `Origin`, require a matching `Referer`;
- consider a CSRF token if deployment patterns later become cross-site;
- never implement mutations as GET requests.

OAuth callback specifically relies on validated OAuth `state`.

## 9. Account synchronization

On each successful Discord login, refresh local snapshots:

- discord username;
- display name;
- avatar URL.

Do not automatically change the local catalog handle when Discord username changes.

The public catalog URL must remain stable.

## 10. Handle creation

On first login:

```text
discord username: Gunawan.Keyboard
normalized: gunawan-keyboard
```

If collision:

```text
gunawan-keyboard-2
```

or another deterministic/random suffix.

Store the resulting handle as local application data.

## 11. Login return target

Support an internal `return_to`.

Use case:

```text
Buyer opens listing
-> Contact Seller
-> Discord login
-> callback
-> buyer returns to listing or directly continues conversation creation
```

Security:

- allow only relative/internal paths;
- reject absolute external URLs;
- prevent open redirects.

## 12. Environment variables

```text
DISCORD_CLIENT_ID
DISCORD_CLIENT_SECRET
DISCORD_REDIRECT_URI
APP_BASE_URL
SESSION_COOKIE_NAME
```

`SESSION_COOKIE_NAME` defaults to `keebhub_session` in development. Production validates that the configured application, redirect, and cookie settings agree with HTTPS deployment.

Never commit client secret.

### Local Developer Portal setup

Use the existing Discord Developer Portal application whose client ID matches `DISCORD_CLIENT_ID`.

Register exactly this OAuth redirect URL for local development:

```text
http://localhost:8080/auth/discord/callback
```

Do not create a bot, enable privileged intents, add bot permissions, or request scopes beyond `identify` for authentication.

For native development, `make server` loads an untracked `.env` file when `APP_ENV` is unset or `development`. The file supplies only missing variables, so explicitly injected environment values take precedence. Docker Compose reads the same local `.env` file before starting containers. A missing `.env` is allowed for public-only development; malformed local files stop startup without logging their values.

## 13. Failure cases

Handle:

- user denies authorization;
- invalid state;
- expired state;
- code exchange failure;
- Discord API timeout;
- malformed user response;
- disabled local account.

OAuth failure should return the user to a safe login page with a user-facing error and a request ID for support.

The implemented login page recognizes only these public error codes:

```text
authorization_denied
oauth_state_invalid
discord_unavailable
account_disabled
authentication_unavailable
authentication_failed
```

Unknown values use a generic message. Internal errors and Discord tokens are never placed in the redirect URL.
