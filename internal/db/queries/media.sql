-- name: CreateMedia :one
INSERT INTO media (id, owner_id, purpose) VALUES ($1, $2, $3) RETURNING *;

-- name: GetMedia :one
SELECT * FROM media WHERE id = $1;

-- name: StartProcessing :many
-- Claims the caller's pending uploads. Fewer rows than ids means some were
-- not theirs, not pending, or meant for something else.
UPDATE media SET status = 'processing', updated_at = now()
WHERE id = ANY(@ids::uuid[]) AND owner_id = @owner_id AND purpose = @purpose AND status = 'pending'
RETURNING id;

-- name: MediaReady :exec
UPDATE media SET status = 'ready', width = @width::int, height = @height::int, updated_at = now()
WHERE id = @id AND status = 'processing';

-- name: MediaFailed :exec
UPDATE media SET status = 'failed', updated_at = now()
WHERE id = $1 AND status = 'processing';

-- name: SetAvatar :exec
UPDATE users SET avatar_media_id = @media_id, updated_at = now() WHERE id = @user_id;
