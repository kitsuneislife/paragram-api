-- +goose Up
CREATE TABLE posts (
    id              uuid PRIMARY KEY,
    author_id       uuid NOT NULL REFERENCES users ON DELETE CASCADE,
    caption         text NOT NULL DEFAULT '' CHECK (char_length(caption) <= 2200),
    likes_count     integer NOT NULL DEFAULT 0 CHECK (likes_count >= 0),
    comments_count  integer NOT NULL DEFAULT 0 CHECK (comments_count >= 0),
    -- Client-chosen key that makes a retried create return the same post.
    idempotency_key uuid,
    created_at      timestamptz NOT NULL DEFAULT now(),
    updated_at      timestamptz NOT NULL DEFAULT now(),
    UNIQUE (author_id, idempotency_key)
);
CREATE INDEX posts_author_created ON posts (author_id, created_at DESC, id DESC);

-- The carousel: 1 to 10 media in order.
CREATE TABLE post_media (
    post_id  uuid NOT NULL REFERENCES posts ON DELETE CASCADE,
    media_id uuid NOT NULL UNIQUE REFERENCES media ON DELETE CASCADE,
    position smallint NOT NULL CHECK (position BETWEEN 0 AND 9),
    PRIMARY KEY (post_id, position)
);

-- A post is shown to others only once every media is processed. This is the
-- one definition of that rule; queries call it.
-- +goose StatementBegin
CREATE FUNCTION post_ready(post uuid) RETURNS boolean LANGUAGE sql STABLE AS $$
    SELECT NOT EXISTS (
        SELECT 1 FROM post_media pm JOIN media m ON m.id = pm.media_id
        WHERE pm.post_id = post AND m.status <> 'ready'
    )
$$;
-- +goose StatementEnd

-- +goose Down
DROP FUNCTION post_ready;
DROP TABLE post_media;
DROP TABLE posts;
