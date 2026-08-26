import { useEffect, useRef, useState } from "preact/hooks";

import type { Message, MessageResult, RealtimeEvent } from "../../api/types";

export const MAX_CLIENT_MESSAGES = 200;
const MAX_ENTITY_PATCHES = 200;
const MAX_PINNED_MESSAGES = 20;

type TimelineWindow = "latest" | "older";
type EntityPatch =
	| { kind: "edit"; body: string; renderedBody: string; editedAt: string }
	| { kind: "delete"; deletedAt: string };

export interface TimelineState {
	messages: Message[];
	entitySeq: Map<number, number>;
	entityPatches: Map<number, EntityPatch>;
	pinnedIds: Set<number>;
	historyGeneration: number;
	window: TimelineWindow;
}

export interface PendingMessage {
	idempotencyKey: string;
	body: string;
	queuedAt: string;
}

export const emptyTimeline = (): TimelineState => ({
	messages: [], entitySeq: new Map(), entityPatches: new Map(), pinnedIds: new Set(), historyGeneration: 0, window: "latest",
});

function objectPayload(payload: unknown): Record<string, unknown> | null {
	return typeof payload === "object" && payload !== null && !Array.isArray(payload) ? payload as Record<string, unknown> : null;
}

function stringField(payload: Record<string, unknown>, field: string) {
	return typeof payload[field] === "string" ? payload[field] as string : undefined;
}

function pin(pinnedIds: Set<number>, id: number) {
	const next = new Set(pinnedIds); next.delete(id); next.add(id);
	while (next.size > MAX_PINNED_MESSAGES) next.delete(next.values().next().value!);
	return next;
}

function retainMessages(items: Message[], window: TimelineWindow, pinnedIds: Set<number>) {
	const sorted = [...new Map(items.map((item) => [item.id, item])).values()].sort((left, right) => left.id - right.id);
	if (sorted.length <= MAX_CLIENT_MESSAGES) return sorted;
	const pinned = sorted.filter((item) => pinnedIds.has(item.id));
	const unpinned = sorted.filter((item) => !pinnedIds.has(item.id));
	const capacity = MAX_CLIENT_MESSAGES - pinned.length;
	const edge = window === "latest" ? unpinned.slice(-capacity) : unpinned.slice(0, capacity);
	return [...edge, ...pinned].sort((left, right) => left.id - right.id);
}

function upsert(messages: Message[], item: Message, window: TimelineWindow = "latest", pinnedIds = new Set<number>()) {
	return retainMessages([...messages.filter((message) => message.id !== item.id), item], window, pinnedIds);
}

export function mergeMessages(existing: Message[], incoming: Message[]) {
	return incoming.reduce((messages, item) => upsert(messages, item), existing);
}

function recordPatch(patches: Map<number, EntityPatch>, id: number, patch: EntityPatch) {
	const next = new Map(patches);
	next.delete(id);
	next.set(id, patch);
	let overflowed = false;
	while (next.size > MAX_ENTITY_PATCHES) { next.delete(next.keys().next().value!); overflowed = true; }
	return { patches: next, overflowed };
}

function applyPatch(item: Message, patch: EntityPatch) {
	return patch.kind === "edit"
		? { ...item, body: patch.body, rendered_body: patch.renderedBody, edited_at: patch.editedAt }
		: { ...item, body: "", rendered_body: "", deleted_at: patch.deletedAt };
}

function compactState(messages: Message[], entitySeq: Map<number, number>, entityPatches: Map<number, EntityPatch>, pinnedIds: Set<number>, historyGeneration: number, window: TimelineWindow) {
	const retained = new Set([...messages.map((message) => message.id), ...entityPatches.keys()]);
	return {
		messages, entitySeq: new Map([...entitySeq].filter(([id]) => retained.has(id))), entityPatches,
		pinnedIds: new Set([...pinnedIds].filter((id) => retained.has(id))), historyGeneration, window,
	};
}

