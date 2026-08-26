import { useMemo } from "preact/hooks";

import { ApiClient, errorDetail } from "./api/client";
import { useSession } from "./auth/session";
import { NewConversationForm } from "./features/conversations/create";
import { ConversationDetail } from "./features/conversations/detail";
import { ConversationList, conversationLabel } from "./features/conversations/list";
import { defaultSocketFactory, type WorkspaceSocketFactory, useWorkspace } from "./features/chat/use-workspace";
import { Composer } from "./features/messages/composer";
import { Timeline } from "./features/messages/timeline";
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

	return <WorkspaceShell selectionKey={state.selectionGeneration}
		navigation={<>
			<header class="brand-block"><h1>Threadhall</h1><div><span>{user.username}</span><button type="button" onClick={() => void logout().catch((error) => state.setMutationError(errorDetail(error)))}>Sign out</button></div></header>
			<NewConversationForm onCreate={state.createConversation} />
			<ConversationList conversations={state.conversations} selectedId={state.selectedId} loading={state.conversationLoading} loadingMore={state.conversationLoadingMore} error={state.conversationError} hasMore={state.conversationCursor !== undefined} onLoadMore={() => void state.loadMoreConversations()} onSelect={(item) => state.select(item.id)} />
		</>}
		main={<div class="conversation-main">
			<header class="conversation-header"><h2>{selected ? title : "Select a conversation"}</h2><span class={`connection-status ${state.connection}`}><i />{connectionLabel(state.connection)}</span></header>
			{state.mutationError && <p class="global-error" role="alert">{state.mutationError}</p>}
			{selected ? <>
				<Timeline messages={state.timeline.messages} pending={state.pending} currentUserId={user.id} memberNames={memberNames} loading={state.messageLoading} error={state.messageError} hasOlder={state.messageCursor !== undefined} onLoadOlder={() => void state.loadOlderMessages()} onEdit={state.editMessage} onDelete={state.deleteMessage} />
				<Composer key={state.selectionGeneration} conversationName={title} onSend={state.sendMessage} />
			</> : <div class="empty-workspace"><p>Select a conversation from the navigation, or create the first one.</p></div>}
		</div>}
		context={<ConversationDetail conversation={state.detail} members={state.members} loading={state.messageLoading} loadingMoreMembers={state.memberLoadingMore} error={state.detailError} hasMoreMembers={state.memberCursor !== undefined} onLoadMoreMembers={() => void state.loadMoreMembers()} />}
	/>;
}
