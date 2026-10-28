-- +goose Up
CREATE TABLE follows (
    follower_id uuid NOT NULL REFERENCES users ON DELETE CASCADE,
    followee_id uuid NOT NULL REFERENCES users ON DELETE CASCADE,
    created_at  timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (follower_id, followee_id),
    CHECK (follower_id <> followee_id)
);
-- Followers and following lists, newest first.
CREATE INDEX follows_followee_created ON follows (followee_id, created_at DESC, follower_id DESC);
CREATE INDEX follows_follower_created ON follows (follower_id, created_at DESC, followee_id DESC);

-- +goose Down
DROP TABLE follows;
