CREATE TABLE message_apps (
    message_id INTEGER NOT NULL REFERENCES messages(id) ON DELETE CASCADE,
    ordinal INTEGER NOT NULL CHECK (ordinal >= 0 AND ordinal < 4),
    server TEXT NOT NULL,
    tool TEXT NOT NULL,
    resource_uri TEXT NOT NULL,
    html TEXT NOT NULL,
    arguments_json BLOB NOT NULL,
    result_json BLOB NOT NULL,
    PRIMARY KEY (message_id, ordinal)
) STRICT;
