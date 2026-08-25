import type { Conversation } from "../../api/types";

export function conversationLabel(conversation: Conversation) {
	if (conversation.kind === "dm") return `Direct message ${conversation.id}`;
	return conversation.name ?? `Conversation ${conversation.id}`;
}

function accessibleLabel(conversation: Conversation) {
	if (conversation.kind === "dm") return conversationLabel(conversation);
	return `${conversationLabel(conversation)}, ${conversation.kind === "private" ? "private" : "public"} channel`;
}

interface ConversationListProps {
	conversations: Conversation[];
	selectedId?: number;
	loading?: boolean;
	error?: string;
	onSelect: (conversation: Conversation) => void;
}

export function ConversationList({ conversations, selectedId, loading, error, onSelect }: ConversationListProps) {
	return (
		<nav class="conversation-list" aria-label="Conversations">
			<div class="nav-heading"><span>Conversations</span><small>{conversations.length}</small></div>
			{loading && conversations.length === 0 && <p class="muted">Loading conversations…</p>}
			{error && <p class="inline-error" role="alert">{error}</p>}
			{!loading && !error && conversations.length === 0 && <p class="muted">No conversations yet.</p>}
			{conversations.map((conversation) => (
				<button
					class={conversation.id === selectedId ? "conversation-link selected" : "conversation-link"}
					type="button"
					key={conversation.id}
					aria-label={accessibleLabel(conversation)}
					aria-current={conversation.id === selectedId ? "page" : undefined}
					onClick={() => onSelect(conversation)}
				>
					<span aria-hidden="true">{conversation.kind === "dm" ? "↔" : conversation.kind === "private" ? "◇" : "#"}</span>
					<span>{conversationLabel(conversation)}</span>
				</button>
			))}
		</nav>
	);
}
