CREATE UNIQUE INDEX conversations_channel_name_nocase
    ON conversations(name COLLATE NOCASE)
    WHERE kind IN ('channel', 'private');

CREATE TABLE conversation_mutations (
    actor_id INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    idempotency_key TEXT NOT NULL,
    operation TEXT NOT NULL CHECK (
        operation IN ('create_channel', 'create_dm', 'add_member', 'remove_member')
    ),
    fingerprint TEXT NOT NULL,
    conversation_id INTEGER NOT NULL REFERENCES conversations(id) ON DELETE CASCADE,
    created_at INTEGER NOT NULL,
    PRIMARY KEY (actor_id, idempotency_key)
);
