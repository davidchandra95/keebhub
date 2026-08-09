-- name: ListActiveCategories :many
SELECT id, slug, name, sort_order, active
FROM categories
WHERE active = TRUE
ORDER BY sort_order, id;

-- name: GetActiveCategoryBySlug :one
SELECT id, slug, name, sort_order, active
FROM categories
WHERE slug = $1
  AND active = TRUE;

-- name: CreateListing :one
WITH inserted AS (
    INSERT INTO listings (
        seller_id,
        category_id,
        title,
        description,
        price_idr,
        quantity,
        condition,
        status,
        moderation_status,
        negotiable,
        created_at,
        updated_at
    ) VALUES (
        sqlc.arg(seller_id),
        sqlc.arg(category_id),
        sqlc.arg(title),
        sqlc.arg(description),
        sqlc.arg(price_idr),
        sqlc.arg(quantity),
        sqlc.arg(condition),
        'active',
        'visible',
        sqlc.arg(negotiable),
        sqlc.arg(created_at),
        sqlc.arg(updated_at)
    )
    RETURNING *
)
SELECT inserted.id,
       inserted.seller_id,
       inserted.category_id,
       inserted.title,
       inserted.description,
       inserted.price_idr,
       inserted.quantity,
       inserted.condition,
       inserted.status,
       inserted.moderation_status,
       inserted.negotiable,
       inserted.created_at,
       inserted.updated_at,
       categories.slug AS category_slug,
       categories.name AS category_name,
       users.handle AS seller_handle,
       users.display_name AS seller_display_name,
       users.avatar_url AS seller_avatar_url,
       users.location AS seller_location,
       users.bio AS seller_bio
FROM inserted
JOIN categories ON categories.id = inserted.category_id
JOIN users ON users.id = inserted.seller_id;

-- name: GetListingByID :one
SELECT listings.id,
       listings.seller_id,
       listings.category_id,
       listings.title,
       listings.description,
       listings.price_idr,
       listings.quantity,
       listings.condition,
       listings.status,
       listings.moderation_status,
       listings.negotiable,
       listings.created_at,
       listings.updated_at,
       categories.slug AS category_slug,
       categories.name AS category_name,
       users.handle AS seller_handle,
       users.display_name AS seller_display_name,
       users.avatar_url AS seller_avatar_url,
       users.location AS seller_location,
       users.bio AS seller_bio
FROM listings
JOIN categories ON categories.id = listings.category_id
JOIN users ON users.id = listings.seller_id
WHERE listings.id = $1;

-- name: UpdateOwnedListing :one
WITH updated AS (
    UPDATE listings
    SET category_id = CASE WHEN sqlc.arg(set_category_id)::boolean THEN sqlc.arg(category_id)::bigint ELSE category_id END,
        title = CASE WHEN sqlc.arg(set_title)::boolean THEN sqlc.arg(title)::text ELSE title END,
        description = CASE WHEN sqlc.arg(set_description)::boolean THEN sqlc.arg(description)::text ELSE description END,
        price_idr = CASE WHEN sqlc.arg(set_price_idr)::boolean THEN sqlc.arg(price_idr)::bigint ELSE price_idr END,
        quantity = CASE WHEN sqlc.arg(set_quantity)::boolean THEN sqlc.arg(quantity)::integer ELSE quantity END,
        condition = CASE WHEN sqlc.arg(set_condition)::boolean THEN sqlc.arg(condition)::text ELSE condition END,
        negotiable = CASE WHEN sqlc.arg(set_negotiable)::boolean THEN sqlc.arg(negotiable)::boolean ELSE negotiable END,
        updated_at = sqlc.arg(updated_at)::timestamptz
    WHERE listings.id = sqlc.arg(listing_id)
      AND listings.seller_id = sqlc.arg(seller_id)
      AND listings.moderation_status = 'visible'
    RETURNING *
)
SELECT updated.id,
       updated.seller_id,
       updated.category_id,
       updated.title,
       updated.description,
       updated.price_idr,
       updated.quantity,
       updated.condition,
       updated.status,
       updated.moderation_status,
       updated.negotiable,
       updated.created_at,
       updated.updated_at,
       categories.slug AS category_slug,
       categories.name AS category_name,
       users.handle AS seller_handle,
       users.display_name AS seller_display_name,
       users.avatar_url AS seller_avatar_url,
       users.location AS seller_location,
       users.bio AS seller_bio
FROM updated
JOIN categories ON categories.id = updated.category_id
JOIN users ON users.id = updated.seller_id;

