import type { Conversation, ThreadSummary } from "../../api/types";

export function conversationLabel(conversation: Conversation) {
	if (conversation.kind === "dm") return conversation.peer_username ?? `Direct message ${conversation.id}`;
	return conversation.name ?? `Conversation ${conversation.id}`;
}

function accessibleLabel(conversation: Conversation) {
	if (conversation.kind === "dm") return conversationLabel(conversation);
	return `${conversationLabel(conversation)}, ${conversation.kind === "private" ? "private" : "public"} channel`;
}

interface ConversationListProps {
	conversations: Conversation[];
	selectedId?: number;
	selectedThreadId?: number;
	threads?: ThreadSummary[];
	loading?: boolean;
	loadingMore?: boolean;
	error?: string;
	hasMore?: boolean;
	onLoadMore?: () => void;
	onSelect: (conversation: Conversation) => void;
	onSelectThread?: (rootMessageId: number) => void;
}

function threadPreview(thread: ThreadSummary) {
	if (thread.root.deleted_at) return "Deleted message";
	const value = thread.root.body.trim().replace(/\s+/g, " ");
	return value.length > 36 ? `${value.slice(0, 35)}…` : value;
}

export function ConversationList({ conversations, selectedId, selectedThreadId, threads = [], loading, loadingMore, error, hasMore, onLoadMore, onSelect, onSelectThread }: ConversationListProps) {
	return (
		<nav class="conversation-list" aria-label="Conversations">
			<div class="nav-heading"><span>Conversations</span><small>{conversations.length}</small></div>
			{loading && conversations.length === 0 && <p class="muted">Loading conversations…</p>}
			{error && <p class="inline-error" role="alert">{error}</p>}
			{!loading && !error && conversations.length === 0 && <p class="muted">No conversations yet.</p>}
			{conversations.map((conversation) => <div class="conversation-group" key={conversation.id}>
				<button
					class={conversation.id === selectedId ? "conversation-link active-parent" : "conversation-link"}
					type="button"
					aria-label={accessibleLabel(conversation)}
					aria-current={conversation.id === selectedId && selectedThreadId === undefined ? "page" : undefined}
					onClick={() => onSelect(conversation)}
				>
					<span aria-hidden="true">{conversation.kind === "dm" ? "↔" : conversation.kind === "private" ? "◇" : "#"}</span>
					<span>{conversationLabel(conversation)}</span>
				</button>
				{conversation.id === selectedId && threads.map((thread) => {
					const preview = threadPreview(thread);
					const replies = `${thread.reply_count} ${thread.reply_count === 1 ? "reply" : "replies"}`;
					return <button class={thread.root.id === selectedThreadId ? "thread-link selected" : "thread-link"}
						type="button" key={thread.root.id} aria-label={`Thread: ${preview}, ${replies}`}
						aria-current={thread.root.id === selectedThreadId ? "page" : undefined}
						onClick={() => onSelectThread?.(thread.root.id)}>
						<span aria-hidden="true">└</span><span>{preview}</span><small>{thread.reply_count}</small>
					</button>;
				})}
			</div>)}
			{hasMore && <button class="load-more-nav" type="button" disabled={loadingMore} onClick={onLoadMore}>{loadingMore ? "Loading…" : "Load more conversations"}</button>}
		</nav>
	);
}
