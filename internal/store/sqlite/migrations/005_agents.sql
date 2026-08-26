ALTER TABLE users ADD COLUMN principal_kind TEXT NOT NULL DEFAULT 'human'
    CHECK (principal_kind IN ('human', 'agent'));

ALTER TABLE conversations ADD COLUMN agent_policy TEXT NOT NULL DEFAULT 'explicit'
    CHECK (agent_policy IN ('explicit', 'human_only'));

CREATE TABLE agents (
    user_id INTEGER PRIMARY KEY REFERENCES users(id) ON DELETE CASCADE,
    token_hash BLOB NOT NULL UNIQUE CHECK (length(token_hash) = 32),
    created_by INTEGER NOT NULL REFERENCES users(id),
    created_at INTEGER NOT NULL,
    revoked_at INTEGER
);

CREATE TABLE agent_conversation_grants (
    agent_id INTEGER NOT NULL REFERENCES agents(user_id) ON DELETE CASCADE,
    conversation_id INTEGER NOT NULL REFERENCES conversations(id) ON DELETE CASCADE,
    created_by INTEGER NOT NULL REFERENCES users(id),
    created_at INTEGER NOT NULL,
    PRIMARY KEY (agent_id, conversation_id)
);

CREATE TABLE agent_tasks (
    id INTEGER PRIMARY KEY,
    agent_id INTEGER NOT NULL REFERENCES agents(user_id),
    conversation_id INTEGER NOT NULL REFERENCES conversations(id) ON DELETE CASCADE,
    owner_id INTEGER NOT NULL REFERENCES users(id),
    invoking_message_id INTEGER NOT NULL REFERENCES messages(id) ON DELETE CASCADE,
    thread_root_id INTEGER REFERENCES messages(id),
    state TEXT NOT NULL CHECK (state IN ('queued', 'running', 'completed', 'failed', 'denied')),
    runtime_thread_id TEXT,
    output_message_id INTEGER REFERENCES messages(id),
    public_error TEXT,
    created_at INTEGER NOT NULL,
    started_at INTEGER,
    completed_at INTEGER,
    UNIQUE (agent_id, invoking_message_id)
);

CREATE INDEX agent_tasks_queue ON agent_tasks(agent_id, state, id);
