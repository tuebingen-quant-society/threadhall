ALTER TABLE messages ADD COLUMN reference_message_id INTEGER REFERENCES messages(id) ON DELETE SET NULL;

CREATE INDEX messages_reference_target
    ON messages(reference_message_id)
    WHERE reference_message_id IS NOT NULL;
