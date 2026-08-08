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
INSERT INTO listings (
    seller_id,
    category_id,
    title,
    description,
    price_idr,
    quantity,
    condition,
    negotiable,
    created_at,
    updated_at
)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $9)
RETURNING id;

-- name: GetListingByID :one
SELECT sqlc.embed(listings), sqlc.embed(categories), sqlc.embed(users)
FROM listings
JOIN categories ON categories.id = listings.category_id
JOIN users ON users.id = listings.seller_id
WHERE listings.id = $1;

-- name: UpdateListing :one
UPDATE listings
SET title = COALESCE(sqlc.narg('title')::text, title),
    description = COALESCE(sqlc.narg('description')::text, description),
    price_idr = COALESCE(sqlc.narg('price_idr')::bigint, price_idr),
    quantity = COALESCE(sqlc.narg('quantity')::integer, quantity),
    category_id = COALESCE(sqlc.narg('category_id')::bigint, category_id),
    condition = COALESCE(sqlc.narg('condition')::text, condition),
    negotiable = COALESCE(sqlc.narg('negotiable')::boolean, negotiable),
    updated_at = @updated_at
WHERE listings.id = @id
  AND listings.seller_id = @seller_id
RETURNING id;

-- name: ChangeListingStatus :one
UPDATE listings
SET status = @status,
    updated_at = @updated_at
WHERE listings.id = @id
  AND listings.seller_id = @seller_id
RETURNING id;

-- name: ListOwnedListings :many
SELECT sqlc.embed(listings), sqlc.embed(categories), sqlc.embed(users)
FROM listings
JOIN categories ON categories.id = listings.category_id
JOIN users ON users.id = listings.seller_id
WHERE listings.seller_id = @seller_id
  AND listings.moderation_status = 'visible'
  AND (sqlc.narg('status')::text IS NULL OR listings.status = sqlc.narg('status')::text)
  AND (
      sqlc.narg('cursor_updated_at')::timestamptz IS NULL
      OR (listings.updated_at, listings.id) < (
          sqlc.narg('cursor_updated_at')::timestamptz,
          sqlc.narg('cursor_id')::bigint
      )
  )
ORDER BY listings.updated_at DESC, listings.id DESC
LIMIT @page_limit;

-- name: SearchListingsNewest :many
SELECT sqlc.embed(listings), sqlc.embed(categories), sqlc.embed(users)
FROM listings
JOIN categories ON categories.id = listings.category_id
JOIN users ON users.id = listings.seller_id
WHERE listings.status IN ('active', 'reserved')
  AND listings.moderation_status = 'visible'
  AND (sqlc.narg('query')::text IS NULL OR (
      listings.title ILIKE '%' || sqlc.narg('query')::text || '%' ESCAPE '\'
      OR listings.description ILIKE '%' || sqlc.narg('query')::text || '%' ESCAPE '\'
  ))
  AND (sqlc.narg('category_id')::bigint IS NULL OR listings.category_id = sqlc.narg('category_id')::bigint)
  AND (sqlc.narg('condition')::text IS NULL OR listings.condition = sqlc.narg('condition')::text)
  AND (sqlc.narg('min_price')::bigint IS NULL OR listings.price_idr >= sqlc.narg('min_price')::bigint)
  AND (sqlc.narg('max_price')::bigint IS NULL OR listings.price_idr <= sqlc.narg('max_price')::bigint)
  AND (
      sqlc.narg('cursor_created_at')::timestamptz IS NULL
      OR (listings.created_at, listings.id) < (
          sqlc.narg('cursor_created_at')::timestamptz,
          sqlc.narg('cursor_id')::bigint
      )
  )
ORDER BY listings.created_at DESC, listings.id DESC
LIMIT @page_limit;

-- name: SearchListingsPriceAsc :many
SELECT sqlc.embed(listings), sqlc.embed(categories), sqlc.embed(users)
FROM listings
JOIN categories ON categories.id = listings.category_id
JOIN users ON users.id = listings.seller_id
WHERE listings.status IN ('active', 'reserved')
  AND listings.moderation_status = 'visible'
  AND (sqlc.narg('query')::text IS NULL OR (
      listings.title ILIKE '%' || sqlc.narg('query')::text || '%' ESCAPE '\'
      OR listings.description ILIKE '%' || sqlc.narg('query')::text || '%' ESCAPE '\'
  ))
  AND (sqlc.narg('category_id')::bigint IS NULL OR listings.category_id = sqlc.narg('category_id')::bigint)
  AND (sqlc.narg('condition')::text IS NULL OR listings.condition = sqlc.narg('condition')::text)
  AND (sqlc.narg('min_price')::bigint IS NULL OR listings.price_idr >= sqlc.narg('min_price')::bigint)
  AND (sqlc.narg('max_price')::bigint IS NULL OR listings.price_idr <= sqlc.narg('max_price')::bigint)
  AND (
      sqlc.narg('cursor_price_idr')::bigint IS NULL
      OR listings.price_idr > sqlc.narg('cursor_price_idr')::bigint
      OR (listings.price_idr = sqlc.narg('cursor_price_idr')::bigint AND listings.id > sqlc.narg('cursor_id')::bigint)
  )
ORDER BY listings.price_idr ASC, listings.id ASC
LIMIT @page_limit;

-- name: SearchListingsPriceDesc :many
SELECT sqlc.embed(listings), sqlc.embed(categories), sqlc.embed(users)
FROM listings
JOIN categories ON categories.id = listings.category_id
JOIN users ON users.id = listings.seller_id
WHERE listings.status IN ('active', 'reserved')
  AND listings.moderation_status = 'visible'
  AND (sqlc.narg('query')::text IS NULL OR (
      listings.title ILIKE '%' || sqlc.narg('query')::text || '%' ESCAPE '\'
      OR listings.description ILIKE '%' || sqlc.narg('query')::text || '%' ESCAPE '\'
  ))
  AND (sqlc.narg('category_id')::bigint IS NULL OR listings.category_id = sqlc.narg('category_id')::bigint)
  AND (sqlc.narg('condition')::text IS NULL OR listings.condition = sqlc.narg('condition')::text)
  AND (sqlc.narg('min_price')::bigint IS NULL OR listings.price_idr >= sqlc.narg('min_price')::bigint)
  AND (sqlc.narg('max_price')::bigint IS NULL OR listings.price_idr <= sqlc.narg('max_price')::bigint)
  AND (
      sqlc.narg('cursor_price_idr')::bigint IS NULL
      OR listings.price_idr < sqlc.narg('cursor_price_idr')::bigint
      OR (listings.price_idr = sqlc.narg('cursor_price_idr')::bigint AND listings.id < sqlc.narg('cursor_id')::bigint)
  )
ORDER BY listings.price_idr DESC, listings.id DESC
LIMIT @page_limit;
