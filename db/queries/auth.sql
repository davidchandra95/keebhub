-- name: GetUserByDiscordID :one
SELECT id, discord_id, discord_username, display_name, avatar_url, handle, location, bio, status, created_at, updated_at
FROM users
WHERE discord_id = $1;

-- name: CreateUser :one
INSERT INTO users (discord_id, discord_username, display_name, avatar_url, handle)
VALUES ($1, $2, $3, $4, $5)
RETURNING id, discord_id, discord_username, display_name, avatar_url, handle, location, bio, status, created_at, updated_at;

-- name: UpdateDiscordIdentity :one
UPDATE users
SET discord_username = $2,
    display_name = $3,
    avatar_url = $4,
    updated_at = $5
WHERE id = $1
RETURNING id, discord_id, discord_username, display_name, avatar_url, handle, location, bio, status, created_at, updated_at;

-- name: CreateSession :exec
INSERT INTO sessions (user_id, token_hash, expires_at, created_at, last_seen_at)
VALUES ($1, $2, $3, $4, $4);

-- name: DeleteSessionByHash :execrows
DELETE FROM sessions
WHERE token_hash = $1;

-- name: TouchAndGetSessionUser :one
WITH valid_session AS (
    UPDATE sessions
    SET last_seen_at = $2
    WHERE token_hash = $1
      AND expires_at > $2
    RETURNING user_id
)
SELECT users.id,
       users.discord_id,
       users.discord_username,
       users.display_name,
       users.avatar_url,
       users.handle,
       users.location,
       users.bio,
       users.status,
       users.created_at,
       users.updated_at
FROM users
JOIN valid_session ON valid_session.user_id = users.id;

-- name: UpdateUserProfile :one
UPDATE users
SET location = CASE WHEN sqlc.arg(set_location)::boolean THEN sqlc.narg(location)::text ELSE location END,
    bio = CASE WHEN sqlc.arg(set_bio)::boolean THEN sqlc.narg(bio)::text ELSE bio END,
    updated_at = sqlc.arg(updated_at)::timestamptz
WHERE id = sqlc.arg(user_id)
  AND status = 'active'
RETURNING id, discord_id, discord_username, display_name, avatar_url, handle, location, bio, status, created_at, updated_at;

-- name: GetSellerProfileByHandle :one
SELECT users.id,
       users.handle,
       users.display_name,
       users.avatar_url,
       users.location,
       users.bio,
       users.created_at,
       count(listings.id)::bigint AS active_listing_count
FROM users
LEFT JOIN listings
  ON listings.seller_id = users.id
 AND listings.status = 'active'
 AND listings.moderation_status = 'visible'
WHERE users.handle = $1
GROUP BY users.id;
