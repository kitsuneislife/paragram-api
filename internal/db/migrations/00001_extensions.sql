-- +goose Up
CREATE EXTENSION IF NOT EXISTS citext;  -- case-insensitive username and email
CREATE EXTENSION IF NOT EXISTS pg_trgm; -- fuzzy search on names

-- +goose Down
DROP EXTENSION IF EXISTS pg_trgm;
DROP EXTENSION IF EXISTS citext;
