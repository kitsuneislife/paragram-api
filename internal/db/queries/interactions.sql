-- name: PostVisible :one
SELECT EXISTS (SELECT 1 FROM posts WHERE id = $1 AND post_ready(id));

-- name: Like :one
-- No row means already liked.
INSERT INTO likes (user_id, post_id) VALUES ($1, $2)
ON CONFLICT DO NOTHING
RETURNING created_at;

-- name: Unlike :one
-- No row means not liked.
DELETE FROM likes WHERE user_id = $1 AND post_id = $2 RETURNING created_at;

-- name: AddLikesCount :exec
UPDATE posts SET likes_count = likes_count + @delta WHERE id = @id;

-- name: AddCommentsCount :exec
UPDATE posts SET comments_count = comments_count + @delta WHERE id = @id;

-- name: CreateComment :one
-- No row means the author already used this idempotency key.
INSERT INTO comments (id, post_id, author_id, body, idempotency_key) VALUES ($1, $2, $3, $4, $5)
ON CONFLICT (author_id, idempotency_key) DO NOTHING
RETURNING *;

-- name: CommentByKey :one
SELECT * FROM comments WHERE author_id = $1 AND idempotency_key = $2;

-- name: DeleteComment :one
-- The comment's author or the post's author may delete it.
DELETE FROM comments c
WHERE c.id = @id AND (c.author_id = @user_id OR c.post_id IN (SELECT p.id FROM posts p WHERE p.author_id = @user_id))
RETURNING c.post_id;

-- name: ListComments :many
-- Oldest first, after the cursor.
SELECT sqlc.embed(c), sqlc.embed(u)
FROM comments c JOIN users u ON u.id = c.author_id
WHERE c.post_id = @post_id
  AND (c.created_at, c.id) > (@cursor_time::timestamptz, @cursor_id::uuid)
ORDER BY c.created_at, c.id
LIMIT @lim;
