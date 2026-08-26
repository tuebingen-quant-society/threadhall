import { useState } from "preact/hooks";

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
	threadsByConversation?: Map<number, ThreadSummary[]>;
	loading?: boolean;
	loadingMore?: boolean;
	error?: string;
	hasMore?: boolean;
	onLoadMore?: () => void;
	onSelect: (conversation: Conversation) => void;
	onSelectThread?: (conversation: Conversation, thread: ThreadSummary) => void;
}

function threadPreview(thread: ThreadSummary) {
	if (thread.root.deleted_at) return "Deleted message";
	const value = thread.root.body.trim().replace(/\s+/g, " ");
	return value.length > 36 ? `${value.slice(0, 35)}…` : value;
}

export function ConversationList({ conversations, selectedId, selectedThreadId, threadsByConversation = new Map(), loading, loadingMore, error, hasMore, onLoadMore, onSelect, onSelectThread }: ConversationListProps) {
	const [collapsed, setCollapsed] = useState<Set<number>>(new Set());
	function toggle(conversationId: number) {
		setCollapsed((current) => {
			const next = new Set(current);
			if (next.has(conversationId)) next.delete(conversationId); else next.add(conversationId);
			return next;
		});
	}
	return (
		<nav class="conversation-list" aria-label="Conversations">
			<div class="nav-heading"><span>Conversations</span><small>{conversations.length}</small></div>
			{loading && conversations.length === 0 && <p class="muted">Loading conversations…</p>}
			{error && <p class="inline-error" role="alert">{error}</p>}
			{!loading && !error && conversations.length === 0 && <p class="muted">No conversations yet.</p>}
			{conversations.map((conversation) => {
				const threads = threadsByConversation.get(conversation.id) ?? [];
				const hidden = collapsed.has(conversation.id);
				return <div class="conversation-group" key={conversation.id}>
					<div class="conversation-row">
					<button
					class={`conversation-link${conversation.id === selectedId ? " active-parent" : ""}${conversation.id === selectedId && selectedThreadId === undefined ? " current" : ""}`}
					type="button"
					aria-label={accessibleLabel(conversation)}
					aria-current={conversation.id === selectedId && selectedThreadId === undefined ? "page" : undefined}
					onClick={() => onSelect(conversation)}
					>
						<span aria-hidden="true">{conversation.kind === "dm" ? "↔" : conversation.kind === "private" ? "◇" : "#"}</span>
						<span>{conversationLabel(conversation)}</span>
						{(conversation.unread_count ?? 0) > 0 && <small class="unread-count" aria-label={`${conversation.unread_count} unread messages`}>{conversation.unread_count}</small>}
					</button>
					{threads.length > 0 && <button class="thread-toggle" type="button" aria-label={`${hidden ? "Expand" : "Collapse"} threads for ${conversationLabel(conversation)}`} aria-expanded={!hidden} onClick={() => toggle(conversation.id)}>{hidden ? "›" : "⌄"}</button>}
					</div>
					{!hidden && threads.map((thread) => {
					const preview = threadPreview(thread);
					const replies = `${thread.reply_count} ${thread.reply_count === 1 ? "reply" : "replies"}`;
					return <button class={thread.root.id === selectedThreadId ? "thread-link selected" : "thread-link"}
						type="button" key={thread.root.id} aria-label={`Thread: ${preview}, ${replies}`}
						aria-current={thread.root.id === selectedThreadId ? "page" : undefined}
						onClick={() => onSelectThread?.(conversation, thread)}>
						<span aria-hidden="true">└</span><span>{preview}</span>{(thread.unread_count ?? 0) > 0
							? <small class="unread-count" aria-label={`${thread.unread_count} unread replies`}>{thread.unread_count}</small>
							: <small>{thread.reply_count}</small>}
					</button>;
					})}
				</div>;
			})}
			{hasMore && <button class="load-more-nav" type="button" disabled={loadingMore} onClick={onLoadMore}>{loadingMore ? "Loading…" : "Load more conversations"}</button>}
		</nav>
	);
}
