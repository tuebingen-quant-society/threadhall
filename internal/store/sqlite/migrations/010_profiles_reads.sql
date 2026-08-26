ALTER TABLE users ADD COLUMN avatar_mime TEXT;
ALTER TABLE users ADD COLUMN avatar_data BLOB;
ALTER TABLE users ADD COLUMN avatar_updated_at INTEGER;

CREATE TABLE conversation_reads (
    user_id INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    conversation_id INTEGER NOT NULL REFERENCES conversations(id) ON DELETE CASCADE,
    last_read_message_id INTEGER NOT NULL DEFAULT 0,
    updated_at INTEGER NOT NULL,
    PRIMARY KEY (user_id, conversation_id)
);

CREATE TABLE thread_reads (
    user_id INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    root_message_id INTEGER NOT NULL REFERENCES messages(id) ON DELETE CASCADE,
    last_read_message_id INTEGER NOT NULL DEFAULT 0,
    updated_at INTEGER NOT NULL,
    PRIMARY KEY (user_id, root_message_id)
);
