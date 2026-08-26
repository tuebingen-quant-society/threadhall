import { act } from "preact/test-utils";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import { fakeApi, general, installMatchMedia, ready, renderWorkspace, socketHarness } from "./chat-workspace-test-utils";

beforeEach(installMatchMedia);
afterEach(() => vi.useRealTimers());

async function settle() {
	await Promise.resolve();
	await Promise.resolve();
	await Promise.resolve();
}

describe("ChatWorkspace invalidation retry", () => {
	it("clears a failed invalidation after its retry refresh succeeds", async () => {
		const list = vi.fn().mockResolvedValueOnce({ conversations: [general] })
			.mockRejectedValueOnce(new Error("reload unavailable"))
			.mockResolvedValue({ conversations: [general] });
		const api = fakeApi({ listConversations: list });
		const socket = socketHarness();
		renderWorkspace(api, socket.factory);
		await ready();
		vi.useFakeTimers();

		act(() => socket.callbacks.onEvent({
			seq: 30, type: "conversation.member_added", conversation_id: 2, entity_id: 1, payload: [2, 1],
		}));
		await act(settle);
		expect(list).toHaveBeenCalledTimes(2);
		expect(document.body.textContent).toContain("reload unavailable");

		await act(async () => { await vi.advanceTimersByTimeAsync(500); });
		expect(list).toHaveBeenCalledTimes(3);
		expect(document.body.textContent).not.toContain("reload unavailable");
		expect(document.querySelector("#message-composer")).not.toBeNull();
	});

	it("cancels a failed invalidation retry when unmounted", async () => {
		const list = vi.fn().mockResolvedValueOnce({ conversations: [general] })
			.mockRejectedValue(new Error("reload unavailable"));
		const api = fakeApi({ listConversations: list });
		const socket = socketHarness();
		const view = renderWorkspace(api, socket.factory);
		await ready();
		vi.useFakeTimers();

		act(() => socket.callbacks.onEvent({
			seq: 31, type: "conversation.member_removed", conversation_id: 2, entity_id: 1, payload: [2, 1],
		}));
		await act(settle);
		expect(list).toHaveBeenCalledTimes(2);
		view.unmount();
		await vi.advanceTimersByTimeAsync(30_000);
		expect(list).toHaveBeenCalledTimes(2);
	});
});
