-- name: StartConversation :one
INSERT INTO conversations (listing_id, seller_id, buyer_id, created_at)
VALUES (sqlc.arg(listing_id), sqlc.arg(seller_id), sqlc.arg(buyer_id), sqlc.arg(created_at))
ON CONFLICT (listing_id, seller_id, buyer_id) DO UPDATE
SET created_at = conversations.created_at
RETURNING id, listing_id, seller_id, buyer_id, seller_last_read_message_id, buyer_last_read_message_id,
          created_at, last_message_at, (xmax = 0) AS created;

-- name: ListConversationsForUser :many
SELECT conversations.id,
       conversations.listing_id,
       conversations.seller_id,
       conversations.buyer_id,
       conversations.created_at AS conversation_created_at,
       conversations.last_message_at AS conversation_last_message_at,
       listings.title AS listing_title,
       listings.status AS listing_status,
       counterpart.id AS counterpart_id,
       counterpart.handle AS counterpart_handle,
       counterpart.display_name AS counterpart_display_name,
       counterpart.avatar_url AS counterpart_avatar_url,
       COALESCE(latest.id, 0) AS last_message_id,
       COALESCE(latest.sender_id, 0) AS last_message_sender_id,
       COALESCE(latest.body, '') AS last_message_body,
       COALESCE(latest.created_at, 'epoch'::timestamptz) AS last_message_created_at,
       (
           SELECT count(*)
           FROM messages unread
           WHERE unread.conversation_id = conversations.id
             AND unread.sender_id <> sqlc.arg(user_id)
             AND unread.id > CASE
                 WHEN conversations.seller_id = sqlc.arg(user_id) THEN COALESCE(conversations.seller_last_read_message_id, 0)
                 ELSE COALESCE(conversations.buyer_last_read_message_id, 0)
             END
       )::bigint AS unread_count,
       COALESCE(conversations.last_message_at, conversations.created_at) AS activity_at
FROM conversations
JOIN listings ON listings.id = conversations.listing_id
JOIN users counterpart ON counterpart.id = CASE
    WHEN conversations.seller_id = sqlc.arg(user_id) THEN conversations.buyer_id
    ELSE conversations.seller_id
END
LEFT JOIN LATERAL (
    SELECT id, sender_id, body, created_at
    FROM messages
    WHERE conversation_id = conversations.id
    ORDER BY id DESC
    LIMIT 1
) latest ON TRUE
WHERE (conversations.seller_id = sqlc.arg(user_id) OR conversations.buyer_id = sqlc.arg(user_id))
  AND (
      sqlc.narg(cursor_activity_at)::timestamptz IS NULL
      OR (COALESCE(conversations.last_message_at, conversations.created_at), conversations.id)
          < (sqlc.narg(cursor_activity_at)::timestamptz, sqlc.narg(cursor_id)::bigint)
  )
ORDER BY COALESCE(conversations.last_message_at, conversations.created_at) DESC, conversations.id DESC
LIMIT sqlc.arg(page_limit)::integer;

-- name: GetConversationForParticipant :one
SELECT id, listing_id, seller_id, buyer_id, seller_last_read_message_id, buyer_last_read_message_id, created_at, last_message_at
FROM conversations
WHERE id = sqlc.arg(conversation_id)
  AND (seller_id = sqlc.arg(user_id) OR buyer_id = sqlc.arg(user_id));

-- name: ListMessagesBefore :many
SELECT id, conversation_id, sender_id, body, created_at
FROM messages
WHERE conversation_id = sqlc.arg(conversation_id)
  AND (sqlc.narg(before_id)::bigint IS NULL OR id < sqlc.narg(before_id)::bigint)
ORDER BY id DESC
LIMIT sqlc.arg(page_limit)::integer;

-- name: ListMessagesAfter :many
SELECT id, conversation_id, sender_id, body, created_at
FROM messages
WHERE conversation_id = sqlc.arg(conversation_id)
  AND id > sqlc.arg(after_id)
ORDER BY id ASC
LIMIT sqlc.arg(page_limit)::integer;

-- name: InsertMessage :one
INSERT INTO messages (conversation_id, sender_id, body, created_at)
VALUES (sqlc.arg(conversation_id), sqlc.arg(sender_id), sqlc.arg(body), sqlc.arg(created_at))
RETURNING id, conversation_id, sender_id, body, created_at;

-- name: UpdateConversationLastMessageAt :exec
UPDATE conversations
SET last_message_at = sqlc.arg(last_message_at)
WHERE id = sqlc.arg(conversation_id);

-- name: GetMessageInConversation :one
SELECT id
FROM messages
WHERE id = sqlc.arg(message_id)
  AND conversation_id = sqlc.arg(conversation_id);

-- name: AdvanceSellerReadPointer :exec
UPDATE conversations
SET seller_last_read_message_id = CASE
    WHEN seller_last_read_message_id IS NULL OR seller_last_read_message_id < sqlc.arg(message_id)
        THEN sqlc.arg(message_id)
    ELSE seller_last_read_message_id
END
WHERE id = sqlc.arg(conversation_id)
  AND seller_id = sqlc.arg(user_id);

-- name: AdvanceBuyerReadPointer :exec
UPDATE conversations
SET buyer_last_read_message_id = CASE
    WHEN buyer_last_read_message_id IS NULL OR buyer_last_read_message_id < sqlc.arg(message_id)
        THEN sqlc.arg(message_id)
    ELSE buyer_last_read_message_id
END
WHERE id = sqlc.arg(conversation_id)
  AND buyer_id = sqlc.arg(user_id);