export function applyRealtimeEvent(state: TimelineState, event: RealtimeEvent): TimelineState {
	if (event.seq <= (state.entitySeq.get(event.entity_id) ?? 0)) return state;
	const payload = objectPayload(event.payload);
	if (payload === null) return state;
	let messages = state.messages;
	let entityPatches = state.entityPatches;
	let pinnedIds = state.pinnedIds;
	let historyGeneration = state.historyGeneration;
	if (event.type === "message.sent") {
		const author = payload.author_id;
		const body = stringField(payload, "body");
		const rendered = stringField(payload, "rendered_body");
		const created = stringField(payload, "created_at");
		if (typeof author === "number" && body !== undefined && rendered !== undefined && created !== undefined) {
			pinnedIds = pin(pinnedIds, event.entity_id);
			messages = upsert(messages, {
				id: event.entity_id, conversation_id: event.conversation_id, author_id: author,
				body, rendered_body: rendered, created_at: created,
			}, state.window, pinnedIds);
			if (entityPatches.has(event.entity_id)) {
				entityPatches = new Map(entityPatches); entityPatches.delete(event.entity_id);
			}
		} else return state;
	} else if (event.type === "message.edited") {
		const body = stringField(payload, "body");
		const rendered = stringField(payload, "rendered_body");
		const edited = stringField(payload, "edited_at");
		if (body !== undefined && rendered !== undefined && edited !== undefined) {
			pinnedIds = pin(pinnedIds, event.entity_id);
			if (messages.some((message) => message.id === event.entity_id)) {
				messages = messages.map((message) => message.id === event.entity_id
					? { ...message, body, rendered_body: rendered, edited_at: edited }
					: message);
			} else {
				const recorded = recordPatch(entityPatches, event.entity_id, { kind: "edit", body, renderedBody: rendered, editedAt: edited });
				entityPatches = recorded.patches; if (recorded.overflowed) historyGeneration += 1;
			}
		} else return state;
	} else if (event.type === "message.deleted") {
		const deleted = stringField(payload, "deleted_at");
		if (deleted !== undefined) pinnedIds = pin(pinnedIds, event.entity_id);
		if (deleted !== undefined && messages.some((message) => message.id === event.entity_id)) {
			messages = messages.map((message) => message.id === event.entity_id
				? { ...message, body: "", rendered_body: "", deleted_at: deleted }
				: message);
		} else if (deleted !== undefined) {
			const recorded = recordPatch(entityPatches, event.entity_id, { kind: "delete", deletedAt: deleted });
			entityPatches = recorded.patches; if (recorded.overflowed) historyGeneration += 1;
		}
		else return state;
	} else return state;
	const entitySeq = new Map(state.entitySeq);
	entitySeq.set(event.entity_id, event.seq);
	return compactState(messages, entitySeq, entityPatches, pinnedIds, historyGeneration, state.window);
}

export function mergeMessageResult(state: TimelineState, result: MessageResult): TimelineState {
	if (result.event.seq <= (state.entitySeq.get(result.message.id) ?? 0)) return state;
	const pinnedIds = pin(state.pinnedIds, result.message.id);
	const messages = upsert(state.messages, result.message, state.window, pinnedIds);
	const entitySeq = new Map(state.entitySeq);
	entitySeq.set(result.message.id, result.event.seq);
	const entityPatches = new Map(state.entityPatches); entityPatches.delete(result.message.id);
	return compactState(messages, entitySeq, entityPatches, pinnedIds, state.historyGeneration, state.window);
}

function mergeHTTPPage(state: TimelineState, incoming: Message[], window: TimelineWindow, generation: number, recovery = false): TimelineState {
	if (state.historyGeneration !== generation) return state;
	const entityPatches = new Map(state.entityPatches);
	const patched = incoming.map((item) => {
		const patch = entityPatches.get(item.id);
		if (patch === undefined) return item;
		entityPatches.delete(item.id);
		return applyPatch(item, patch);
	});
	const combined = new Map(patched.map((item) => [item.id, item]));
	for (const item of state.messages) combined.set(item.id, item);
	const messages = retainMessages([...combined.values()], window, state.pinnedIds);
	return compactState(messages, state.entitySeq, recovery ? new Map() : entityPatches, state.pinnedIds, generation, window);
}

export const mergeHistoryPage = (state: TimelineState, incoming: Message[], generation = state.historyGeneration) => mergeHTTPPage(state, incoming, "latest", generation);
export const mergeOlderHistoryPage = (state: TimelineState, incoming: Message[], generation = state.historyGeneration) => mergeHTTPPage(state, incoming, "older", generation);
export const recoverHistoryPage = (state: TimelineState, incoming: Message[], generation: number) => mergeHTTPPage(state, incoming, "latest", generation, true);

