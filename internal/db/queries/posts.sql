-- name: CreatePost :one
-- No row means the author already used this idempotency key.
INSERT INTO posts (id, author_id, caption, idempotency_key) VALUES ($1, $2, $3, $4)
ON CONFLICT (author_id, idempotency_key) DO NOTHING
RETURNING id;

-- name: PostIDByKey :one
SELECT id FROM posts WHERE author_id = $1 AND idempotency_key = $2;

-- name: AddPostMedia :exec
INSERT INTO post_media (post_id, media_id, position)
SELECT @post_id, m.id, m.ord - 1
FROM unnest(@media_ids::uuid[]) WITH ORDINALITY AS m(id, ord);

-- name: AddPostsCount :exec
UPDATE users SET posts_count = posts_count + @delta WHERE id = @id;

-- name: DeletePost :one
DELETE FROM posts WHERE id = $1 AND author_id = $2 RETURNING id;

-- name: GetPosts :many
-- Full rows for ids, in no particular order.
SELECT sqlc.embed(p), sqlc.embed(u), post_ready(p.id)::bool AS ready
FROM posts p JOIN users u ON u.id = p.author_id
WHERE p.id = ANY(@ids::uuid[]);

-- name: ListPostMedia :many
SELECT pm.post_id, sqlc.embed(m)
FROM post_media pm JOIN media m ON m.id = pm.media_id
WHERE pm.post_id = ANY(@post_ids::uuid[])
ORDER BY pm.post_id, pm.position;

-- name: ListUserPosts :many
-- Newest first, before the cursor. Unready posts only for their author.
SELECT id, created_at FROM posts
WHERE author_id = @author_id
  AND (created_at, id) < (@cursor_time::timestamptz, @cursor_id::uuid)
  AND (@include_unready::bool OR post_ready(id))
ORDER BY created_at DESC, id DESC
LIMIT @lim;
