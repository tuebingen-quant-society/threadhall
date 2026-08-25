import { useCallback, useEffect, useMemo, useRef, useState } from "preact/hooks";

import { ApiClient, errorDetail } from "./api/client";
import type { ConnectionState, Conversation, Member, Message } from "./api/types";
import { useSession } from "./auth/session";
import { type ConversationDraft, NewConversationForm } from "./features/conversations/create";
import { ConversationDetail } from "./features/conversations/detail";
import { ConversationList, conversationLabel } from "./features/conversations/list";
import { Composer } from "./features/messages/composer";
import { applyRealtimeEvent, MAX_CLIENT_MESSAGES, mergeMessages, type PendingMessage, queuePending, reconcilePending, Timeline } from "./features/messages/timeline";
import { WorkspaceShell } from "./layout/workspace";
import { RealtimeSocket } from "./realtime/socket";

function mutationKey(operation: string) {
	return `${operation}-${crypto.randomUUID()}`;
}

function connectionLabel(status: ConnectionState) {
	return { connecting: "Connecting", connected: "Live", reconnecting: "Reconnecting", resyncing: "Resyncing", offline: "Offline" }[status];
}

export function ChatWorkspace({ api }: { api: ApiClient }) {
	const { user, logout } = useSession();
	const [conversations, setConversations] = useState<Conversation[]>([]);
	const [selectedId, setSelectedId] = useState<number>();
	const [detail, setDetail] = useState<Conversation | null>(null);
	const [members, setMembers] = useState<Member[]>([]);
	const [timeline, setTimeline] = useState({ messages: [] as Message[], lastSeq: 0 });
	const [pending, setPending] = useState<PendingMessage[]>([]);
	const [nextBeforeId, setNextBeforeId] = useState<number>();
	const [conversationLoading, setConversationLoading] = useState(true);
	const [messageLoading, setMessageLoading] = useState(false);
	const [conversationError, setConversationError] = useState("");
	const [detailError, setDetailError] = useState("");
	const [messageError, setMessageError] = useState("");
	const [mutationError, setMutationError] = useState("");
	const [connection, setConnection] = useState<ConnectionState>("connecting");
	const selectedRef = useRef(selectedId);
	selectedRef.current = selectedId;

	const loadConversationList = useCallback(async (signal?: AbortSignal) => {
		const page = await api.listConversations(signal);
		setConversations(page.conversations.slice(0, 100));
		setSelectedId((current) => current ?? page.conversations[0]?.id);
		return page.conversations;
	}, [api]);

	useEffect(() => {
		const controller = new AbortController();
		setConversationLoading(true);
		setConversationError("");
		loadConversationList(controller.signal).catch((error) => {
			if (!controller.signal.aborted) setConversationError(errorDetail(error));
		}).finally(() => { if (!controller.signal.aborted) setConversationLoading(false); });
		return () => controller.abort();
	}, [loadConversationList]);

	useEffect(() => {
		setMutationError("");
		setDetailError("");
		setMessageError("");
		setPending([]);
		if (selectedId === undefined) {
			setDetail(null); setMembers([]); setTimeline({ messages: [], lastSeq: 0 });
			return;
		}
		const controller = new AbortController();
		setMessageLoading(true);
		Promise.all([api.conversation(selectedId, controller.signal), api.members(selectedId, controller.signal), api.history(selectedId, controller.signal)])
			.then(([item, memberPage, messagePage]) => {
				setDetail(item);
				setMembers(memberPage.members.slice(0, 100));
				setTimeline((current) => ({ messages: mergeMessages([], messagePage.messages).slice(-MAX_CLIENT_MESSAGES), lastSeq: current.lastSeq }));
				setNextBeforeId(messagePage.next_before_id);
			})
			.catch((error) => { if (!controller.signal.aborted) setDetailError(errorDetail(error)); })
			.finally(() => { if (!controller.signal.aborted) setMessageLoading(false); });
		return () => controller.abort();
	}, [api, selectedId]);

	useEffect(() => {
		const controller = new AbortController();
		const socket = new RealtimeSocket({
			onStatus: setConnection,
			onEvent: (event) => {
				if (event.type.startsWith("conversation.")) {
					loadConversationList(controller.signal).catch((error) => { if (!controller.signal.aborted) setConversationError(errorDetail(error)); });
				}
				if (event.conversation_id === selectedRef.current && event.type.startsWith("message.")) {
					setTimeline((current) => applyRealtimeEvent(current, event));
				}
			},
			onResync: async () => {
				try {
					const list = await loadConversationList(controller.signal);
					const id = selectedRef.current;
					if (id === undefined || !list.some((item) => item.id === id)) return;
					const [item, memberPage, messagePage] = await Promise.all([api.conversation(id, controller.signal), api.members(id, controller.signal), api.history(id, controller.signal)]);
					setDetail(item); setMembers(memberPage.members.slice(0, 100));
					setTimeline({ messages: mergeMessages([], messagePage.messages), lastSeq: 0 });
					setNextBeforeId(messagePage.next_before_id);
					setMessageError("");
				} catch (error) {
					if (!controller.signal.aborted) setMessageError(`Resync failed: ${errorDetail(error)}`);
				}
			},
		});
		socket.start();
		return () => { controller.abort(); socket.stop(); };
	}, [api, loadConversationList]);

	async function createConversation(draft: ConversationDraft) {
		setMutationError("");
		const created = draft.kind === "dm"
			? await api.createDM(draft.otherUserId, mutationKey("dm"))
			: await api.createChannel(draft.kind, draft.name, mutationKey("channel"));
		await loadConversationList();
		setSelectedId(created.id);
	}

	async function sendMessage(body: string) {
		if (selectedId === undefined) return;
		const key = mutationKey("send");
		setPending((current) => queuePending(current, key, body));
		try {
			const result = await api.sendMessage(selectedId, body, key);
			setTimeline((current) => ({ messages: mergeMessages(current.messages, [result.message]), lastSeq: Math.max(current.lastSeq, result.event.seq) }));
		} finally {
			setPending((current) => reconcilePending(current, key));
		}
	}

	async function editMessage(message: Message, body: string) {
		setMutationError("");
		try {
			const result = await api.editMessage(message.id, body, mutationKey("edit"));
			setTimeline((current) => ({ messages: mergeMessages(current.messages, [result.message]), lastSeq: Math.max(current.lastSeq, result.event.seq) }));
		} catch (error) { setMutationError(errorDetail(error)); }
	}

	async function deleteMessage(message: Message) {
		setMutationError("");
		try {
			const result = await api.deleteMessage(message.id, mutationKey("delete"));
			setTimeline((current) => ({ messages: mergeMessages(current.messages, [result.message]), lastSeq: Math.max(current.lastSeq, result.event.seq) }));
		} catch (error) { setMutationError(errorDetail(error)); }
	}

	async function loadOlder() {
		if (selectedId === undefined || nextBeforeId === undefined) return;
		setMessageLoading(true);
		try {
			const page = await api.history(selectedId, undefined, nextBeforeId);
			const reachesCap = mergeMessages(page.messages, timeline.messages).length >= MAX_CLIENT_MESSAGES;
			setTimeline((current) => ({ ...current, messages: mergeMessages(page.messages, current.messages) }));
			setNextBeforeId(reachesCap ? undefined : page.next_before_id);
		} catch (error) { setMessageError(errorDetail(error)); }
		finally { setMessageLoading(false); }
	}

	const memberNames = useMemo(() => new Map(members.map((member) => [member.user_id, member.username])), [members]);
	const selected = conversations.find((item) => item.id === selectedId);
	const title = selected ? conversationLabel(selected) : "Conversation";

	return <WorkspaceShell selectionKey={selectedId}
		navigation={<><header class="brand-block"><p class="eyebrow">THREADHALL</p><h1>Workshop</h1><div><span>{user.username}</span><button type="button" onClick={() => void logout().catch((error) => setMutationError(errorDetail(error)))}>Sign out</button></div></header><NewConversationForm onCreate={createConversation} /><ConversationList conversations={conversations} selectedId={selectedId} loading={conversationLoading} error={conversationError} onSelect={(item) => setSelectedId(item.id)} /></>}
		main={<div class="conversation-main"><header class="conversation-header"><div><p class="section-kicker">{selected?.kind === "dm" ? "Direct message" : "Channel"}</p><h2>{selected ? title : "Select a conversation"}</h2></div><span class={`connection-status ${connection}`}><i />{connectionLabel(connection)}</span></header>{mutationError && <p class="global-error" role="alert">{mutationError}</p>}{selected ? <><Timeline messages={timeline.messages} pending={pending} currentUserId={user.id} memberNames={memberNames} loading={messageLoading} error={messageError} hasOlder={nextBeforeId !== undefined} onLoadOlder={() => void loadOlder()} onEdit={editMessage} onDelete={deleteMessage} /><Composer conversationName={title} onSend={sendMessage} /></> : <div class="empty-workspace"><p>Select a conversation from the navigation, or create the first one.</p></div>}</div>}
		context={<ConversationDetail conversation={detail} members={members} loading={messageLoading} error={detailError} />}
	/>;
}
