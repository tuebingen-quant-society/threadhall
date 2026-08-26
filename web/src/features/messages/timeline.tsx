import { useEffect, useRef, useState } from "preact/hooks";

import type { Message, MessageResult, RealtimeEvent } from "../../api/types";

export const MAX_CLIENT_MESSAGES = 200;

export interface TimelineState {
	messages: Message[];
	entitySeq: Map<number, number>;
}

export interface PendingMessage {
	idempotencyKey: string;
	body: string;
	queuedAt: string;
}

function objectPayload(payload: unknown): Record<string, unknown> | null {
	return typeof payload === "object" && payload !== null && !Array.isArray(payload) ? payload as Record<string, unknown> : null;
}

function stringField(payload: Record<string, unknown>, field: string) {
	return typeof payload[field] === "string" ? payload[field] as string : undefined;
}

function upsert(messages: Message[], item: Message) {
	const next = messages.filter((message) => message.id !== item.id);
	next.push(item);
	next.sort((left, right) => left.id - right.id);
	return next.slice(-MAX_CLIENT_MESSAGES);
}

export function mergeMessages(existing: Message[], incoming: Message[]) {
	return incoming.reduce(upsert, existing);
}

export function applyRealtimeEvent(state: TimelineState, event: RealtimeEvent): TimelineState {
	if (event.seq <= (state.entitySeq.get(event.entity_id) ?? 0)) return state;
	const payload = objectPayload(event.payload);
	if (payload === null) return state;
	let messages = state.messages;
	if (event.type === "message.sent") {
		const author = payload.author_id;
		const body = stringField(payload, "body");
		const rendered = stringField(payload, "rendered_body");
		const created = stringField(payload, "created_at");
		if (typeof author === "number" && body !== undefined && rendered !== undefined && created !== undefined) {
			messages = upsert(messages, {
				id: event.entity_id, conversation_id: event.conversation_id, author_id: author,
				body, rendered_body: rendered, created_at: created,
			});
		} else return state;
	} else if (event.type === "message.edited") {
		if (!messages.some((message) => message.id === event.entity_id)) return state;
		const body = stringField(payload, "body");
		const rendered = stringField(payload, "rendered_body");
		const edited = stringField(payload, "edited_at");
		if (body !== undefined && rendered !== undefined && edited !== undefined) {
			messages = messages.map((message) => message.id === event.entity_id
				? { ...message, body, rendered_body: rendered, edited_at: edited }
				: message);
		} else return state;
	} else if (event.type === "message.deleted") {
		if (!messages.some((message) => message.id === event.entity_id)) return state;
		const deleted = stringField(payload, "deleted_at");
		if (deleted !== undefined) messages = messages.map((message) => message.id === event.entity_id
			? { ...message, body: "", rendered_body: "", deleted_at: deleted }
			: message);
		else return state;
	} else return state;
	if (messages === state.messages) return state;
	const identifiers = new Set(messages.map((message) => message.id));
	const entitySeq = new Map([...state.entitySeq].filter(([id]) => identifiers.has(id)));
	entitySeq.set(event.entity_id, event.seq);
	return { messages, entitySeq };
}

export function mergeMessageResult(state: TimelineState, result: MessageResult): TimelineState {
	if (result.event.seq <= (state.entitySeq.get(result.message.id) ?? 0)) return state;
	const messages = mergeMessages(state.messages, [result.message]);
	const identifiers = new Set(messages.map((message) => message.id));
	const entitySeq = new Map([...state.entitySeq].filter(([id]) => identifiers.has(id)));
	entitySeq.set(result.message.id, result.event.seq);
	return { messages, entitySeq };
}

export function mergeHistoryPage(state: TimelineState, incoming: Message[]): TimelineState {
	const messages = mergeMessages(incoming, state.messages);
	const identifiers = new Set(messages.map((message) => message.id));
	return { messages, entitySeq: new Map([...state.entitySeq].filter(([id]) => identifiers.has(id))) };
}

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
