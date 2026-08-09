-- name: ListActiveCategories :many
SELECT id, slug, name
FROM categories
WHERE active = TRUE
ORDER BY sort_order, id;

-- name: GetActiveCategoryBySlug :one
SELECT id, slug, name
FROM categories
WHERE slug = sqlc.arg(slug)::text
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
    )
    VALUES (
        sqlc.arg(seller_id)::bigint,
        sqlc.arg(category_id)::bigint,
        sqlc.arg(title)::text,
        sqlc.arg(description)::text,
        sqlc.arg(price_idr)::bigint,
        sqlc.arg(quantity)::integer,
        sqlc.arg(condition)::text,
        sqlc.arg(status)::text,
        sqlc.arg(moderation_status)::text,
        sqlc.arg(negotiable)::boolean,
        sqlc.arg(created_at)::timestamptz,
        sqlc.arg(updated_at)::timestamptz
    )
    RETURNING *
)
SELECT
    listing.id AS listing_id,
    listing.seller_id AS listing_seller_id,
    listing.category_id AS listing_category_id,
    listing.title,
    listing.description,
    listing.price_idr,
    listing.quantity,
    listing.condition,
    listing.status,
    listing.moderation_status,
    listing.negotiable,
    listing.created_at,
    listing.updated_at,
    category.id AS category_id,
    category.slug AS category_slug,
    category.name AS category_name,
    seller.id AS seller_id,
    seller.handle AS seller_handle,
    seller.display_name AS seller_display_name,
    seller.avatar_url AS seller_avatar_url,
    seller.location AS seller_location
FROM inserted AS listing
JOIN categories AS category ON category.id = listing.category_id
JOIN users AS seller ON seller.id = listing.seller_id;

-- name: GetListingByID :one
SELECT
    listing.id AS listing_id,
    listing.seller_id AS listing_seller_id,
    listing.category_id AS listing_category_id,
    listing.title,
    listing.description,
    listing.price_idr,
    listing.quantity,
    listing.condition,
    listing.status,
    listing.moderation_status,
    listing.negotiable,
    listing.created_at,
    listing.updated_at,
    category.id AS category_id,
    category.slug AS category_slug,
    category.name AS category_name,
    seller.id AS seller_id,
    seller.handle AS seller_handle,
    seller.display_name AS seller_display_name,
    seller.avatar_url AS seller_avatar_url,
    seller.location AS seller_location
FROM listings AS listing
JOIN categories AS category ON category.id = listing.category_id
JOIN users AS seller ON seller.id = listing.seller_id
WHERE listing.id = sqlc.arg(listing_id)::bigint;

-- name: UpdateOwnedListing :one
WITH updated AS (
    UPDATE listings
    SET
        title = CASE WHEN sqlc.arg(has_title)::boolean THEN sqlc.arg(title)::text ELSE title END,
        description = CASE WHEN sqlc.arg(has_description)::boolean THEN sqlc.arg(description)::text ELSE description END,
        price_idr = CASE WHEN sqlc.arg(has_price_idr)::boolean THEN sqlc.arg(price_idr)::bigint ELSE price_idr END,
        quantity = CASE WHEN sqlc.arg(has_quantity)::boolean THEN sqlc.arg(quantity)::integer ELSE quantity END,
        category_id = CASE WHEN sqlc.arg(has_category_id)::boolean THEN sqlc.arg(category_id)::bigint ELSE category_id END,
        condition = CASE WHEN sqlc.arg(has_condition)::boolean THEN sqlc.arg(condition)::text ELSE condition END,
        negotiable = CASE WHEN sqlc.arg(has_negotiable)::boolean THEN sqlc.arg(negotiable)::boolean ELSE negotiable END,
        updated_at = sqlc.arg(updated_at)::timestamptz
    WHERE id = sqlc.arg(listing_id)::bigint
      AND seller_id = sqlc.arg(seller_id)::bigint
    RETURNING *
)
SELECT
    listing.id AS listing_id,
    listing.seller_id AS listing_seller_id,
    listing.category_id AS listing_category_id,
    listing.title,
    listing.description,
    listing.price_idr,
    listing.quantity,
    listing.condition,
    listing.status,
    listing.moderation_status,
    listing.negotiable,
    listing.created_at,
    listing.updated_at,
    category.id AS category_id,
    category.slug AS category_slug,
    category.name AS category_name,
    seller.id AS seller_id,
    seller.handle AS seller_handle,
    seller.display_name AS seller_display_name,
    seller.avatar_url AS seller_avatar_url,
    seller.location AS seller_location
