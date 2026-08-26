import { fireEvent, screen, waitFor } from "@testing-library/preact";
import { act } from "preact/test-utils";
import { beforeEach, describe, expect, it, vi } from "vitest";

import { ApiProblem } from "./api/client";
import { baseMessage, deferred, fakeApi, general, installMatchMedia, ready, renderWorkspace, research, socketHarness } from "./chat-workspace-test-utils";

beforeEach(installMatchMedia);

const pagedGeneral = { ...general, id: 200 };
const fullPage = [pagedGeneral, ...Array.from({ length: 99 }, (_, index) => ({ ...general, id: 199 - index, name: `channel-${199 - index}` }))];

function pagedApi(listConversations: ReturnType<typeof vi.fn>) {
	return fakeApi({
		listConversations,
		conversation: vi.fn((id: number) => Promise.resolve(id === 200 ? pagedGeneral : research)),
		history: vi.fn((id: number) => Promise.resolve({ messages: id === 200 ? [{ ...baseMessage, conversation_id: 200 }] : [] })),
	});
}

describe("ChatWorkspace list coordination", () => {
	it("lets a refresh finish before starting requested pagination", async () => {
		const refresh = deferred<{ conversations: typeof fullPage; next_before_id?: number }>();
		const pagination = deferred<{ conversations: (typeof research)[]; next_before_id?: number }>();
		let calls = 0;
		const list = vi.fn((signal: AbortSignal, before?: number) => {
			calls += 1;
			if (calls === 1) return Promise.resolve({ conversations: fullPage, next_before_id: 100 });
			if (before !== undefined) return pagination.promise;
			return refresh.promise;
		});
		const api = pagedApi(list);
		const socket = socketHarness();
		renderWorkspace(api, socket.factory);
		await ready();

		act(() => socket.callbacks.onEvent({ seq: 20, type: "conversation.member_added", conversation_id: 200, entity_id: 1, payload: [200, 1] }));
		await waitFor(() => expect(list).toHaveBeenCalledTimes(2));
		const refreshSignal = list.mock.calls[1][0];
		fireEvent.click(screen.getByRole("button", { name: "Load more conversations" }));

		expect(refreshSignal.aborted).toBe(false);
		expect(list).toHaveBeenCalledTimes(2);
		act(() => refresh.resolve({ conversations: fullPage, next_before_id: 100 }));
		await waitFor(() => expect(list).toHaveBeenCalledTimes(3));
		act(() => pagination.resolve({ conversations: [research] }));
		await screen.findByRole("button", { name: "research, public channel" });
	});

	it("aborts pagination for refresh and always clears its Loading state", async () => {
		let paginationSignal: AbortSignal | undefined;
		let calls = 0;
		const list = vi.fn((signal: AbortSignal, before?: number) => {
			calls += 1;
			if (calls === 1) return Promise.resolve({ conversations: fullPage, next_before_id: 100 });
			if (before !== undefined) {
				paginationSignal = signal;
				return new Promise((_, reject) => signal.addEventListener("abort", () => reject(new DOMException("aborted", "AbortError")), { once: true }));
			}
			return Promise.resolve({ conversations: fullPage, next_before_id: 100 });
		});
		const api = pagedApi(list);
		const socket = socketHarness();
		renderWorkspace(api, socket.factory);
		await ready();

		fireEvent.click(screen.getByRole("button", { name: "Load more conversations" }));
		await screen.findByRole("button", { name: "Loading…" });
		act(() => socket.callbacks.onEvent({ seq: 21, type: "conversation.member_added", conversation_id: 200, entity_id: 1, payload: [200, 1] }));

		await waitFor(() => expect(paginationSignal?.aborted).toBe(true));
		await screen.findByRole("button", { name: "Load more conversations" });
	});

	it("retries an invalidation refresh made stale by a selection change", async () => {
		const stale = deferred<{ conversations: (typeof general | typeof research)[] }>();
		const list = vi.fn().mockResolvedValueOnce({ conversations: [general, research] })
			.mockReturnValueOnce(stale.promise).mockResolvedValue({ conversations: [general, research] });
		const api = fakeApi({ listConversations: list });
		const socket = socketHarness();
		renderWorkspace(api, socket.factory);
		await ready();

		act(() => socket.callbacks.onEvent({ seq: 22, type: "conversation.member_removed", conversation_id: 2, entity_id: 1, payload: [2, 1] }));
		await waitFor(() => expect(list).toHaveBeenCalledTimes(2));
		fireEvent.click(screen.getByRole("button", { name: "research, public channel" }));
		await screen.findByText("research note");
		act(() => stale.resolve({ conversations: [general, research] }));

		await waitFor(() => expect(list).toHaveBeenCalledTimes(3));
	});
});

