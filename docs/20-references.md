# Technical References

These are primary references for external technologies used by the project.

## Astryx

Meta open-source repository:

- https://github.com/facebook/astryx

Relevant current project facts at the time this documentation was written:

- Astryx is open source;
- built on React and StyleX;
- currently marked Beta;
- provides a large prebuilt component set.

Because it is Beta, the implementation plan recommends pinning exact dependency versions.

## Discord OAuth2

Discord developer documentation:

- https://docs.discord.com/developers/platform/oauth2-and-permissions
- https://docs.discord.com/developers/topics/oauth2
- https://docs.discord.com/developers/resources/user

## Backend foundation

- https://go.dev/dl/
- https://github.com/labstack/echo
- https://docs.sqlc.dev/en/stable/
- https://github.com/jackc/pgx
- https://github.com/pressly/goose
- https://www.postgresql.org/docs/18/

The initial foundation uses Go 1.26.5, Echo v5, sqlc 1.31.1, pgx v5, Goose v3, and PostgreSQL 18.4. Exact dependency versions are pinned in build files and lockfiles.

Relevant design facts:

- Discord uses OAuth2;
- `identify` grants access to basic user profile information;
- email is a separate permission scope;
- OAuth authorization code flows should validate `state`;
- user identity can be fetched through Discord's user API after authorization.

## Project-specific interpretation

KeebHub deliberately uses Discord only for identity in v1.

It does not require a Discord bot or guild/channel access for the catalog-export feature because sellers manually paste generated text into Discord.
