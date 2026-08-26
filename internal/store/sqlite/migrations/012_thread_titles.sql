CREATE TABLE thread_titles (
    root_message_id INTEGER PRIMARY KEY REFERENCES messages(id) ON DELETE CASCADE,
    conversation_id INTEGER NOT NULL REFERENCES conversations(id) ON DELETE CASCADE,
    title TEXT NOT NULL CHECK (length(title) BETWEEN 1 AND 80),
    updated_at INTEGER NOT NULL
) STRICT;

CREATE TABLE thread_title_mutations (
    actor_id INTEGER NOT NULL REFERENCES users(id),
    idempotency_key TEXT NOT NULL,
    fingerprint TEXT NOT NULL,
    root_message_id INTEGER NOT NULL REFERENCES messages(id) ON DELETE CASCADE,
    created_at INTEGER NOT NULL,
    PRIMARY KEY (actor_id, idempotency_key)
) STRICT;

CREATE TABLE conversation_rename_mutations (
    actor_id INTEGER NOT NULL REFERENCES users(id),
    idempotency_key TEXT NOT NULL,
    fingerprint TEXT NOT NULL,
    conversation_id INTEGER NOT NULL REFERENCES conversations(id) ON DELETE CASCADE,
    created_at INTEGER NOT NULL,
    PRIMARY KEY (actor_id, idempotency_key)
) STRICT;
