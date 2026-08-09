-- +goose Up
CREATE TABLE categories (
    id BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    slug TEXT NOT NULL UNIQUE,
    name TEXT NOT NULL,
    sort_order INTEGER NOT NULL,
    active BOOLEAN NOT NULL DEFAULT TRUE,
    CONSTRAINT categories_slug_format CHECK (
        slug = lower(slug)
        AND slug = btrim(slug)
        AND char_length(slug) BETWEEN 1 AND 50
    ),
    CONSTRAINT categories_name_format CHECK (
        char_length(btrim(name)) BETWEEN 1 AND 100
    ),
    CONSTRAINT categories_sort_order_nonnegative CHECK (sort_order >= 0)
);

INSERT INTO categories (slug, name, sort_order)
VALUES
    ('keyboard', 'Keyboard', 10),
    ('keycaps', 'Keycaps', 20),
    ('switches', 'Switches', 30),
    ('parts', 'Parts', 40),
    ('accessories', 'Accessories', 50),
    ('other', 'Other', 60);

CREATE TABLE listings (
    id BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    seller_id BIGINT NOT NULL REFERENCES users(id),
    category_id BIGINT NOT NULL REFERENCES categories(id),
    title TEXT NOT NULL,
    description TEXT NOT NULL DEFAULT '',
    price_idr BIGINT NOT NULL,
    quantity INTEGER NOT NULL DEFAULT 1,
    condition TEXT NOT NULL,
    status TEXT NOT NULL DEFAULT 'active',
    moderation_status TEXT NOT NULL DEFAULT 'visible',
    negotiable BOOLEAN NOT NULL DEFAULT FALSE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT listings_title_length CHECK (char_length(btrim(title)) BETWEEN 1 AND 120),
    CONSTRAINT listings_description_length CHECK (char_length(description) <= 5000),
    CONSTRAINT listings_price_idr_range CHECK (price_idr BETWEEN 1 AND 10000000000),
    CONSTRAINT listings_quantity_range CHECK (quantity BETWEEN 1 AND 1000000),
    CONSTRAINT listings_condition_valid CHECK (condition IN ('new', 'used')),
    CONSTRAINT listings_status_valid CHECK (status IN ('active', 'reserved', 'sold', 'archived')),
    CONSTRAINT listings_moderation_status_valid CHECK (moderation_status IN ('visible', 'removed')),
    CONSTRAINT listings_updated_at_after_created_at CHECK (updated_at >= created_at),
    CONSTRAINT listings_id_seller_id_key UNIQUE (id, seller_id)
);

CREATE INDEX listings_status_created_at_id_idx
    ON listings (status, created_at DESC, id DESC);
CREATE INDEX listings_category_id_status_created_at_idx
    ON listings (category_id, status, created_at DESC, id DESC);
CREATE INDEX listings_seller_id_status_updated_at_idx
    ON listings (seller_id, status, updated_at DESC, id DESC);
CREATE INDEX listings_price_id_idx ON listings (price_idr, id);

-- +goose Down
DROP TABLE listings;
DROP TABLE categories;