FROM updated AS listing
JOIN categories AS category ON category.id = listing.category_id
JOIN users AS seller ON seller.id = listing.seller_id;

-- name: UpdateOwnedListingStatus :one
WITH updated AS (
    UPDATE listings
    SET status = sqlc.arg(status)::text,
        updated_at = sqlc.arg(updated_at)::timestamptz
    WHERE id = sqlc.arg(listing_id)::bigint
      AND seller_id = sqlc.arg(seller_id)::bigint
    RETURNING *
)
SELECT
    listing.id AS listing_id,
    listing.seller_id AS listing_seller_id,
    listing.category_id AS listing_category_id,
    listing.title,
    listing.description,
    listing.price_idr,
    listing.quantity,
    listing.condition,
    listing.status,
    listing.moderation_status,
    listing.negotiable,
    listing.created_at,
    listing.updated_at,
    category.id AS category_id,
    category.slug AS category_slug,
    category.name AS category_name,
    seller.id AS seller_id,
    seller.handle AS seller_handle,
    seller.display_name AS seller_display_name,
    seller.avatar_url AS seller_avatar_url,
    seller.location AS seller_location
FROM updated AS listing
JOIN categories AS category ON category.id = listing.category_id
JOIN users AS seller ON seller.id = listing.seller_id;

-- name: ListOwnedListings :many
SELECT
    listing.id AS listing_id,
    listing.seller_id AS listing_seller_id,
    listing.category_id AS listing_category_id,
    listing.title,
    listing.description,
    listing.price_idr,
    listing.quantity,
    listing.condition,
    listing.status,
    listing.moderation_status,
    listing.negotiable,
    listing.created_at,
    listing.updated_at,
    category.id AS category_id,
    category.slug AS category_slug,
    category.name AS category_name,
    seller.id AS seller_id,
    seller.handle AS seller_handle,
    seller.display_name AS seller_display_name,
    seller.avatar_url AS seller_avatar_url,
    seller.location AS seller_location
FROM listings AS listing
JOIN categories AS category ON category.id = listing.category_id
JOIN users AS seller ON seller.id = listing.seller_id
WHERE listing.seller_id = sqlc.arg(seller_id)::bigint
  AND listing.moderation_status = 'visible'
  AND (NOT sqlc.arg(has_status)::boolean OR listing.status = sqlc.arg(status)::text)
  AND (
      NOT sqlc.arg(has_cursor)::boolean
      OR (listing.updated_at, listing.id) < (
          sqlc.arg(cursor_updated_at)::timestamptz,
          sqlc.arg(cursor_id)::bigint
      )
  )
ORDER BY listing.updated_at DESC, listing.id DESC
LIMIT sqlc.arg(page_size)::integer;

-- name: SearchListingsNewest :many
SELECT
    listing.id AS listing_id,
    listing.seller_id AS listing_seller_id,
    listing.category_id AS listing_category_id,
    listing.title,
    listing.description,
    listing.price_idr,
    listing.quantity,
    listing.condition,
    listing.status,
    listing.moderation_status,
    listing.negotiable,
    listing.created_at,
    listing.updated_at,
    category.id AS category_id,
    category.slug AS category_slug,
    category.name AS category_name,
    seller.id AS seller_id,
    seller.handle AS seller_handle,
    seller.display_name AS seller_display_name,
    seller.avatar_url AS seller_avatar_url,
    seller.location AS seller_location
FROM listings AS listing
JOIN categories AS category ON category.id = listing.category_id
JOIN users AS seller ON seller.id = listing.seller_id
WHERE listing.status IN ('active', 'reserved')
  AND listing.moderation_status = 'visible'
  AND (
      NOT sqlc.arg(has_query)::boolean
      OR listing.title ILIKE '%' || sqlc.arg(query)::text || '%' ESCAPE E'\\'
      OR listing.description ILIKE '%' || sqlc.arg(query)::text || '%' ESCAPE E'\\'
  )
  AND (NOT sqlc.arg(has_category)::boolean OR category.slug = sqlc.arg(category)::text)
  AND (NOT sqlc.arg(has_condition)::boolean OR listing.condition = sqlc.arg(condition)::text)
  AND (NOT sqlc.arg(has_min_price)::boolean OR listing.price_idr >= sqlc.arg(min_price)::bigint)
  AND (NOT sqlc.arg(has_max_price)::boolean OR listing.price_idr <= sqlc.arg(max_price)::bigint)
  AND (
      NOT sqlc.arg(has_cursor)::boolean
      OR (listing.created_at, listing.id) < (
          sqlc.arg(cursor_created_at)::timestamptz,
          sqlc.arg(cursor_id)::bigint
      )
  )
