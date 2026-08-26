CREATE TABLE agent_capabilities (
    agent_id INTEGER NOT NULL REFERENCES agents(user_id) ON DELETE CASCADE,
    kind TEXT NOT NULL CHECK (kind IN ('plugin', 'skill')),
    capability_id TEXT NOT NULL CHECK (length(capability_id) BETWEEN 1 AND 256),
    name TEXT NOT NULL CHECK (length(name) BETWEEN 1 AND 128),
    description TEXT NOT NULL CHECK (length(description) <= 1024),
    updated_at INTEGER NOT NULL,
    PRIMARY KEY (agent_id, kind, capability_id)
);

CREATE INDEX agent_capabilities_agent_kind ON agent_capabilities(agent_id, kind, name);
