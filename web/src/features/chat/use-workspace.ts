import { useCallback, useEffect, useRef, useState } from "preact/hooks";

import { ApiClient, ApiProblem, errorDetail } from "../../api/client";
import type { ConnectionState, Conversation, Member, Message } from "../../api/types";
import type { ConversationDraft } from "../conversations/create";
import { appendConversationPage, appendMemberPage } from "../conversations/collection";
import { applyRealtimeEvent, mergeHistoryPage, mergeMessageResult, type PendingMessage, queuePending, reconcilePending, type TimelineState } from "../messages/timeline";
import { RealtimeSocket, type SocketCallbacks } from "../../realtime/socket";
import { createInvalidationCoalescer } from "./invalidation";
import { loadConversationPages, loadMemberPages } from "./load-pages";

export interface WorkspaceSocket { start(): void; stop(): void }
export type WorkspaceSocketFactory = (callbacks: SocketCallbacks) => WorkspaceSocket;
export const defaultSocketFactory: WorkspaceSocketFactory = (callbacks) => new RealtimeSocket(callbacks);

type Scope = { id: number | undefined; generation: number };
const emptyTimeline = (): TimelineState => ({ messages: [], entitySeq: new Map() });
const mutationKey = (operation: string) => `${operation}-${crypto.randomUUID()}`;