export function queuePending(items: PendingMessage[], idempotencyKey: string, body: string) {
	if (items.some((item) => item.idempotencyKey === idempotencyKey)) return items;
	return [...items, { idempotencyKey, body, queuedAt: new Date().toISOString() }].slice(-20);
}

export function reconcilePending(items: PendingMessage[], idempotencyKey: string) {
	return items.filter((item) => item.idempotencyKey !== idempotencyKey);
}

interface TimelineProps {
	messages: Message[];
	pending?: PendingMessage[];
	currentUserId: number;
	memberNames: Map<number, string>;
	loading?: boolean;
	error?: string;
	hasOlder?: boolean;
	onLoadOlder?: () => void;
	onEdit: (message: Message, body: string) => void | Promise<void>;
	onDelete: (message: Message) => void | Promise<void>;
}

function MessageRow({ message, own, author, onEdit, onDelete }: {
	message: Message; own: boolean; author: string;
	onEdit: TimelineProps["onEdit"]; onDelete: TimelineProps["onDelete"];
}) {
	const [editing, setEditing] = useState(false);
	const [draft, setDraft] = useState(message.body);
	const deleted = Boolean(message.deleted_at);
	const time = new Date(message.created_at);

	function save(event: Event) {
		event.preventDefault();
		if (draft.trim() === "") return;
		void onEdit(message, draft);
		setEditing(false);
	}

	return (
		<article class="message-row" data-message-id={message.id}>
			<header>
				<strong>{author}</strong>
				<time dateTime={message.created_at} title={time.toLocaleString()}>{time.toLocaleTimeString([], { hour: "2-digit", minute: "2-digit" })}</time>
				{message.edited_at && !deleted && <span>edited</span>}
			</header>
			{deleted ? <p class="tombstone">Message deleted</p> : editing ? (
				<form class="edit-form" onSubmit={save}>
					<label class="sr-only" for={`edit-${message.id}`}>Edit message text</label>
					<textarea id={`edit-${message.id}`} value={draft} onInput={(event) => setDraft(event.currentTarget.value)} maxLength={16_384} />
					<div><button class="text-button" type="button" onClick={() => setEditing(false)}>Cancel</button><button class="small-button" type="submit">Save edit</button></div>
				</form>
			) : <div class="message-body" dangerouslySetInnerHTML={{ __html: message.rendered_body }} />}
			{own && !deleted && !editing && <div class="message-actions">
				<button type="button" aria-label="Edit message" onClick={() => setEditing(true)}>Edit</button>
				<button type="button" aria-label="Delete message" onClick={() => void onDelete(message)}>Delete</button>
			</div>}
		</article>
	);
}

export function Timeline(props: TimelineProps) {
	const end = useRef<HTMLDivElement>(null);
	useEffect(() => end.current?.scrollIntoView?.({ block: "end" }), [props.messages.length, props.pending?.length]);

	return (
		<section class="timeline" aria-label="Message history" aria-busy={props.loading}>
			{props.hasOlder && <button class="load-older" type="button" onClick={props.onLoadOlder}>Load earlier messages</button>}
			{props.loading && props.messages.length === 0 && <div class="timeline-state"><span class="loading-line" /><p>Loading conversation…</p></div>}
			{props.error && <p class="inline-error" role="alert">{props.error}</p>}
			{!props.loading && !props.error && props.messages.length === 0 && !props.pending?.length && <div class="timeline-state"><p class="section-kicker">No messages yet</p><h2>Begin the thread.</h2><p>Write the first note for this conversation.</p></div>}
			{props.messages.map((message) => <MessageRow
				key={message.id} message={message} own={message.author_id === props.currentUserId}
				author={props.memberNames.get(message.author_id) ?? `User ${message.author_id}`}
				onEdit={props.onEdit} onDelete={props.onDelete}
			/>)}
			{props.pending?.map((message) => <article class="message-row pending-message" key={message.idempotencyKey}>
				<header><strong>You</strong><span>Sending…</span></header><p>{message.body}</p>
			</article>)}
			<div ref={end} />
		</section>
	);
}
