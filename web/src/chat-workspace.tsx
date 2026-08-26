import { useCallback, useEffect, useMemo, useState } from "preact/hooks";

import { ApiClient, errorDetail } from "./api/client";
import { useSession } from "./auth/session";
import { NewConversationForm } from "./features/conversations/create";
import { ConversationDetail } from "./features/conversations/detail";
import { ConversationList, conversationLabel } from "./features/conversations/list";
import { defaultSocketFactory, type WorkspaceSocketFactory, useWorkspace } from "./features/chat/use-workspace";
import { Composer } from "./features/messages/composer";
import { Timeline } from "./features/messages/timeline";
import { ThreadView } from "./features/threads/view";
import { ProfilePanel } from "./features/profile/profile";
import { NotificationPermissionControl } from "./features/notifications/permission";
import type { Capability, InlineApp, Message, Question, ThreadSummary } from "./api/types";
import { WorkspaceShell } from "./layout/workspace";
import { FilePreview } from "./features/messages/file-preview";

export type { WorkspaceSocketFactory } from "./features/chat/use-workspace";

function connectionLabel(status: ReturnType<typeof useWorkspace>["connection"]) {
	return { connecting: "Connecting", connected: "Live", reconnecting: "Reconnecting", resyncing: "Resyncing", error: "Sync error", offline: "Offline" }[status];
}