-- name: UpdateOwnedListingStatus :one
WITH updated AS (
    UPDATE listings
    SET status = sqlc.arg(status)::text,
        updated_at = sqlc.arg(updated_at)::timestamptz
    WHERE listings.id = sqlc.arg(listing_id)
      AND listings.seller_id = sqlc.arg(seller_id)
      AND listings.moderation_status = 'visible'
    RETURNING *
)
SELECT updated.id,
       updated.seller_id,
       updated.category_id,
       updated.title,
       updated.description,
       updated.price_idr,
       updated.quantity,
       updated.condition,
       updated.status,
       updated.moderation_status,
       updated.negotiable,
       updated.created_at,
       updated.updated_at,
       categories.slug AS category_slug,
       categories.name AS category_name,
       users.handle AS seller_handle,
       users.display_name AS seller_display_name,
       users.avatar_url AS seller_avatar_url,
       users.location AS seller_location,
       users.bio AS seller_bio
FROM updated
JOIN categories ON categories.id = updated.category_id
JOIN users ON users.id = updated.seller_id;

-- name: ListOwnedListings :many
SELECT listings.id,
       listings.seller_id,
       listings.category_id,
       listings.title,
       listings.description,
       listings.price_idr,
       listings.quantity,
       listings.condition,
       listings.status,
       listings.moderation_status,
       listings.negotiable,
       listings.created_at,
       listings.updated_at,
       categories.slug AS category_slug,
       categories.name AS category_name,
       users.handle AS seller_handle,
       users.display_name AS seller_display_name,
       users.avatar_url AS seller_avatar_url,
       users.location AS seller_location,
       users.bio AS seller_bio
FROM listings
JOIN categories ON categories.id = listings.category_id
JOIN users ON users.id = listings.seller_id
WHERE listings.seller_id = sqlc.arg(seller_id)
  AND listings.moderation_status = 'visible'
  AND (sqlc.narg(status_filter)::text IS NULL OR listings.status = sqlc.narg(status_filter)::text)
  AND (
      sqlc.narg(cursor_updated_at)::timestamptz IS NULL
      OR (listings.updated_at, listings.id) < (sqlc.narg(cursor_updated_at)::timestamptz, sqlc.narg(cursor_id)::bigint)
  )
ORDER BY listings.updated_at DESC, listings.id DESC
LIMIT sqlc.arg(page_limit)::integer;

-- name: ListSellerListings :many
SELECT listings.id,
       listings.seller_id,
       listings.category_id,
       listings.title,
       listings.description,
       listings.price_idr,
       listings.quantity,
       listings.condition,
       listings.status,
       listings.moderation_status,
       listings.negotiable,
       listings.created_at,
       listings.updated_at,
       categories.slug AS category_slug,
       categories.name AS category_name,
       users.handle AS seller_handle,
       users.display_name AS seller_display_name,
       users.avatar_url AS seller_avatar_url,
       users.location AS seller_location,
       users.bio AS seller_bio
FROM listings
JOIN categories ON categories.id = listings.category_id
JOIN users ON users.id = listings.seller_id
WHERE listings.seller_id = sqlc.arg(seller_id)
  AND listings.moderation_status = 'visible'
  AND listings.status = ANY(sqlc.arg(statuses)::text[])
  AND (sqlc.narg(category_slug)::text IS NULL OR categories.slug = sqlc.narg(category_slug)::text)
  AND (
      sqlc.narg(cursor_status_rank)::integer IS NULL
      OR CASE listings.status WHEN 'active' THEN 0 WHEN 'reserved' THEN 1 ELSE 2 END > sqlc.narg(cursor_status_rank)::integer
      OR (
          CASE listings.status WHEN 'active' THEN 0 WHEN 'reserved' THEN 1 ELSE 2 END = sqlc.narg(cursor_status_rank)::integer
          AND (listings.updated_at, listings.id) < (sqlc.narg(cursor_updated_at)::timestamptz, sqlc.narg(cursor_id)::bigint)
      )
  )
ORDER BY CASE listings.status WHEN 'active' THEN 0 WHEN 'reserved' THEN 1 ELSE 2 END,
         listings.updated_at DESC,
         listings.id DESC
LIMIT sqlc.arg(page_limit)::integer;

-- name: SearchListingsNewest :many
SELECT listings.id,
       listings.seller_id,
       listings.category_id,
       listings.title,
       listings.description,
       listings.price_idr,
       listings.quantity,
       listings.condition,
       listings.status,
       listings.moderation_status,
       listings.negotiable,
       listings.created_at,
       listings.updated_at,
       categories.slug AS category_slug,
       categories.name AS category_name,
       users.handle AS seller_handle,
       users.display_name AS seller_display_name,
       users.avatar_url AS seller_avatar_url,
       users.location AS seller_location,
       users.bio AS seller_bio
