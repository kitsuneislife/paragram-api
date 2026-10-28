-- +goose Up
CREATE TABLE likes (
    user_id    uuid NOT NULL REFERENCES users ON DELETE CASCADE,
    post_id    uuid NOT NULL REFERENCES posts ON DELETE CASCADE,
    created_at timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (user_id, post_id)
);
CREATE INDEX likes_post_id ON likes (post_id);

CREATE TABLE comments (
    id              uuid PRIMARY KEY,
    post_id         uuid NOT NULL REFERENCES posts ON DELETE CASCADE,
    author_id       uuid NOT NULL REFERENCES users ON DELETE CASCADE,
    body            text NOT NULL CHECK (char_length(body) BETWEEN 1 AND 2200),
    -- Client-chosen key that makes a retried create return the same comment.
    idempotency_key uuid,
    created_at      timestamptz NOT NULL DEFAULT now(),
    UNIQUE (author_id, idempotency_key)
);
CREATE INDEX comments_post_created ON comments (post_id, created_at, id);

-- +goose Down
DROP TABLE comments;
DROP TABLE likes;
