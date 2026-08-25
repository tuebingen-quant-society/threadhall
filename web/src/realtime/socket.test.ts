import { describe, expect, it, vi } from "vitest";

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
		FakeSocket.instances[0].message({ seq: 4, type: "message.sent", conversation_id: 1, entity_id: 2, payload: {} });
		FakeSocket.instances[0].message({ seq: 4, type: "message.sent", conversation_id: 1, entity_id: 2, payload: {} });
		FakeSocket.instances[0].disconnect();
		vi.runOnlyPendingTimers();

		expect(received).toHaveBeenCalledTimes(1);
		expect(FakeSocket.instances[1].url).toContain("after_seq=4");
		socket.stop();
		vi.useRealTimers();
	});

	it("performs authoritative resync before reopening from a fresh cursor", async () => {
		vi.useFakeTimers();
		FakeSocket.instances = [];
		const resync = vi.fn().mockResolvedValue(undefined);
		const socket = new RealtimeSocket({ onEvent: vi.fn(), onResync: resync }, FakeSocket as never);
		socket.start();
		FakeSocket.instances[0].message({ type: "resync_required" });
		await vi.runAllTimersAsync();

		expect(resync).toHaveBeenCalledTimes(1);
		expect(FakeSocket.instances[1].url).toContain("after_seq=0");
		socket.stop();
		vi.useRealTimers();
	});
});