ORDER BY listing.created_at DESC, listing.id DESC
LIMIT sqlc.arg(page_size)::integer;

-- name: SearchListingsPriceAscending :many
SELECT
    listing.id AS listing_id,
    listing.seller_id AS listing_seller_id,
    listing.category_id AS listing_category_id,
    listing.title,
    listing.description,
    listing.price_idr,
    listing.quantity,
    listing.condition,
    listing.status,
    listing.moderation_status,
    listing.negotiable,
    listing.created_at,
    listing.updated_at,
    category.id AS category_id,
    category.slug AS category_slug,
    category.name AS category_name,
    seller.id AS seller_id,
    seller.handle AS seller_handle,
    seller.display_name AS seller_display_name,
    seller.avatar_url AS seller_avatar_url,
    seller.location AS seller_location
FROM listings AS listing
JOIN categories AS category ON category.id = listing.category_id
JOIN users AS seller ON seller.id = listing.seller_id
WHERE listing.status IN ('active', 'reserved')
  AND listing.moderation_status = 'visible'
  AND (
      NOT sqlc.arg(has_query)::boolean
      OR listing.title ILIKE '%' || sqlc.arg(query)::text || '%' ESCAPE E'\\'
      OR listing.description ILIKE '%' || sqlc.arg(query)::text || '%' ESCAPE E'\\'
  )
  AND (NOT sqlc.arg(has_category)::boolean OR category.slug = sqlc.arg(category)::text)
  AND (NOT sqlc.arg(has_condition)::boolean OR listing.condition = sqlc.arg(condition)::text)
  AND (NOT sqlc.arg(has_min_price)::boolean OR listing.price_idr >= sqlc.arg(min_price)::bigint)
  AND (NOT sqlc.arg(has_max_price)::boolean OR listing.price_idr <= sqlc.arg(max_price)::bigint)
  AND (
      NOT sqlc.arg(has_cursor)::boolean
      OR (listing.price_idr, listing.id) > (
          sqlc.arg(cursor_price_idr)::bigint,
          sqlc.arg(cursor_id)::bigint
      )
  )
ORDER BY listing.price_idr ASC, listing.id ASC
LIMIT sqlc.arg(page_size)::integer;

-- name: SearchListingsPriceDescending :many
SELECT
    listing.id AS listing_id,
    listing.seller_id AS listing_seller_id,
    listing.category_id AS listing_category_id,
    listing.title,
    listing.description,
    listing.price_idr,
    listing.quantity,
    listing.condition,
    listing.status,
    listing.moderation_status,
    listing.negotiable,
    listing.created_at,
    listing.updated_at,
    category.id AS category_id,
    category.slug AS category_slug,
    category.name AS category_name,
    seller.id AS seller_id,
    seller.handle AS seller_handle,
    seller.display_name AS seller_display_name,
    seller.avatar_url AS seller_avatar_url,
    seller.location AS seller_location
FROM listings AS listing
JOIN categories AS category ON category.id = listing.category_id
JOIN users AS seller ON seller.id = listing.seller_id
WHERE listing.status IN ('active', 'reserved')
  AND listing.moderation_status = 'visible'
  AND (
      NOT sqlc.arg(has_query)::boolean
      OR listing.title ILIKE '%' || sqlc.arg(query)::text || '%' ESCAPE E'\\'
      OR listing.description ILIKE '%' || sqlc.arg(query)::text || '%' ESCAPE E'\\'
  )
  AND (NOT sqlc.arg(has_category)::boolean OR category.slug = sqlc.arg(category)::text)
  AND (NOT sqlc.arg(has_condition)::boolean OR listing.condition = sqlc.arg(condition)::text)
  AND (NOT sqlc.arg(has_min_price)::boolean OR listing.price_idr >= sqlc.arg(min_price)::bigint)
  AND (NOT sqlc.arg(has_max_price)::boolean OR listing.price_idr <= sqlc.arg(max_price)::bigint)
  AND (
      NOT sqlc.arg(has_cursor)::boolean
      OR (listing.price_idr, listing.id) < (
          sqlc.arg(cursor_price_idr)::bigint,
          sqlc.arg(cursor_id)::bigint
      )
  )
ORDER BY listing.price_idr DESC, listing.id DESC
LIMIT sqlc.arg(page_size)::integer;
