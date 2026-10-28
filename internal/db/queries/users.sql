-- name: CreateUser :one
INSERT INTO users (id, username, email, password_hash, display_name)
VALUES ($1, $2, $3, $4, $5)
RETURNING *;

-- name: GetUserByLogin :one
-- login is a username or an email; both are stored lowercase.
SELECT * FROM users WHERE username = lower(@login::text) OR email = lower(@login::text);

-- name: GetUser :one
SELECT * FROM users WHERE id = $1;
