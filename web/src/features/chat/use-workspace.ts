import { useCallback, useEffect, useRef, useState } from "preact/hooks";

import { ApiClient, ApiProblem, errorDetail } from "../../api/client";
import type { ConnectionState, Conversation, Member, Message } from "../../api/types";
import { RealtimeSocket, type SocketCallbacks } from "../../realtime/socket";
import { appendConversationPage, appendMemberPage } from "../conversations/collection";
import type { ConversationDraft } from "../conversations/create";
import { applyRealtimeEvent, mergeHistoryPage, mergeMessageResult, type PendingMessage, queuePending, reconcilePending, type TimelineState } from "../messages/timeline";
import { createInvalidationCoalescer } from "./invalidation";
import { ListCoordinator, isAbortError, staleRequest } from "./list-coordinator";
import { loadConversationPages, loadMemberPages } from "./load-pages";

export interface WorkspaceSocket { start(): void; stop(): void }
export type WorkspaceSocketFactory = (callbacks: SocketCallbacks) => WorkspaceSocket;
export const defaultSocketFactory: WorkspaceSocketFactory = (callbacks) => new RealtimeSocket(callbacks);

type Scope = { id: number | undefined; selection: number; fetch: number };
type SelectedScope = Scope & { id: number };
type MutationScope = Pick<SelectedScope, "id" | "selection">;
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
	const historyReady = useRef(false);
	const scopeRef = useRef<Scope>({ id: undefined, selection: 0, fetch: 0 });
	const selectionController = useRef<AbortController | null>(null);
	const dataControllers = useRef(new Set<AbortController>());
	const mutationControllers = useRef(new Set<AbortController>());
	const lists = useRef(new ListCoordinator());

	const scopeCurrent = useCallback((scope: Scope, controller?: AbortController) =>
		scopeRef.current.id === scope.id && scopeRef.current.selection === scope.selection &&
		scopeRef.current.fetch === scope.fetch && !controller?.signal.aborted, []);
	const mutationCurrent = useCallback((scope: MutationScope, controller: AbortController) =>
		scopeRef.current.id === scope.id && scopeRef.current.selection === scope.selection && !controller.signal.aborted, []);

	const select = useCallback((id: number | undefined) => {
		selectionController.current?.abort();
		for (const controller of [...dataControllers.current, ...mutationControllers.current]) controller.abort();
		dataControllers.current.clear(); mutationControllers.current.clear();
		const selection = scopeRef.current.selection + 1;
		scopeRef.current = { id, selection, fetch: scopeRef.current.fetch + 1 };
		setSelectedId(id); setSelectionGeneration(selection);
		setDetail(null); setMembers([]); membersRef.current = [];
		historyReady.current = false;
		setTimeline(emptyTimeline()); setPending([]);
		setMemberCursor(undefined); setMessageCursor(undefined);
		setDetailError(""); setMessageError(""); setMutationError("");
		setMemberLoadingMore(false); setMessageLoading(id !== undefined);
	}, []);

	const renewFetchScope = useCallback(() => {
		selectionController.current?.abort();
		for (const controller of dataControllers.current) controller.abort();
		dataControllers.current.clear(); setMemberLoadingMore(false);
		const next = { ...scopeRef.current, fetch: scopeRef.current.fetch + 1 };
		scopeRef.current = next;
		return next;
	}, []);

	const replaceConversations = useCallback((items: Conversation[], cursor?: number) => {
		conversationsRef.current = items; setConversations(items); setConversationCursor(cursor);
	}, []);

	useEffect(() => {
		const initial = { ...scopeRef.current };
		setConversationLoading(true); setConversationError("");
		void lists.current.refresh(async (ticket) => {
			const result = await loadConversationPages(api, 100, ticket.controller.signal);
			if (ticket.controller.signal.aborted) throw staleRequest();
			replaceConversations(result.items, result.nextBeforeId);
			if (scopeCurrent(initial)) select(result.items[0]?.id);
		}).catch((error) => {
			if (!isAbortError(error)) setConversationError(errorDetail(error));
		}).finally(() => setConversationLoading(false));
	}, [api, replaceConversations, scopeCurrent, select]);

	useEffect(() => {
		if (selectedId === undefined) { setMessageLoading(false); return; }
		const scope = { ...scopeRef.current } as SelectedScope;
		const controller = new AbortController(); selectionController.current = controller;
		setMessageLoading(true);
		Promise.all([api.conversation(scope.id, controller.signal), api.members(scope.id, controller.signal), api.history(scope.id, controller.signal)])
			.then(([item, memberPage, messagePage]) => {
				if (!scopeCurrent(scope, controller)) throw staleRequest();
				const result = appendMemberPage([], memberPage);
				setDetail(item); membersRef.current = result.items; setMembers(result.items); setMemberCursor(result.nextBeforeId);
				setTimeline((state) => mergeHistoryPage(state, messagePage.messages)); setMessageCursor(messagePage.next_before_id); historyReady.current = true;
			}).catch((error) => {
				if (scopeCurrent(scope, controller) && !isAbortError(error)) { const text = errorDetail(error); setDetailError(text); setMessageError(text); }
			}).finally(() => { if (scopeCurrent(scope, controller)) setMessageLoading(false); });
		return () => controller.abort();
	}, [api, scopeCurrent, selectedId, selectionGeneration]);

	function captureData() {
		const scope = { ...scopeRef.current };
		if (scope.id === undefined) return null;
		const controller = new AbortController(); dataControllers.current.add(controller);
		return { scope: scope as SelectedScope, controller };
	}

	function captureMutation() {
		const { id, selection } = scopeRef.current;
		if (id === undefined) return null;
		const controller = new AbortController(); mutationControllers.current.add(controller);
		return { scope: { id, selection }, controller };
	}

	async function loadMoreConversations() {
		const cursor = conversationCursor;
		if (cursor === undefined) return;
		const ticket = await lists.current.beginPagination();
		if (ticket === null) return;
		setConversationLoadingMore(true);
		try {
			const page = await api.listConversations(ticket.controller.signal, cursor);
			if (!lists.current.paginationCurrent(ticket)) throw staleRequest();
			const result = appendConversationPage(conversationsRef.current, page);
			replaceConversations(result.items, result.nextBeforeId);
		} catch (error) {
			if (!isAbortError(error)) setConversationError(errorDetail(error));
		} finally {
			if (lists.current.finishPagination(ticket)) setConversationLoadingMore(false);
		}
	}

	async function loadMoreMembers() {
		const action = captureData(); if (action === null || memberCursor === undefined) return;
		setMemberLoadingMore(true);
		try {
			const page = await api.members(action.scope.id, action.controller.signal, memberCursor);
			if (!scopeCurrent(action.scope, action.controller)) throw staleRequest();
			const result = appendMemberPage(membersRef.current, page);
			membersRef.current = result.items; setMembers(result.items); setMemberCursor(result.nextBeforeId);
		} catch (error) {
			if (scopeCurrent(action.scope, action.controller) && !isAbortError(error)) setDetailError(errorDetail(error));
		} finally {
			if (scopeCurrent(action.scope, action.controller)) setMemberLoadingMore(false);
			dataControllers.current.delete(action.controller);
		}
	}

	async function loadOlderMessages() {
		const action = captureData(); if (action === null || messageCursor === undefined) return;
		setMessageLoading(true);
		try {
			const page = await api.history(action.scope.id, action.controller.signal, messageCursor);
			if (!scopeCurrent(action.scope, action.controller)) throw staleRequest();
			setTimeline((state) => mergeHistoryPage(state, page.messages)); setMessageCursor(page.next_before_id);
		} catch (error) {
			if (scopeCurrent(action.scope, action.controller) && !isAbortError(error)) setMessageError(errorDetail(error));
		} finally {
			if (scopeCurrent(action.scope, action.controller)) setMessageLoading(false);
			dataControllers.current.delete(action.controller);
		}
	}

	async function sendMessage(body: string, key: string) {
		const action = captureMutation(); if (action === null) return;
		setPending((items) => queuePending(items, key, body));
		try {
			const result = await api.sendMessage(action.scope.id, body, key, action.controller.signal);
			if (!mutationCurrent(action.scope, action.controller)) throw staleRequest();
			setTimeline((state) => mergeMessageResult(state, result)); setPending((items) => reconcilePending(items, key));
		} catch (error) {
			if (mutationCurrent(action.scope, action.controller)) throw error;
		} finally { mutationControllers.current.delete(action.controller); }
	}

	async function changeMessage(operation: "edit" | "delete", message: Message, body?: string) {
		const action = captureMutation(); if (action === null) return;
		setMutationError("");
		try {
			const result = operation === "edit" ? await api.editMessage(message.id, body!, mutationKey("edit"), action.controller.signal)
				: await api.deleteMessage(message.id, mutationKey("delete"), action.controller.signal);
			if (mutationCurrent(action.scope, action.controller)) setTimeline((state) => mergeMessageResult(state, result));
		} catch (error) {
			if (mutationCurrent(action.scope, action.controller)) setMutationError(errorDetail(error));
		} finally { mutationControllers.current.delete(action.controller); }
	}

	async function createConversation(draft: ConversationDraft) {
		setMutationError("");
		const created = draft.kind === "dm" ? await api.createDM(draft.otherUserId, mutationKey("dm"))
			: await api.createChannel(draft.kind, draft.name, mutationKey("channel"));
		await lists.current.refresh(async (ticket) => {
			const result = await loadConversationPages(api, Math.max(100, conversationsRef.current.length), ticket.controller.signal);
			if (ticket.controller.signal.aborted) throw staleRequest();
			replaceConversations(result.items, result.nextBeforeId); select(created.id);
		});
	}

	const refreshSelected = useCallback(async (scope: SelectedScope, controller: AbortController, history: boolean) => {
		const [item, memberResult] = await Promise.all([
			api.conversation(scope.id, controller.signal),
			loadMemberPages(api, scope.id, Math.max(100, membersRef.current.length), controller.signal),
		]);
		const page = history ? await api.history(scope.id, controller.signal) : null;
		if (!scopeCurrent(scope, controller)) throw staleRequest();
		setDetail(item); membersRef.current = memberResult.items; setMembers(memberResult.items); setMemberCursor(memberResult.nextBeforeId);
		if (page !== null) { setTimeline((state) => mergeHistoryPage(state, page.messages)); setMessageCursor(page.next_before_id); }
		setDetailError(""); setMessageError("");
	}, [api, scopeCurrent]);

	const refreshAuthoritative = useCallback(async (history: boolean) => {
		const scope = history ? renewFetchScope() : { ...scopeRef.current };
		let refreshingSelection = false;
		let refreshingHistory = history;
		if (history && scope.id !== undefined) setMessageLoading(true);
		try {
			await lists.current.refresh(async (ticket) => {
				if (!scopeCurrent(scope)) throw staleRequest();
				if (history) selectionController.current = ticket.controller;
				const result = await loadConversationPages(api, Math.max(100, conversationsRef.current.length), ticket.controller.signal);
				if (ticket.controller.signal.aborted || !scopeCurrent(scope)) throw staleRequest();
				replaceConversations(result.items, result.nextBeforeId);
				if (scope.id === undefined) return;
				if (!result.items.some((item) => item.id === scope.id)) { select(undefined); return; }
				if (!history) {
					refreshingHistory = !historyReady.current;
					selectionController.current?.abort(); selectionController.current = ticket.controller;
					if (refreshingHistory) setMessageLoading(true);
				}
				refreshingSelection = true;
				await refreshSelected(scope as SelectedScope, ticket.controller, refreshingHistory);
			});
			if (refreshingHistory && scopeCurrent(scope)) { historyReady.current = true; setMessageLoading(false); }
		} catch (error) {
			if (!scopeCurrent(scope) || isAbortError(error)) throw staleRequest();
			if (refreshingSelection && error instanceof ApiProblem && error.code === "not_found" && scope.id !== undefined) { select(undefined); return; }
			const text = history ? `Resync failed: ${errorDetail(error)}` : errorDetail(error);
			setConversationError(text); setDetailError(text); if (refreshingHistory) { setMessageError(text); setMessageLoading(false); }
			throw error;
		}
	}, [api, refreshSelected, renewFetchScope, replaceConversations, scopeCurrent, select]);

	useEffect(() => {
		const coalescer = createInvalidationCoalescer(() => refreshAuthoritative(false));
		const socket = socketFactory({
			onStatus: setConnection,
			onEvent: (event) => {
				if (event.type.startsWith("conversation.")) coalescer.request();
				if (event.type.startsWith("message.") && event.conversation_id === scopeRef.current.id) setTimeline((state) => applyRealtimeEvent(state, event));
			},
			onResync: () => refreshAuthoritative(true),
		});
		socket.start();
		return () => { coalescer.stop(); socket.stop(); };
	}, [refreshAuthoritative, socketFactory]);

	useEffect(() => () => {
		selectionController.current?.abort(); lists.current.dispose();
		for (const controller of [...dataControllers.current, ...mutationControllers.current]) controller.abort();
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
