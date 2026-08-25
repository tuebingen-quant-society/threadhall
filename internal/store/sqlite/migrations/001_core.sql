CREATE TABLE users (
    id INTEGER PRIMARY KEY,
    username TEXT NOT NULL UNIQUE,
    password_hash TEXT NOT NULL,
    is_admin INTEGER NOT NULL DEFAULT 0 CHECK (is_admin IN (0, 1)),
    created_at INTEGER NOT NULL
);

CREATE TABLE sessions (
    id INTEGER PRIMARY KEY,
    user_id INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    token_hash BLOB NOT NULL UNIQUE CHECK (length(token_hash) = 32),
    expires_at INTEGER NOT NULL,
    created_at INTEGER NOT NULL
);

CREATE TABLE invites (
    id INTEGER PRIMARY KEY,
    token_hash BLOB NOT NULL UNIQUE CHECK (length(token_hash) = 32),
    created_by INTEGER NOT NULL REFERENCES users(id),
    redeemed_by INTEGER REFERENCES users(id),
    expires_at INTEGER NOT NULL,
    created_at INTEGER NOT NULL,
    redeemed_at INTEGER
);

CREATE TABLE conversations (
    id INTEGER PRIMARY KEY,
    kind TEXT NOT NULL CHECK (kind IN ('channel', 'private', 'dm')),
    name TEXT,
    created_by INTEGER NOT NULL REFERENCES users(id),
    dm_user_low INTEGER REFERENCES users(id),
    dm_user_high INTEGER REFERENCES users(id),
    idempotency_key TEXT NOT NULL,
    created_at INTEGER NOT NULL,
    CHECK (
        (kind = 'dm' AND name IS NULL AND dm_user_low < dm_user_high) OR
        (kind != 'dm' AND name IS NOT NULL AND dm_user_low IS NULL AND dm_user_high IS NULL)
    ),
    UNIQUE (dm_user_low, dm_user_high),
    UNIQUE (created_by, idempotency_key)
);

CREATE TABLE conversation_members (
    conversation_id INTEGER NOT NULL REFERENCES conversations(id) ON DELETE CASCADE,
    user_id INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    joined_at INTEGER NOT NULL,
    PRIMARY KEY (conversation_id, user_id)
);

CREATE TABLE messages (
    id INTEGER PRIMARY KEY,
    conversation_id INTEGER NOT NULL REFERENCES conversations(id) ON DELETE CASCADE,
    author_id INTEGER NOT NULL REFERENCES users(id),
    reply_to_id INTEGER REFERENCES messages(id),
    body TEXT NOT NULL,
    rendered_body TEXT NOT NULL,
    idempotency_key TEXT NOT NULL,
    created_at INTEGER NOT NULL,
    edited_at INTEGER,
    deleted_at INTEGER,
    UNIQUE (author_id, idempotency_key)
);

CREATE INDEX messages_conversation_history
    ON messages(conversation_id, id DESC);

CREATE TABLE events (
    seq INTEGER PRIMARY KEY AUTOINCREMENT,
    conversation_id INTEGER REFERENCES conversations(id) ON DELETE CASCADE,
    actor_id INTEGER REFERENCES users(id),
    kind TEXT NOT NULL,
    entity_id INTEGER,
    payload TEXT NOT NULL,
    created_at INTEGER NOT NULL
);

CREATE INDEX events_conversation_sequence
    ON events(conversation_id, seq);
