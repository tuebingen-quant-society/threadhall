import { useEffect, useMemo, useState } from "preact/hooks";

import { ApiClient, errorDetail } from "./api/client";
import { useSession } from "./auth/session";
import { NewConversationForm } from "./features/conversations/create";
import { ConversationDetail } from "./features/conversations/detail";
import { ConversationList, conversationLabel } from "./features/conversations/list";
import { defaultSocketFactory, type WorkspaceSocketFactory, useWorkspace } from "./features/chat/use-workspace";
import { Composer } from "./features/messages/composer";
import { Timeline } from "./features/messages/timeline";
import { ThreadView } from "./features/threads/view";
import type { ThreadSummary } from "./api/types";
import { WorkspaceShell } from "./layout/workspace";

export type { WorkspaceSocketFactory } from "./features/chat/use-workspace";

function connectionLabel(status: ReturnType<typeof useWorkspace>["connection"]) {
	return { connecting: "Connecting", connected: "Live", reconnecting: "Reconnecting", resyncing: "Resyncing", error: "Sync error", offline: "Offline" }[status];
}

export function ChatWorkspace({ api, socketFactory = defaultSocketFactory }: { api: ApiClient; socketFactory?: WorkspaceSocketFactory }) {
	const { user, logout } = useSession();
	const state = useWorkspace(api, socketFactory);
	const selected = state.conversations.find((item) => item.id === state.selectedId);
	const title = selected ? conversationLabel(selected) : "Conversation";
	const memberNames = useMemo(() => new Map(state.members.map((member) => [member.user_id, member.username])), [state.members]);
	const [threadRoot, setThreadRoot] = useState<(typeof state.timeline.messages)[number]>();
	const [threads, setThreads] = useState<ThreadSummary[]>([]);
	useEffect(() => { setThreadRoot(undefined); setThreads([]); }, [state.selectedId]);
	useEffect(() => {
		if (selected === undefined) return;
		const controller = new AbortController();
		void api.threads(selected.id, controller.signal)
			.then((result) => { if (!controller.signal.aborted) setThreads(result.threads); })
			.catch((error) => { if (!controller.signal.aborted) state.setMutationError(errorDetail(error)); });
		return () => controller.abort();
	}, [api, selected?.id, state.threadRevision]);

	function openThread(root: (typeof state.timeline.messages)[number]) {
		setThreadRoot(root);
		setThreads((current) => current.some((item) => item.root.id === root.id)
			? current : [{ root, reply_count: 0 }, ...current]);
	}

	return <WorkspaceShell selectionKey={threadRoot ? `thread-${threadRoot.id}` : `conversation-${state.selectionGeneration}`}
		navigation={<>
			<header class="brand-block"><h1>Threadhall</h1><div><span>{user.username}</span><button type="button" onClick={() => void logout().catch((error) => state.setMutationError(errorDetail(error)))}>Sign out</button></div></header>
			<NewConversationForm onCreate={state.createConversation} onFindUsers={(query, signal) => api.findUsers(query, signal)} />
			<ConversationList conversations={state.conversations} selectedId={state.selectedId} selectedThreadId={threadRoot?.id} threads={threads}
				loading={state.conversationLoading} loadingMore={state.conversationLoadingMore} error={state.conversationError}
				hasMore={state.conversationCursor !== undefined} onLoadMore={() => void state.loadMoreConversations()}
				onSelect={(item) => { setThreadRoot(undefined); state.select(item.id); }}
				onSelectThread={(rootId) => { const summary = threads.find((item) => item.root.id === rootId); if (summary) setThreadRoot(summary.root); }} />
		</>}
		main={<div class="conversation-main">
			<header class="conversation-header"><div class="conversation-title">{threadRoot && selected && <button type="button" aria-label={`Back to ${title}`} onClick={() => setThreadRoot(undefined)}>←</button>}<h2>{threadRoot ? `Thread in ${title}` : selected ? title : "Select a conversation"}</h2></div><span class={`connection-status ${state.connection}`}><i />{connectionLabel(state.connection)}</span></header>
			{state.mutationError && <p class="global-error" role="alert">{state.mutationError}</p>}
			{selected ? threadRoot
				? <ThreadView api={api} conversationId={selected.id} root={threadRoot} memberNames={memberNames} revision={state.threadRevision} onFork={async (messageId, kind, name) => {
					await api.forkConversation(selected.id, messageId, kind, name, `fork-${crypto.randomUUID()}`);
				}} />
				: <><Timeline messages={state.timeline.messages} pending={state.pending} currentUserId={user.id} memberNames={memberNames} loading={state.messageLoading} error={state.messageError} hasOlder={state.messageCursor !== undefined} onLoadOlder={() => void state.loadOlderMessages()} onEdit={state.editMessage} onDelete={state.deleteMessage} onOpenThread={openThread} />
					<Composer key={state.selectionGeneration} conversationName={title} onSend={state.sendMessage} /></>
				: <div class="empty-workspace"><p>Select a conversation from the navigation, or create the first one.</p></div>}
		</div>}
		context={<ConversationDetail conversation={state.detail} members={state.members} loading={state.messageLoading} loadingMoreMembers={state.memberLoadingMore} error={state.detailError} hasMoreMembers={state.memberCursor !== undefined} onLoadMoreMembers={() => void state.loadMoreMembers()} />}
	/>;
}
