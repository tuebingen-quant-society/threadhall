import { describe, expect, it, vi } from "vitest";

import type { TimelineState } from "../features/messages/timeline";
import { applyRealtimeEvent, mergeMessageResult } from "../features/messages/timeline";
import { RealtimeSocket } from "./socket";

class FakeSocket {
	static instances: FakeSocket[] = [];
	url: string;
	readyState = 0;
	onopen: (() => void) | null = null;
	onmessage: ((event: MessageEvent<string>) => void) | null = null;
	onclose: ((event: CloseEvent) => void) | null = null;
	onerror: (() => void) | null = null;
	constructor(url: string) { this.url = url; FakeSocket.instances.push(this); }
	close() { this.readyState = 3; }
	open() { this.readyState = 1; this.onopen?.(); }
	message(data: unknown) { this.onmessage?.({ data: JSON.stringify(data) } as MessageEvent<string>); }
	disconnect(reason = "") { this.readyState = 3; this.onclose?.({ reason } as CloseEvent); }
}

describe("RealtimeSocket", () => {
	it("deduplicates replay and reconnects with the in-memory last sequence", () => {
		vi.useFakeTimers();
		FakeSocket.instances = [];
		const received = vi.fn();
		const socket = new RealtimeSocket({ onEvent: received }, FakeSocket as never);
		socket.start();
		FakeSocket.instances[0].open();
		const event = { seq: 4, type: "message.sent", conversation_id: 1, entity_id: 2, payload: { author_id: 1, body: "hi", rendered_body: "<p>hi</p>", created_at: "2026-08-25T12:00:00Z" } };
		FakeSocket.instances[0].message(event);
		FakeSocket.instances[0].message(event);
		FakeSocket.instances[0].disconnect();
		vi.runOnlyPendingTimers();

		expect(received).toHaveBeenCalledTimes(1);
		expect(FakeSocket.instances[1].url).toContain("after_seq=4");
		socket.stop();
		vi.useRealTimers();
	});

	it("advances only from sockets while entity-deduplicating delayed events after HTTP", () => {
		vi.useFakeTimers();
		FakeSocket.instances = [];
		const message = { id: 2, conversation_id: 1, author_id: 4, body: "HTTP edit", rendered_body: "<p>HTTP edit</p>", created_at: "2026-08-25T12:00:00Z", edited_at: "2026-08-25T12:09:00Z" };
		let timeline: TimelineState = mergeMessageResult({ messages: [], entitySeq: new Map() }, {
			message,
			event: { seq: 9, type: "message.edited", conversation_id: 1, entity_id: 2, payload: {} },
		});
		const afterHTTP = timeline;
		const socket = new RealtimeSocket({ onEvent: (event) => { timeline = applyRealtimeEvent(timeline, event); } }, FakeSocket as never);
		socket.start();
		FakeSocket.instances[0].open();
		FakeSocket.instances[0].message({
			seq: 8, type: "message.sent", conversation_id: 1, entity_id: 2,
			payload: { author_id: 4, body: "stale", rendered_body: "<p>stale</p>", created_at: message.created_at },
		});
		FakeSocket.instances[0].disconnect();
		vi.advanceTimersByTime(500);

		expect(timeline).toBe(afterHTTP);
		expect(FakeSocket.instances[1].url).toContain("after_seq=8");
		FakeSocket.instances[1].open();
		FakeSocket.instances[1].message({
			seq: 9, type: "message.edited", conversation_id: 1, entity_id: 2,
			payload: { body: message.body, rendered_body: message.rendered_body, edited_at: message.edited_at },
		});
		FakeSocket.instances[1].disconnect();
		vi.advanceTimersByTime(500);

		expect(timeline).toBe(afterHTTP);
		expect(FakeSocket.instances[2].url).toContain("after_seq=9");
		socket.stop();
		vi.useRealTimers();
	});

	it("keeps resync failed until an authoritative retry succeeds", async () => {
		vi.useFakeTimers();
		FakeSocket.instances = [];
		const statuses: string[] = [];
		const resync = vi.fn().mockRejectedValueOnce(new Error("reload failed")).mockResolvedValueOnce(undefined);
		const socket = new RealtimeSocket({ onEvent: vi.fn(), onResync: resync, onStatus: (status) => statuses.push(status) }, FakeSocket as never);
		socket.start();
		FakeSocket.instances[0].message({ type: "resync_required" });
		await Promise.resolve();
		await Promise.resolve();
		expect(FakeSocket.instances).toHaveLength(1);
		expect(statuses.at(-1)).toBe("error");
		await vi.advanceTimersByTimeAsync(500);

		expect(resync).toHaveBeenCalledTimes(2);
		expect(FakeSocket.instances[1].url).toContain("after_seq=0");
		socket.stop();
		vi.useRealTimers();
	});

	it("rejects invalid and empty payloads without advancing the replay cursor", () => {
		vi.useFakeTimers();
		FakeSocket.instances = [];
		const received = vi.fn();
		const socket = new RealtimeSocket({ onEvent: received }, FakeSocket as never);
		socket.start();
		FakeSocket.instances[0].message({ seq: 8, type: "message.sent", conversation_id: 1, entity_id: 2, payload: {} });
		FakeSocket.instances[0].message({ seq: 9, type: "message.sent", conversation_id: 1, entity_id: 2, payload: { body: "missing fields" } });
		FakeSocket.instances[0].disconnect();
		vi.advanceTimersByTime(500);

		expect(received).not.toHaveBeenCalled();
		expect(FakeSocket.instances[1].url).toContain("after_seq=0");
		socket.stop();
		vi.useRealTimers();
	});

	it("retains exponential backoff across short upgrade-close loops", () => {
		vi.useFakeTimers();
		FakeSocket.instances = [];
		const socket = new RealtimeSocket({ onEvent: vi.fn() }, FakeSocket as never);
		socket.start();
		FakeSocket.instances[0].open();
		FakeSocket.instances[0].disconnect();
		vi.advanceTimersByTime(500);
		expect(FakeSocket.instances).toHaveLength(2);
		FakeSocket.instances[1].open();
		FakeSocket.instances[1].disconnect();
		vi.advanceTimersByTime(999);
		expect(FakeSocket.instances).toHaveLength(2);
		vi.advanceTimersByTime(1);
		expect(FakeSocket.instances).toHaveLength(3);
		socket.stop();
		vi.useRealTimers();
	});

	it("resets reconnect backoff only after a stable ten-second open", () => {
		vi.useFakeTimers();
		FakeSocket.instances = [];
		const socket = new RealtimeSocket({ onEvent: vi.fn() }, FakeSocket as never);
		socket.start();
		FakeSocket.instances[0].disconnect();
		vi.advanceTimersByTime(500);
		FakeSocket.instances[1].open();
		vi.advanceTimersByTime(10_000);
		FakeSocket.instances[1].disconnect();
		vi.advanceTimersByTime(499);
		expect(FakeSocket.instances).toHaveLength(2);
		vi.advanceTimersByTime(1);
		expect(FakeSocket.instances).toHaveLength(3);
		socket.stop();
		vi.useRealTimers();
	});
});
