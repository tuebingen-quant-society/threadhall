import type { ConnectionState, RealtimeEvent } from "../api/types";

export interface SocketCallbacks {
	onEvent: (event: RealtimeEvent) => void;
	onStatus?: (status: ConnectionState) => void;
	onResync?: () => Promise<void>;
}

interface SocketLike {
	readyState: number;
	onopen: (() => void) | null;
	onmessage: ((event: MessageEvent<string>) => void) | null;
	onclose: ((event: CloseEvent) => void) | null;
	onerror: (() => void) | null;
	close(): void;
}

type SocketConstructor = new (url: string) => SocketLike;
const MAX_RETRY_DELAY_MS = 15_000;
const STABLE_OPEN_MS = 10_000;

function socketURL(afterSeq: number) {
	const url = new URL("/api/v1/realtime", window.location.href);
	url.protocol = url.protocol === "https:" ? "wss:" : "ws:";
	url.searchParams.set("after_seq", String(afterSeq));
	return url.toString();
}

function record(value: unknown): Record<string, unknown> | null {
	return typeof value === "object" && value !== null && !Array.isArray(value) ? value as Record<string, unknown> : null;
}

function strings(value: Record<string, unknown>, names: string[]) {
	return names.every((name) => typeof value[name] === "string" && (value[name] as string).length > 0);
}

function validPayload(type: string, payload: unknown) {
	const value = record(payload);
	switch (type) {
	case "message.sent":
		return value !== null && Number.isSafeInteger(value.author_id) && (value.author_id as number) > 0 &&
			strings(value, ["body", "rendered_body", "created_at"]);
	case "message.edited":
		return value !== null && strings(value, ["body", "rendered_body", "edited_at"]);
	case "message.deleted":
		return value !== null && strings(value, ["deleted_at"]);
	case "conversation.member_added":
	case "conversation.member_removed":
		return Array.isArray(payload) && payload.length === 2 && payload.every((item) => Number.isSafeInteger(item) && item > 0);
	case "conversation.created":
		return Array.isArray(payload) && payload.length === 2 && payload.every((item) =>
			(typeof item === "string" && item.length > 0) || (Number.isSafeInteger(item) && item > 0));
	default:
		return false;
	}
}

function isEvent(value: unknown): value is RealtimeEvent {
	if (typeof value !== "object" || value === null) return false;
	const event = value as Partial<RealtimeEvent>;
	return Number.isSafeInteger(event.seq) && (event.seq ?? 0) > 0 &&
		Number.isSafeInteger(event.conversation_id) && (event.conversation_id ?? 0) > 0 &&
		Number.isSafeInteger(event.entity_id) && (event.entity_id ?? 0) > 0 &&
		typeof event.type === "string" && validPayload(event.type, event.payload);
}

function retryDelay(attempt: number) {
	return Math.min(MAX_RETRY_DELAY_MS, 500 * 2 ** Math.min(attempt - 1, 5));
}

export class RealtimeSocket {
	private connection: SocketLike | null = null;
	private reconnectTimer: ReturnType<typeof setTimeout> | null = null;
	private resyncTimer: ReturnType<typeof setTimeout> | null = null;
	private stabilityTimer: ReturnType<typeof setTimeout> | null = null;
	private active = false;
	private resyncing = false;
	private attempts = 0;
	private resyncAttempts = 0;
	private socketSeq = 0;

	constructor(private readonly callbacks: SocketCallbacks, private readonly Socket: SocketConstructor = WebSocket) {}

	start() {
		if (this.active) return;
		this.active = true;
		this.connect();
	}

	stop() {
		this.active = false;
		this.resyncing = false;
		this.clearTimers();
		this.connection?.close();
		this.connection = null;
		this.callbacks.onStatus?.("offline");
	}

	private clearTimers() {
		if (this.reconnectTimer !== null) clearTimeout(this.reconnectTimer);
		if (this.resyncTimer !== null) clearTimeout(this.resyncTimer);
		if (this.stabilityTimer !== null) clearTimeout(this.stabilityTimer);
		this.reconnectTimer = this.resyncTimer = this.stabilityTimer = null;
	}

	private connect() {
		if (!this.active || this.resyncing) return;
		this.callbacks.onStatus?.(this.attempts === 0 ? "connecting" : "reconnecting");
		const connection = new this.Socket(socketURL(this.socketSeq));
		this.connection = connection;
		connection.onopen = () => {
			if (connection !== this.connection) return;
			this.callbacks.onStatus?.("connected");
			this.stabilityTimer = setTimeout(() => {
				if (connection === this.connection) this.markStable();
			}, STABLE_OPEN_MS);
		};
		connection.onmessage = (message) => this.receive(message.data);
		connection.onerror = () => undefined;
		connection.onclose = (event) => {
			if (connection !== this.connection || !this.active) return;
			if (this.stabilityTimer !== null) clearTimeout(this.stabilityTimer);
			this.stabilityTimer = null;
			this.connection = null;
			if (event.reason === "resync_required") void this.resync();
			else this.scheduleReconnect();
		};
	}

	private markStable() {
		this.attempts = 0;
		if (this.stabilityTimer !== null) clearTimeout(this.stabilityTimer);
		this.stabilityTimer = null;
	}

	private receive(data: string) {
		let decoded: unknown;
		try { decoded = JSON.parse(data); } catch { return; }
		if (typeof decoded === "object" && decoded !== null && (decoded as { type?: string }).type === "resync_required") {
			void this.resync();
			return;
		}
		if (!isEvent(decoded) || decoded.seq <= this.socketSeq) return;
		this.socketSeq = decoded.seq;
		this.markStable();
		this.callbacks.onEvent(decoded);
	}

	private scheduleReconnect() {
		if (!this.active || this.reconnectTimer !== null) return;
		this.attempts += 1;
		this.callbacks.onStatus?.("reconnecting");
		this.reconnectTimer = setTimeout(() => {
			this.reconnectTimer = null;
			this.connect();
		}, retryDelay(this.attempts));
	}

	private scheduleResync() {
		if (!this.active || this.resyncTimer !== null) return;
		this.resyncAttempts += 1;
		this.resyncTimer = setTimeout(() => {
			this.resyncTimer = null;
			void this.resync();
		}, retryDelay(this.resyncAttempts));
	}

	private async resync() {
		if (!this.active || this.resyncing) return;
		this.resyncing = true;
		this.connection?.close();
		this.connection = null;
		this.callbacks.onStatus?.("resyncing");
		try {
			if (this.callbacks.onResync === undefined) throw new Error("authoritative resync is unavailable");
			await this.callbacks.onResync();
			this.socketSeq = 0;
			this.resyncAttempts = 0;
			this.resyncing = false;
			if (this.active) this.connect();
		} catch {
			this.resyncing = false;
			this.callbacks.onStatus?.("error");
			this.scheduleResync();
		}
	}
}