export function useWorkspace(api: ApiClient, socketFactory: WorkspaceSocketFactory) {
	const [conversations, setConversations] = useState<Conversation[]>([]);
	const [conversationCursor, setConversationCursor] = useState<number>();
	const [selectedId, setSelectedId] = useState<number>();
	const [selectionGeneration, setSelectionGeneration] = useState(0);
	const [detail, setDetail] = useState<Conversation | null>(null);
	const [members, setMembers] = useState<Member[]>([]);
	const [memberCursor, setMemberCursor] = useState<number>();
	const [timeline, setTimeline] = useState<TimelineState>(emptyTimeline);
	const [messageCursor, setMessageCursor] = useState<number>();
	const [pending, setPending] = useState<PendingMessage[]>([]);
	const [conversationLoading, setConversationLoading] = useState(true);
	const [conversationLoadingMore, setConversationLoadingMore] = useState(false);
	const [messageLoading, setMessageLoading] = useState(false);
	const [memberLoadingMore, setMemberLoadingMore] = useState(false);
	const [conversationError, setConversationError] = useState("");
	const [detailError, setDetailError] = useState("");
	const [messageError, setMessageError] = useState("");
	const [mutationError, setMutationError] = useState("");
	const [connection, setConnection] = useState<ConnectionState>("connecting");

	const conversationsRef = useRef<Conversation[]>([]);
	const membersRef = useRef<Member[]>([]);
	const scopeRef = useRef<Scope>({ id: undefined, generation: 0 });
	const selectionController = useRef<AbortController | null>(null);
	const actionControllers = useRef(new Set<AbortController>());
	const listRequest = useRef({ generation: 0, controller: null as AbortController | null });

	const current = useCallback((scope: Scope, controller?: AbortController) =>
		scopeRef.current.id === scope.id && scopeRef.current.generation === scope.generation && !controller?.signal.aborted, []);

	const select = useCallback((id: number | undefined) => {
		selectionController.current?.abort();
		for (const controller of actionControllers.current) controller.abort();
		actionControllers.current.clear();
		const generation = scopeRef.current.generation + 1;
		scopeRef.current = { id, generation };
		setSelectedId(id); setSelectionGeneration(generation);
		setDetail(null); setMembers([]); membersRef.current = [];
		setTimeline(emptyTimeline()); setPending([]);
		setMemberCursor(undefined); setMessageCursor(undefined);
		setDetailError(""); setMessageError(""); setMutationError("");
		setMemberLoadingMore(false); setMessageLoading(id !== undefined);
	}, []);

	function beginListRequest() {
		listRequest.current.controller?.abort();
		const controller = new AbortController();
		const generation = listRequest.current.generation + 1;
		listRequest.current = { generation, controller };
		return { generation, controller };
	}

	function listCurrent(generation: number, controller: AbortController) {
		return listRequest.current.generation === generation && listRequest.current.controller === controller && !controller.signal.aborted;
	}

	function replaceConversations(items: Conversation[], cursor: number | undefined) {
		conversationsRef.current = items; setConversations(items); setConversationCursor(cursor);
	}

	useEffect(() => {
		const request = beginListRequest();
		setConversationLoading(true); setConversationError("");
		loadConversationPages(api, 100, request.controller.signal).then((result) => {
			if (!listCurrent(request.generation, request.controller)) return;
			replaceConversations(result.items, result.nextBeforeId);
			if (scopeRef.current.id === undefined) select(result.items[0]?.id);
		}).catch((error) => {
			if (listCurrent(request.generation, request.controller)) setConversationError(errorDetail(error));
		}).finally(() => {
			if (listCurrent(request.generation, request.controller)) setConversationLoading(false);
		});
		return () => request.controller.abort();
	}, [api, select]);

	useEffect(() => {
		if (selectedId === undefined) { setMessageLoading(false); return; }
		const scope = { ...scopeRef.current };
		const controller = new AbortController();
		selectionController.current = controller;
		setMessageLoading(true);
		Promise.all([api.conversation(selectedId, controller.signal), api.members(selectedId, controller.signal), api.history(selectedId, controller.signal)])
			.then(([item, memberPage, messagePage]) => {
				if (!current(scope, controller)) return;
				const memberResult = appendMemberPage([], memberPage);
				setDetail(item); membersRef.current = memberResult.items; setMembers(memberResult.items);
				setMemberCursor(memberResult.nextBeforeId);
				setTimeline(mergeHistoryPage(emptyTimeline(), messagePage.messages)); setMessageCursor(messagePage.next_before_id);
			})
			.catch((error) => {
				if (!current(scope, controller)) return;
				const detail = errorDetail(error); setDetailError(detail); setMessageError(detail);
			})
			.finally(() => { if (current(scope, controller)) setMessageLoading(false); });
		return () => controller.abort();
	}, [api, current, selectedId, selectionGeneration]);

	function captureAction() {
		const scope = { ...scopeRef.current };
		if (scope.id === undefined) return null;
		const controller = new AbortController(); actionControllers.current.add(controller);
		return { scope: scope as Scope & { id: number }, controller };
	}

	function finishAction(controller: AbortController) { actionControllers.current.delete(controller); }

	async function loadMoreConversations() {
		if (conversationCursor === undefined) return;
		const request = beginListRequest(); setConversationLoadingMore(true);
		try {
			const page = await api.listConversations(request.controller.signal, conversationCursor);
			if (!listCurrent(request.generation, request.controller)) return;
			const result = appendConversationPage(conversationsRef.current, page);
			replaceConversations(result.items, result.nextBeforeId);
		} catch (error) {
			if (listCurrent(request.generation, request.controller)) setConversationError(errorDetail(error));
		} finally { if (listCurrent(request.generation, request.controller)) setConversationLoadingMore(false); }
	}

	async function loadMoreMembers() {
		const action = captureAction(); if (action === null || memberCursor === undefined) return;
		setMemberLoadingMore(true);
		try {
			const page = await api.members(action.scope.id, action.controller.signal, memberCursor);
			if (!current(action.scope, action.controller)) return;
			const result = appendMemberPage(membersRef.current, page);
			membersRef.current = result.items; setMembers(result.items); setMemberCursor(result.nextBeforeId);
		} catch (error) {
			if (current(action.scope, action.controller)) setDetailError(errorDetail(error));
		} finally {
			if (current(action.scope, action.controller)) setMemberLoadingMore(false); finishAction(action.controller);
		}
	}

	async function loadOlderMessages() {
		const action = captureAction(); if (action === null || messageCursor === undefined) return;
		setMessageLoading(true);
		try {
			const page = await api.history(action.scope.id, action.controller.signal, messageCursor);
			if (!current(action.scope, action.controller)) return;
			setTimeline((state) => mergeHistoryPage(state, page.messages)); setMessageCursor(page.next_before_id);
		} catch (error) {
			if (current(action.scope, action.controller)) setMessageError(errorDetail(error));
		} finally {
			if (current(action.scope, action.controller)) setMessageLoading(false); finishAction(action.controller);
		}
	}

	async function sendMessage(body: string) {
		const action = captureAction(); if (action === null) return;
		const key = mutationKey("send"); setPending((items) => queuePending(items, key, body));
		try {
			const result = await api.sendMessage(action.scope.id, body, key, action.controller.signal);
			if (current(action.scope, action.controller)) setTimeline((state) => mergeMessageResult(state, result));
		} catch (error) {
			if (current(action.scope, action.controller)) throw error;
		} finally {
			if (current(action.scope, action.controller)) setPending((items) => reconcilePending(items, key));
			finishAction(action.controller);
		}
	}

	async function changeMessage(operation: "edit" | "delete", message: Message, body?: string) {
		const action = captureAction(); if (action === null) return;
		setMutationError("");
		try {
			const result = operation === "edit"
				? await api.editMessage(message.id, body!, mutationKey("edit"), action.controller.signal)
				: await api.deleteMessage(message.id, mutationKey("delete"), action.controller.signal);
			if (current(action.scope, action.controller)) setTimeline((state) => mergeMessageResult(state, result));
		} catch (error) {
			if (current(action.scope, action.controller)) setMutationError(errorDetail(error));
		} finally { finishAction(action.controller); }
	}

	async function createConversation(draft: ConversationDraft) {
		setMutationError("");
		const created = draft.kind === "dm" ? await api.createDM(draft.otherUserId, mutationKey("dm"))
			: await api.createChannel(draft.kind, draft.name, mutationKey("channel"));
		const request = beginListRequest();
		const result = await loadConversationPages(api, Math.max(100, conversationsRef.current.length), request.controller.signal);
		if (listCurrent(request.generation, request.controller)) { replaceConversations(result.items, result.nextBeforeId); select(created.id); }
	}

	const refreshSelected = useCallback(async (scope: Scope & { id: number }, controller: AbortController, history: boolean) => {
		const requests = [api.conversation(scope.id, controller.signal), loadMemberPages(api, scope.id, Math.max(100, membersRef.current.length), controller.signal)] as const;
		const [item, memberResult] = await Promise.all(requests);
		const messagePage = history ? await api.history(scope.id, controller.signal) : null;
		if (!current(scope, controller)) return;
		setDetail(item); membersRef.current = memberResult.items; setMembers(memberResult.items); setMemberCursor(memberResult.nextBeforeId);
		if (messagePage !== null) { setTimeline(mergeHistoryPage(emptyTimeline(), messagePage.messages)); setMessageCursor(messagePage.next_before_id); setPending([]); }
		setDetailError(""); setMessageError("");
	}, [api, current]);

	const refreshAuthoritative = useCallback(async (history: boolean) => {
		const request = beginListRequest();
		try {
			const result = await loadConversationPages(api, Math.max(100, conversationsRef.current.length), request.controller.signal);
			if (!listCurrent(request.generation, request.controller)) throw new Error("refresh superseded");
			replaceConversations(result.items, result.nextBeforeId);
			const scope = { ...scopeRef.current };
			if (scope.id === undefined) return;
			if (!result.items.some((item) => item.id === scope.id)) { select(undefined); return; }
			await refreshSelected(scope as Scope & { id: number }, request.controller, history);
		} catch (error) {
			if (request.controller.signal.aborted) throw error;
			if (error instanceof ApiProblem && error.code === "not_found" && scopeRef.current.id !== undefined) { select(undefined); return; }
			const detail = history ? `Resync failed: ${errorDetail(error)}` : errorDetail(error);
			setConversationError(detail); setDetailError(detail); if (history) setMessageError(detail);
			throw error;
		}
	}, [api, refreshSelected, select]);

	useEffect(() => {
		const coalescer = createInvalidationCoalescer(() => refreshAuthoritative(false));
		const socket = socketFactory({
			onStatus: setConnection,
			onEvent: (event) => {
				if (event.type.startsWith("conversation.")) coalescer.request();
				if (event.type.startsWith("message.") && event.conversation_id === scopeRef.current.id) setTimeline((state) => applyRealtimeEvent(state, event));
			},
			onResync: async () => {
				for (const controller of actionControllers.current) controller.abort();
				actionControllers.current.clear(); setPending([]);
				await refreshAuthoritative(true);
			},
		});
		socket.start();
		return () => { coalescer.stop(); socket.stop(); };
	}, [refreshAuthoritative, socketFactory]);

	useEffect(() => () => {
		selectionController.current?.abort(); listRequest.current.controller?.abort();
		for (const controller of actionControllers.current) controller.abort();
	}, []);

	return {
		conversations, conversationCursor, selectedId, selectionGeneration, detail, members, memberCursor,
		timeline, messageCursor, pending, conversationLoading, conversationLoadingMore, messageLoading,
		memberLoadingMore, conversationError, detailError, messageError, mutationError, connection,
		select, loadMoreConversations, loadMoreMembers, loadOlderMessages, sendMessage,
		editMessage: (message: Message, body: string) => changeMessage("edit", message, body),
		deleteMessage: (message: Message) => changeMessage("delete", message), createConversation, setMutationError,
	};
}