FROM listings
JOIN categories ON categories.id = listings.category_id
JOIN users ON users.id = listings.seller_id
WHERE listings.status IN ('active', 'reserved')
  AND listings.moderation_status = 'visible'
  AND (
      sqlc.narg(query_text)::text IS NULL
      OR listings.title ILIKE '%' || sqlc.narg(query_text)::text || '%' ESCAPE E'\\'
      OR listings.description ILIKE '%' || sqlc.narg(query_text)::text || '%' ESCAPE E'\\'
  )
  AND (sqlc.narg(category_slug)::text IS NULL OR categories.slug = sqlc.narg(category_slug)::text)
  AND (sqlc.narg(condition_filter)::text IS NULL OR listings.condition = sqlc.narg(condition_filter)::text)
  AND (sqlc.narg(min_price)::bigint IS NULL OR listings.price_idr >= sqlc.narg(min_price)::bigint)
  AND (sqlc.narg(max_price)::bigint IS NULL OR listings.price_idr <= sqlc.narg(max_price)::bigint)
  AND (
      sqlc.narg(cursor_created_at)::timestamptz IS NULL
      OR (listings.created_at, listings.id) < (sqlc.narg(cursor_created_at)::timestamptz, sqlc.narg(cursor_id)::bigint)
  )
ORDER BY listings.created_at DESC, listings.id DESC
LIMIT sqlc.arg(page_limit)::integer;

-- name: SearchListingsPriceAscending :many
SELECT listings.id,
       listings.seller_id,
       listings.category_id,
       listings.title,
       listings.description,
       listings.price_idr,
       listings.quantity,
       listings.condition,
       listings.status,
       listings.moderation_status,
       listings.negotiable,
       listings.created_at,
       listings.updated_at,
       categories.slug AS category_slug,
       categories.name AS category_name,
       users.handle AS seller_handle,
       users.display_name AS seller_display_name,
       users.avatar_url AS seller_avatar_url,
       users.location AS seller_location,
       users.bio AS seller_bio
FROM listings
JOIN categories ON categories.id = listings.category_id
JOIN users ON users.id = listings.seller_id
WHERE listings.status IN ('active', 'reserved')
  AND listings.moderation_status = 'visible'
  AND (
      sqlc.narg(query_text)::text IS NULL
      OR listings.title ILIKE '%' || sqlc.narg(query_text)::text || '%' ESCAPE E'\\'
      OR listings.description ILIKE '%' || sqlc.narg(query_text)::text || '%' ESCAPE E'\\'
  )
  AND (sqlc.narg(category_slug)::text IS NULL OR categories.slug = sqlc.narg(category_slug)::text)
  AND (sqlc.narg(condition_filter)::text IS NULL OR listings.condition = sqlc.narg(condition_filter)::text)
  AND (sqlc.narg(min_price)::bigint IS NULL OR listings.price_idr >= sqlc.narg(min_price)::bigint)
  AND (sqlc.narg(max_price)::bigint IS NULL OR listings.price_idr <= sqlc.narg(max_price)::bigint)
  AND (
      sqlc.narg(cursor_price_idr)::bigint IS NULL
      OR (listings.price_idr, listings.id) > (sqlc.narg(cursor_price_idr)::bigint, sqlc.narg(cursor_id)::bigint)
  )
ORDER BY listings.price_idr ASC, listings.id ASC
LIMIT sqlc.arg(page_limit)::integer;

-- name: SearchListingsPriceDescending :many
SELECT listings.id,
       listings.seller_id,
       listings.category_id,
       listings.title,
       listings.description,
       listings.price_idr,
       listings.quantity,
       listings.condition,
       listings.status,
       listings.moderation_status,
       listings.negotiable,
       listings.created_at,
       listings.updated_at,
       categories.slug AS category_slug,
       categories.name AS category_name,
       users.handle AS seller_handle,
       users.display_name AS seller_display_name,
       users.avatar_url AS seller_avatar_url,
       users.location AS seller_location,
       users.bio AS seller_bio
FROM listings
JOIN categories ON categories.id = listings.category_id
JOIN users ON users.id = listings.seller_id
WHERE listings.status IN ('active', 'reserved')
  AND listings.moderation_status = 'visible'
  AND (
      sqlc.narg(query_text)::text IS NULL
      OR listings.title ILIKE '%' || sqlc.narg(query_text)::text || '%' ESCAPE E'\\'
      OR listings.description ILIKE '%' || sqlc.narg(query_text)::text || '%' ESCAPE E'\\'
  )
  AND (sqlc.narg(category_slug)::text IS NULL OR categories.slug = sqlc.narg(category_slug)::text)
  AND (sqlc.narg(condition_filter)::text IS NULL OR listings.condition = sqlc.narg(condition_filter)::text)
  AND (sqlc.narg(min_price)::bigint IS NULL OR listings.price_idr >= sqlc.narg(min_price)::bigint)
  AND (sqlc.narg(max_price)::bigint IS NULL OR listings.price_idr <= sqlc.narg(max_price)::bigint)
  AND (
      sqlc.narg(cursor_price_idr)::bigint IS NULL
      OR (listings.price_idr, listings.id) < (sqlc.narg(cursor_price_idr)::bigint, sqlc.narg(cursor_id)::bigint)
  )
ORDER BY listings.price_idr DESC, listings.id DESC
LIMIT sqlc.arg(page_limit)::integer;
