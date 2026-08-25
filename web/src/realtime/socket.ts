import type { ConnectionState, RealtimeEvent } from "../api/types";

interface SocketCallbacks {
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

const MAX_RECONNECT_DELAY_MS = 15_000;

function socketURL(afterSeq: number) {
	const url = new URL("/api/v1/realtime", window.location.href);
	url.protocol = url.protocol === "https:" ? "wss:" : "ws:";
	url.searchParams.set("after_seq", String(afterSeq));
	return url.toString();
}

function isEvent(value: unknown): value is RealtimeEvent {
	if (typeof value !== "object" || value === null) return false;
	const event = value as Partial<RealtimeEvent>;
	return Number.isSafeInteger(event.seq) && (event.seq ?? 0) > 0 &&
		Number.isSafeInteger(event.conversation_id) && (event.conversation_id ?? 0) > 0 &&
		Number.isSafeInteger(event.entity_id) && (event.entity_id ?? 0) > 0 &&
		typeof event.type === "string" && typeof event.payload === "object" && event.payload !== null;
}

export class RealtimeSocket {
	private connection: SocketLike | null = null;
	private reconnectTimer: ReturnType<typeof setTimeout> | null = null;
	private active = false;
	private resyncing = false;
	private attempts = 0;
	private lastSeq = 0;

	constructor(
		private readonly callbacks: SocketCallbacks,
		private readonly Socket: SocketConstructor = WebSocket,
	) {}

	start() {
		if (this.active) return;
		this.active = true;
		this.connect();
	}

	stop() {
		this.active = false;
		this.resyncing = false;
		if (this.reconnectTimer !== null) clearTimeout(this.reconnectTimer);
		this.reconnectTimer = null;
		this.connection?.close();
		this.connection = null;
		this.callbacks.onStatus?.("offline");
	}

	private connect() {
		if (!this.active || this.resyncing) return;
		this.callbacks.onStatus?.(this.attempts === 0 ? "connecting" : "reconnecting");
		const connection = new this.Socket(socketURL(this.lastSeq));
		this.connection = connection;
		connection.onopen = () => {
			if (connection !== this.connection) return;
			this.attempts = 0;
			this.callbacks.onStatus?.("connected");
		};
		connection.onmessage = (message) => this.receive(message.data);
		connection.onerror = () => undefined;
		connection.onclose = (event) => {
			if (connection !== this.connection || !this.active) return;
			this.connection = null;
			if (event.reason === "resync_required") void this.resync();
			else this.scheduleReconnect();
		};
	}

	private receive(data: string) {
		let decoded: unknown;
		try { decoded = JSON.parse(data); } catch { return; }
		if (typeof decoded === "object" && decoded !== null && (decoded as { type?: string }).type === "resync_required") {
			void this.resync();
			return;
		}
		if (!isEvent(decoded) || decoded.seq <= this.lastSeq) return;
		this.lastSeq = decoded.seq;
		this.callbacks.onEvent(decoded);
	}

	private scheduleReconnect() {
		if (!this.active || this.reconnectTimer !== null) return;
		this.attempts += 1;
		this.callbacks.onStatus?.("reconnecting");
		const delay = Math.min(MAX_RECONNECT_DELAY_MS, 500 * 2 ** Math.min(this.attempts - 1, 5));
		this.reconnectTimer = setTimeout(() => {
			this.reconnectTimer = null;
			this.connect();
		}, delay);
	}

	private async resync() {
		if (!this.active || this.resyncing) return;
		this.resyncing = true;
		this.connection?.close();
		this.connection = null;
		this.callbacks.onStatus?.("resyncing");
		try {
			await this.callbacks.onResync?.();
			this.lastSeq = 0;
			this.attempts = 0;
		} finally {
			this.resyncing = false;
			if (this.active) this.connect();
		}
	}
}
