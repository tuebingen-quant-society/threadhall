INSERT INTO agent_conversation_grants(agent_id, conversation_id, created_by, created_at)
SELECT agent.user_id, conversation.id, agent.created_by,
       max(agent.created_at, conversation.created_at)
FROM agents agent
CROSS JOIN conversations conversation
WHERE agent.revoked_at IS NULL AND conversation.kind = 'channel'
ON CONFLICT(agent_id, conversation_id) DO NOTHING;

INSERT INTO conversation_members(conversation_id, user_id, joined_at)
SELECT conversation.id, agent.user_id,
       max(agent.created_at, conversation.created_at)
FROM agents agent
CROSS JOIN conversations conversation
WHERE agent.revoked_at IS NULL AND conversation.kind = 'channel'
ON CONFLICT(conversation_id, user_id) DO NOTHING;
