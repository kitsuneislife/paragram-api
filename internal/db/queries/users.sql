-- name: CreateUser :one
INSERT INTO users (id, username, email, password_hash, display_name)
VALUES ($1, $2, $3, $4, $5)
RETURNING *;

-- name: GetUserByLogin :one
-- login is a username or an email; both are stored lowercase.
SELECT * FROM users WHERE username = lower(@login::text) OR email = lower(@login::text);

-- name: GetUser :one
SELECT * FROM users WHERE id = $1;

-- name: GetUserByUsername :one
SELECT * FROM users WHERE username = lower(@username::text);

-- name: UpdateUser :one
-- NULL leaves a field unchanged.
UPDATE users SET
    username     = coalesce(sqlc.narg(username), username),
    display_name = coalesce(sqlc.narg(display_name), display_name),
    bio          = coalesce(sqlc.narg(bio), bio),
    website      = coalesce(sqlc.narg(website), website),
    updated_at   = now()
WHERE id = @id
RETURNING *;
