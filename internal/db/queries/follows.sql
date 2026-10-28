-- name: Follow :one
-- No row means already following.
INSERT INTO follows (follower_id, followee_id) VALUES ($1, $2)
ON CONFLICT DO NOTHING
RETURNING created_at;

-- name: Unfollow :one
-- No row means not following.
DELETE FROM follows WHERE follower_id = $1 AND followee_id = $2 RETURNING created_at;

-- name: AddFollowCounts :exec
UPDATE users SET
    following_count = following_count + CASE WHEN id = @follower_id THEN @delta::int ELSE 0 END,
    followers_count = followers_count + CASE WHEN id = @followee_id THEN @delta::int ELSE 0 END
WHERE id IN (@follower_id, @followee_id);

-- name: IsFollowing :one
SELECT EXISTS (SELECT 1 FROM follows WHERE follower_id = $1 AND followee_id = $2);

-- name: ListFollowers :many
SELECT sqlc.embed(u), f.created_at AS followed_at,
    EXISTS (SELECT 1 FROM follows m WHERE m.follower_id = @viewer AND m.followee_id = u.id)::bool AS followed_by_me
FROM follows f JOIN users u ON u.id = f.follower_id
WHERE f.followee_id = @user_id
  AND (f.created_at, f.follower_id) < (@cursor_time::timestamptz, @cursor_id::uuid)
ORDER BY f.created_at DESC, f.follower_id DESC
LIMIT @lim;

-- name: ListFollowing :many
SELECT sqlc.embed(u), f.created_at AS followed_at,
    EXISTS (SELECT 1 FROM follows m WHERE m.follower_id = @viewer AND m.followee_id = u.id)::bool AS followed_by_me
FROM follows f JOIN users u ON u.id = f.followee_id
WHERE f.follower_id = @user_id
  AND (f.created_at, f.followee_id) < (@cursor_time::timestamptz, @cursor_id::uuid)
ORDER BY f.created_at DESC, f.followee_id DESC
LIMIT @lim;

-- name: Feed :many
-- Ready posts by the viewer and everyone they follow, newest first.
-- shortcut: fanout on read, slow once users follow thousands; see roadmap for fanout on write.
SELECT p.id, p.created_at FROM posts p
WHERE (p.author_id = @viewer OR p.author_id IN (SELECT followee_id FROM follows WHERE follower_id = @viewer))
  AND (p.created_at, p.id) < (@cursor_time::timestamptz, @cursor_id::uuid)
  AND post_ready(p.id)
ORDER BY p.created_at DESC, p.id DESC
LIMIT @lim;