export function ChatWorkspace({ api, socketFactory = defaultSocketFactory }: { api: ApiClient; socketFactory?: WorkspaceSocketFactory }) {
	const { user, logout } = useSession();
	const state = useWorkspace(api, socketFactory, user.id);
	const selected = state.conversations.find((item) => item.id === state.selectedId);
	const title = selected ? conversationLabel(selected) : "Conversation";
	const memberNames = useMemo(() => new Map(state.members.map((member) => [member.user_id, member.username])), [state.members]);
	const [threadRoot, setThreadRoot] = useState<(typeof state.timeline.messages)[number]>();
	const [threadsByConversation, setThreadsByConversation] = useState<Map<number, ThreadSummary[]>>(new Map());
	const [capabilities, setCapabilities] = useState<Capability[]>([]);
	const [replyingTo, setReplyingTo] = useState<(typeof state.timeline.messages)[number]>();
	const [profileOpen, setProfileOpen] = useState(false);
	const [filePreview, setFilePreview] = useState<InlineApp>();
	const [filePreviewRequest, setFilePreviewRequest] = useState(0);
	const conversationIDs = useMemo(() => state.conversations.map((conversation) => conversation.id).join(","), [state.conversations]);
	useEffect(() => {
		if (threadRoot && threadRoot.conversation_id !== state.selectedId) setThreadRoot(undefined);
		setCapabilities([]); setReplyingTo(undefined); setFilePreview(undefined);
	}, [state.selectedId]);
	useEffect(() => {
		if (selected === undefined) return;
		const controller = new AbortController();
		void api.capabilities(selected.id, controller.signal)
			.then((result) => { if (!controller.signal.aborted) setCapabilities(result.capabilities); })
			.catch((error) => { if (!controller.signal.aborted) state.setMutationError(errorDetail(error)); });
		return () => controller.abort();
	}, [api, selected?.id]);
	useEffect(() => {
		const controller = new AbortController();
		const conversations = [...state.conversations];
		let cursor = 0;
		const loaded = new Map<number, ThreadSummary[]>();
		async function worker() {
			while (!controller.signal.aborted && cursor < conversations.length) {
				const conversation = conversations[cursor++];
				const result = await api.threads(conversation.id, controller.signal);
				loaded.set(conversation.id, result.threads);
			}
		}
		void Promise.all(Array.from({ length: Math.min(4, conversations.length) }, () => worker()))
			.then(() => { if (!controller.signal.aborted) setThreadsByConversation(loaded); })
			.catch((error) => { if (!controller.signal.aborted) state.setMutationError(errorDetail(error)); });
		return () => controller.abort();
	}, [api, conversationIDs, state.threadRevision]);

	function openThread(root: (typeof state.timeline.messages)[number]) {
		setReplyingTo(undefined);
		setThreadRoot(root);
		if (selected) setThreadsByConversation((current) => {
			const next = new Map(current);
			const threads = next.get(selected.id) ?? [];
			if (!threads.some((item) => item.root.id === root.id)) next.set(selected.id, [{ root, reply_count: 0, unread_count: 0 }, ...threads]);
			return next;
		});
	}
	const clearThreadUnread = useCallback(() => {
		if (!selected || !threadRoot) return;
		setThreadsByConversation((current) => {
			const next = new Map(current);
			next.set(selected.id, (next.get(selected.id) ?? []).map((thread) => thread.root.id === threadRoot.id ? { ...thread, unread_count: 0 } : thread));
			return next;
		});
	}, [selected?.id, threadRoot?.id]);
	const removeOpenThread = useCallback(() => {
		if (!selected || !threadRoot) return;
		setThreadsByConversation((current) => {
			const next = new Map(current); next.set(selected.id, (next.get(selected.id) ?? []).filter((thread) => thread.root.id !== threadRoot.id)); return next;
		});
		setThreadRoot(undefined);
	}, [selected?.id, threadRoot?.id]);

	function replyTo(message: (typeof state.timeline.messages)[number]) {
		setReplyingTo(message);
		requestAnimationFrame(() => document.getElementById("message-composer")?.focus());
	}

	async function answerQuestion(message: Message, question: Question, answer: string) {
		state.setMutationError("");
		try {
			await state.sendMessage(`@codex Answer to "${question.question}": ${answer}`, `answer-${crypto.randomUUID()}`, message.id);
		} catch (error) {
			state.setMutationError(errorDetail(error));
		}
	}
	function openFilePreview(app: InlineApp) {
		setFilePreview(app);
		setFilePreviewRequest((request) => request + 1);
	}

	return <WorkspaceShell selectionKey={threadRoot ? `thread-${threadRoot.id}` : `conversation-${state.selectionGeneration}`} contextRequestKey={filePreviewRequest || undefined} onContextClose={filePreview ? () => setFilePreview(undefined) : undefined}
		navigation={<>
			<header class="brand-block"><h1>Threadhall</h1><div class="brand-actions"><button class="profile-link" type="button" onClick={() => { setFilePreview(undefined); setProfileOpen(true); }}>{user.username}</button><NotificationPermissionControl /><button type="button" onClick={() => void logout().catch((error) => state.setMutationError(errorDetail(error)))}>Sign out</button></div></header>
			<NewConversationForm onCreate={state.createConversation} onFindUsers={(query, signal) => api.findUsers(query, signal)} />
			<ConversationList conversations={state.conversations} selectedId={state.selectedId} selectedThreadId={threadRoot?.id} threadsByConversation={threadsByConversation}
				loading={state.conversationLoading} loadingMore={state.conversationLoadingMore} error={state.conversationError}
				hasMore={state.conversationCursor !== undefined} onLoadMore={() => void state.loadMoreConversations()}
				onSelect={(item) => { setProfileOpen(false); setFilePreview(undefined); setThreadRoot(undefined); state.select(item.id); }}
				onSelectThread={(conversation, thread) => { setProfileOpen(false); setFilePreview(undefined); if (conversation.id !== state.selectedId) state.select(conversation.id); setThreadRoot(thread.root); }} />
		</>}
		main={<div class="conversation-main">
			<header class="conversation-header"><div class="conversation-title">{threadRoot && selected && <button type="button" aria-label={`Back to ${title}`} onClick={() => setThreadRoot(undefined)}>←</button>}<h2>{threadRoot ? `Thread in ${title}` : selected ? title : "Select a conversation"}</h2></div><span class={`connection-status ${state.connection}`}><i />{connectionLabel(state.connection)}</span></header>
			{state.mutationError && <p class="global-error" role="alert">{state.mutationError}</p>}
			{selected ? threadRoot
				? <ThreadView api={api} conversationId={selected.id} root={threadRoot} currentUserId={user.id} memberNames={memberNames} members={state.members} capabilities={capabilities} revision={state.threadRevision}
					canDelete={user.admin || selected.created_by === user.id || threadRoot.author_id === user.id} onDeleted={removeOpenThread} onRead={clearThreadUnread} onOpenAttachment={openFilePreview} onFork={async (messageId, kind, name) => {
					await api.forkConversation(selected.id, messageId, kind, name, `fork-${crypto.randomUUID()}`);
				}} />
				: <><Timeline messages={state.timeline.messages} pending={state.pending} currentUserId={user.id} memberNames={memberNames} loading={state.messageLoading} error={state.messageError} hasOlder={state.messageCursor !== undefined} onLoadOlder={() => void state.loadOlderMessages()} onEdit={state.editMessage} onDelete={state.deleteMessage} onOpenThread={openThread} onReply={replyTo} onQuestionAnswer={answerQuestion} onOpenAttachment={openFilePreview} />
					<Composer key={state.selectionGeneration} conversationName={title} onSend={state.sendMessage} mentionCandidates={state.members} capabilities={capabilities}
						replyTo={replyingTo} replyToAuthor={replyingTo ? memberNames.get(replyingTo.author_id) : undefined} onCancelReply={() => setReplyingTo(undefined)} /></>
				: <div class="empty-workspace"><p>Select a conversation from the navigation, or create the first one.</p></div>}
		</div>}
		context={filePreview ? <FilePreview app={filePreview} />
			: profileOpen ? <ProfilePanel api={api} user={user} onClose={() => setProfileOpen(false)} />
			: <ConversationDetail conversation={state.detail} members={state.members} loading={state.messageLoading} loadingMoreMembers={state.memberLoadingMore} error={state.detailError} hasMoreMembers={state.memberCursor !== undefined} onLoadMoreMembers={() => void state.loadMoreMembers()}
				canDelete={!!selected && selected.kind !== "dm" && (user.admin || selected.created_by === user.id)} onDelete={state.deleteConversation} />}
	/>;
}
