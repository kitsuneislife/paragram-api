-- +goose Up
CREATE TABLE users (
    id              uuid PRIMARY KEY,
    -- Stored lowercase, so plain UNIQUE is already case-insensitive.
    username        text NOT NULL UNIQUE CHECK (username ~ '^[a-z0-9._]{1,30}$'),
    email           citext NOT NULL UNIQUE CHECK (char_length(email) BETWEEN 3 AND 254 AND email LIKE '%_@_%'),
    password_hash   text NOT NULL,
    display_name    text NOT NULL DEFAULT '' CHECK (char_length(display_name) <= 50),
    bio             text NOT NULL DEFAULT '' CHECK (char_length(bio) <= 150),
    website         text NOT NULL DEFAULT '' CHECK (char_length(website) <= 200),
    posts_count     integer NOT NULL DEFAULT 0 CHECK (posts_count >= 0),
    followers_count integer NOT NULL DEFAULT 0 CHECK (followers_count >= 0),
    following_count integer NOT NULL DEFAULT 0 CHECK (following_count >= 0),
    created_at      timestamptz NOT NULL DEFAULT now(),
    updated_at      timestamptz NOT NULL DEFAULT now()
);

-- One row per login. The refresh token rotates on every use; presenting an
-- older one revokes the session (see docs/api-conventions.md).
CREATE TABLE sessions (
    id           uuid PRIMARY KEY,
    user_id      uuid NOT NULL REFERENCES users ON DELETE CASCADE,
    refresh_hash bytea NOT NULL,
    created_at   timestamptz NOT NULL DEFAULT now(),
    updated_at   timestamptz NOT NULL DEFAULT now(),
    revoked_at   timestamptz
);
CREATE INDEX sessions_user_id ON sessions (user_id);

-- +goose Down
DROP TABLE sessions;
DROP TABLE users;
