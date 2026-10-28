-- +goose Up
-- One uploaded image. The original sits at uploads/<id> in the bucket; the
-- worker writes the sizes to media/<id>/<size>.jpg.
-- pending: waiting for upload; processing: attached to a post or avatar, job
-- queued; ready: sizes written; failed: not a usable image.
CREATE TABLE media (
    id         uuid PRIMARY KEY,
    owner_id   uuid NOT NULL REFERENCES users ON DELETE CASCADE,
    purpose    text NOT NULL CHECK (purpose IN ('post', 'avatar')),
    status     text NOT NULL DEFAULT 'pending' CHECK (status IN ('pending', 'processing', 'ready', 'failed')),
    -- Size of the full variant, known once ready.
    width      integer CHECK (width > 0),
    height     integer CHECK (height > 0),
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    CHECK ((status = 'ready') = (width IS NOT NULL AND height IS NOT NULL))
);
CREATE INDEX media_owner_id ON media (owner_id);

-- Set by the worker only once the avatar is ready, so it always points at a
-- displayable image.
ALTER TABLE users ADD COLUMN avatar_media_id uuid REFERENCES media ON DELETE SET NULL;

-- +goose Down
ALTER TABLE users DROP COLUMN avatar_media_id;
DROP TABLE media;
