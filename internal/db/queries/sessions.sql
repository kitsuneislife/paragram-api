-- name: CreateSession :exec
INSERT INTO sessions (id, user_id, refresh_hash) VALUES ($1, $2, $3);

-- name: GetSessionForUpdate :one
SELECT * FROM sessions WHERE id = $1 FOR UPDATE;

-- name: RotateSession :exec
UPDATE sessions SET refresh_hash = $2, updated_at = now() WHERE id = $1;

-- name: SessionActive :one
SELECT EXISTS (SELECT 1 FROM sessions WHERE id = $1 AND user_id = $2 AND revoked_at IS NULL);

-- name: RevokeSession :exec
UPDATE sessions SET revoked_at = now() WHERE id = $1 AND revoked_at IS NULL;
