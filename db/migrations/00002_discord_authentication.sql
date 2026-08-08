-- +goose Up
CREATE TABLE users (
    id BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    discord_id TEXT NOT NULL UNIQUE,
    discord_username TEXT NOT NULL,
    display_name TEXT NOT NULL,
    avatar_url TEXT,
    handle TEXT NOT NULL,
    location TEXT,
    bio TEXT,
    status TEXT NOT NULL DEFAULT 'active',
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT users_discord_id_format CHECK (discord_id ~ '^[0-9]+$'),
    CONSTRAINT users_discord_username_length CHECK (char_length(discord_username) BETWEEN 1 AND 100),
    CONSTRAINT users_display_name_length CHECK (char_length(display_name) BETWEEN 1 AND 100),
    CONSTRAINT users_handle_format CHECK (handle ~ '^[a-z0-9](?:[a-z0-9-]{1,38}[a-z0-9])$'),
    CONSTRAINT users_location_length CHECK (location IS NULL OR char_length(location) <= 100),
    CONSTRAINT users_bio_length CHECK (bio IS NULL OR char_length(bio) <= 500),
    CONSTRAINT users_status_valid CHECK (status IN ('active', 'disabled'))
);

CREATE UNIQUE INDEX users_handle_lower_key ON users (lower(handle));

CREATE TABLE sessions (
    id BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    user_id BIGINT NOT NULL REFERENCES users(id),
    token_hash BYTEA NOT NULL UNIQUE,
    expires_at TIMESTAMPTZ NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    last_seen_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT sessions_token_hash_length CHECK (octet_length(token_hash) = 32),
    CONSTRAINT sessions_expiry_order CHECK (expires_at > created_at)
);

CREATE INDEX sessions_user_id_idx ON sessions (user_id);
CREATE INDEX sessions_expires_at_idx ON sessions (expires_at);

-- +goose Down
DROP TABLE sessions;
DROP TABLE users;
