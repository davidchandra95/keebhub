-- +goose Up
-- The foundation intentionally creates no product tables. The first product
-- migration is added with the Discord authentication vertical slice.
SELECT 1;

-- +goose Down
SELECT 1;