describe("ChatWorkspace authoritative selection scope", () => {
	it("reloads initial history when metadata invalidation takes over its selection fetch", async () => {
		const initialHistory = deferred<{ messages: (typeof baseMessage)[] }>();
		const refreshedHistory = deferred<{ messages: (typeof baseMessage)[] }>();
		const invalidation = deferred<{ conversations: (typeof general | typeof research)[] }>();
		let initialSignal: AbortSignal | undefined;
		const list = vi.fn().mockResolvedValueOnce({ conversations: [general, research] }).mockReturnValueOnce(invalidation.promise);
		const history = vi.fn((_id: number, signal: AbortSignal) => {
			if (history.mock.calls.length === 1) { initialSignal = signal; return initialHistory.promise; }
			return refreshedHistory.promise;
		});
		const api = fakeApi({ history, listConversations: list });
		const socket = socketHarness();
		renderWorkspace(api, socket.factory);
		await waitFor(() => expect(history).toHaveBeenCalledTimes(1));

		act(() => socket.callbacks.onEvent({ seq: 24, type: "conversation.member_added", conversation_id: 2, entity_id: 1, payload: [2, 1] }));
		await waitFor(() => expect(list).toHaveBeenCalledTimes(2));
		act(() => invalidation.resolve({ conversations: [general, research] }));
		await waitFor(() => expect(history).toHaveBeenCalledTimes(2));
		expect(initialSignal?.aborted).toBe(true);
		act(() => refreshedHistory.resolve({ messages: [baseMessage] }));

		await screen.findByText("general note");
		await waitFor(() => expect(screen.getByLabelText("Message history").getAttribute("aria-busy")).toBe("false"));
	});

	it("does not treat a list-level not-found as a successful selected removal", async () => {
		const list = vi.fn().mockResolvedValueOnce({ conversations: [general] })
			.mockRejectedValueOnce(new ApiProblem(404, "not_found", "list unavailable"));
		const api = fakeApi({ listConversations: list });
		const socket = socketHarness();
		renderWorkspace(api, socket.factory);
		await ready();

		await expect(socket.callbacks.onResync?.()).rejects.toThrow("list unavailable");
		expect(screen.getByLabelText("Message general")).toBeTruthy();
		expect((await screen.findAllByText("Resync failed: list unavailable")).length).toBeGreaterThan(0);
	});

	it("does not let a delayed A not-found response clear selected B", async () => {
		const staleDetail = deferred<typeof general>();
		let generalCalls = 0;
		const conversation = vi.fn((id: number) => {
			if (id === 1) return Promise.resolve(research);
			generalCalls += 1;
			return generalCalls === 1 ? Promise.resolve(general) : staleDetail.promise;
		});
		const api = fakeApi({ conversation });
		const socket = socketHarness();
		renderWorkspace(api, socket.factory);
		await ready();

		act(() => socket.callbacks.onEvent({ seq: 23, type: "conversation.member_removed", conversation_id: 2, entity_id: 1, payload: [2, 1] }));
		await waitFor(() => expect(conversation).toHaveBeenCalledTimes(2));
		fireEvent.click(screen.getByRole("button", { name: "research, public channel" }));
		await screen.findByText("research note");
		act(() => staleDetail.reject(new ApiProblem(404, "not_found", "conversation not found")));

		await waitFor(() => expect(screen.getByLabelText("Message research")).toBeTruthy());
		expect(screen.queryByText("Select a conversation")).toBeNull();
	});

	it("rejects resync when selection changes before its refresh completes", async () => {
		const resyncList = deferred<{ conversations: (typeof general | typeof research)[] }>();
		const list = vi.fn().mockResolvedValueOnce({ conversations: [general, research] }).mockReturnValueOnce(resyncList.promise);
		const api = fakeApi({ listConversations: list });
		const socket = socketHarness();
		renderWorkspace(api, socket.factory);
		await ready();

		const resync = socket.callbacks.onResync?.();
		await waitFor(() => expect(list).toHaveBeenCalledTimes(2));
		fireEvent.click(screen.getByRole("button", { name: "research, public channel" }));
		await screen.findByText("research note");
		act(() => resyncList.resolve({ conversations: [general, research] }));

		await expect(resync).rejects.toThrow();
		expect(screen.getByLabelText("Message research")).toBeTruthy();
	});

	it("aborts the active selection fetch when resync establishes its scope", async () => {
		const initialHistory = deferred<{ messages: [] }>();
		let firstSignal: AbortSignal | undefined;
		const history = vi.fn((_id: number, signal: AbortSignal) => {
			if (history.mock.calls.length === 1) { firstSignal = signal; return initialHistory.promise; }
			return Promise.resolve({ messages: [] });
		});
		const api = fakeApi({ history });
		const socket = socketHarness();
		renderWorkspace(api, socket.factory);
		await waitFor(() => expect(history).toHaveBeenCalledTimes(1));

		const resync = socket.callbacks.onResync?.();
		expect(firstSignal?.aborted).toBe(true);
		await resync;
	});
});
