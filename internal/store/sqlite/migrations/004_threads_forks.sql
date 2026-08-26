ALTER TABLE messages ADD COLUMN thread_root_id INTEGER REFERENCES messages(id);

CREATE INDEX messages_thread_history
    ON messages(thread_root_id, id ASC)
    WHERE thread_root_id IS NOT NULL;

CREATE TABLE conversation_forks (
    conversation_id INTEGER PRIMARY KEY REFERENCES conversations(id) ON DELETE CASCADE,
    source_conversation_id INTEGER NOT NULL REFERENCES conversations(id) ON DELETE RESTRICT,
    source_root_message_id INTEGER NOT NULL REFERENCES messages(id) ON DELETE RESTRICT,
    created_by INTEGER NOT NULL REFERENCES users(id),
    created_at INTEGER NOT NULL
);

CREATE TABLE conversation_fork_mutations (
    actor_id INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    idempotency_key TEXT NOT NULL,
    fingerprint TEXT NOT NULL,
    conversation_id INTEGER NOT NULL REFERENCES conversations(id) ON DELETE CASCADE,
    created_at INTEGER NOT NULL,
    PRIMARY KEY (actor_id, idempotency_key)
);
