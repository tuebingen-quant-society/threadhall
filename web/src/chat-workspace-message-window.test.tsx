import { fireEvent, screen, waitFor } from "@testing-library/preact";
import { act } from "preact/test-utils";
import { beforeEach, describe, expect, it, vi } from "vitest";

import type { Message } from "./api/types";
import { baseMessage, deferred, fakeApi, installMatchMedia, renderWorkspace, socketHarness } from "./chat-workspace-test-utils";

beforeEach(installMatchMedia);

function message(id: number, body = `message ${id}`): Message {
	return { ...baseMessage, id, body, rendered_body: `<p>${body}</p>` };
}

function changedMessage(operation: "edit" | "delete", id: number, body: string): Message {
	return operation === "edit"
		? { ...message(id, body), edited_at: "2026-08-25T10:09:00Z" }
		: { ...message(id, ""), rendered_body: "", deleted_at: "2026-08-25T10:09:00Z" };
}

function emitPatchOverflow(socket: ReturnType<typeof socketHarness>, operation: "edit" | "delete") {
	act(() => {
		for (let id = 1; id <= 201; id += 1) socket.callbacks.onEvent(operation === "edit" ? {
			seq: id, type: "message.edited", conversation_id: 2, entity_id: id,
			payload: { body: `edited ${id}`, rendered_body: `<p>edited ${id}</p>`, edited_at: "2026-08-25T10:09:00Z" },
		} : {
			seq: id, type: "message.deleted", conversation_id: 2, entity_id: id,
			payload: { deleted_at: "2026-08-25T10:09:00Z" },
		});
	});
}

describe("ChatWorkspace delayed message patches", () => {
	for (const operation of ["edit", "delete"] as const) {
		it(`${operation} wins over delayed initial history`, async () => {
			const initial = deferred<{ messages: Message[] }>();
			const api = fakeApi({ history: vi.fn().mockReturnValue(initial.promise) });
			const socket = socketHarness();
			renderWorkspace(api, socket.factory);
			await waitFor(() => expect(api.history).toHaveBeenCalledTimes(1));

			act(() => socket.callbacks.onEvent(operation === "edit" ? {
				seq: 8, type: "message.edited", conversation_id: 2, entity_id: 50,
				payload: { body: "edited before initial", rendered_body: "<p>edited before initial</p>", edited_at: "2026-08-25T10:08:00Z" },
			} : {
				seq: 8, type: "message.deleted", conversation_id: 2, entity_id: 50,
				payload: { deleted_at: "2026-08-25T10:08:00Z" },
			}));
			act(() => initial.resolve({ messages: [message(50, "stale initial")] }));

			if (operation === "edit") await screen.findByText("edited before initial");
			else await screen.findByText("Message deleted");
			expect(screen.queryByText("stale initial")).toBeNull();
		});

		it(`${operation} wins over a delayed older page`, async () => {
			const older = deferred<{ messages: Message[] }>();
			const history = vi.fn((_id: number, _signal: AbortSignal, before?: number) => before === 5
				? older.promise : Promise.resolve({ messages: [message(5)], next_before_id: 5 }));
			const api = fakeApi({ history });
			const socket = socketHarness();
			renderWorkspace(api, socket.factory);
			await screen.findByText("message 5");
			fireEvent.click(screen.getByRole("button", { name: "Load earlier messages" }));
			await waitFor(() => expect(history).toHaveBeenCalledWith(2, expect.any(AbortSignal), 5));

			act(() => socket.callbacks.onEvent(operation === "edit" ? {
				seq: 9, type: "message.edited", conversation_id: 2, entity_id: 4,
				payload: { body: "edited older", rendered_body: "<p>edited older</p>", edited_at: "2026-08-25T10:09:00Z" },
			} : {
				seq: 9, type: "message.deleted", conversation_id: 2, entity_id: 4,
				payload: { deleted_at: "2026-08-25T10:09:00Z" },
			}));
			act(() => older.resolve({ messages: [message(4, "stale older")] }));

			if (operation === "edit") await screen.findByText("edited older");
			else await screen.findByText("Message deleted");
			expect(screen.queryByText("stale older")).toBeNull();
		});
	}

	for (const operation of ["edit", "delete"] as const) {
		it(`${operation} overflow invalidates delayed initial history and refetches once`, async () => {
			const initial = deferred<{ messages: Message[] }>();
			const current = changedMessage(operation, 1, "edited 1");
			const history = vi.fn().mockReturnValueOnce(initial.promise).mockResolvedValue({ messages: [current] });
			const api = fakeApi({ history });
			const socket = socketHarness();
			renderWorkspace(api, socket.factory);
			await waitFor(() => expect(history).toHaveBeenCalledTimes(1));

			emitPatchOverflow(socket, operation);
			await waitFor(() => expect(history).toHaveBeenCalledTimes(2));
			act(() => initial.resolve({ messages: [message(1, "stale initial overflow")] }));

			if (operation === "edit") await screen.findByText("edited 1");
			else await screen.findByText("Message deleted");
			expect(screen.queryByText("stale initial overflow")).toBeNull();
			expect(history).toHaveBeenCalledTimes(2);
		});

		it(`${operation} overflow invalidates a delayed older page and keeps pagination usable`, async () => {
			const older = deferred<{ messages: Message[]; next_before_id?: number }>();
			const current = changedMessage(operation, 1, "edited 1");
			let olderCalls = 0;
			const history = vi.fn((_id: number, _signal: AbortSignal, before?: number) => {
				if (before === 500 && olderCalls++ === 0) return older.promise;
				if (before === 500) return Promise.resolve({ messages: [current], next_before_id: 1 });
				if (history.mock.calls.length === 1) return Promise.resolve({ messages: [message(500)], next_before_id: 500 });
				return Promise.resolve({ messages: [message(500), current], next_before_id: 500 });
			});
			const api = fakeApi({ history });
			const socket = socketHarness();
			renderWorkspace(api, socket.factory);
			await screen.findByText("message 500");
			fireEvent.click(screen.getByRole("button", { name: "Load earlier messages" }));
			await waitFor(() => expect(history).toHaveBeenCalledWith(2, expect.any(AbortSignal), 500));

			emitPatchOverflow(socket, operation);
			await waitFor(() => expect(history).toHaveBeenCalledTimes(3));
			act(() => older.resolve({ messages: [message(1, "stale older overflow")], next_before_id: 1 }));
			if (operation === "edit") await screen.findByText("edited 1");
			else await screen.findByText("Message deleted");
			expect(screen.queryByText("stale older overflow")).toBeNull();

			fireEvent.click(screen.getByRole("button", { name: "Load earlier messages" }));
			await waitFor(() => expect(history).toHaveBeenCalledTimes(4));
			expect(history.mock.calls[3][2]).toBe(500);
			expect(screen.queryByText("stale older overflow")).toBeNull();
		});
	}
});

describe("ChatWorkspace bounded message window", () => {
	it("shows each older page, advances cursors, deduplicates, and retains then-new realtime", async () => {
		const history = vi.fn((_id: number, _signal: AbortSignal, before?: number) => {
			if (before === undefined) return Promise.resolve({ messages: Array.from({ length: 100 }, (_, index) => message(300 - index)), next_before_id: 201 });
			if (before === 201) return Promise.resolve({ messages: Array.from({ length: 100 }, (_, index) => message(200 - index)), next_before_id: 101 });
			return Promise.resolve({ messages: [message(101), ...Array.from({ length: 99 }, (_, index) => message(100 - index))], next_before_id: 2 });
		});
		const api = fakeApi({ history });
		const socket = socketHarness();
		renderWorkspace(api, socket.factory);
		await screen.findByText("message 300");

		fireEvent.click(screen.getByRole("button", { name: "Load earlier messages" }));
		await screen.findByText("message 101");
		fireEvent.click(screen.getByRole("button", { name: "Load earlier messages" }));
		await screen.findByText("message 2");

		expect(history.mock.calls.map((call) => call[2])).toEqual([undefined, 201, 101]);
		expect(document.querySelectorAll("[data-message-id]")).toHaveLength(200);
		expect(new Set([...document.querySelectorAll("[data-message-id]")].map((row) => row.getAttribute("data-message-id"))).size).toBe(200);
		expect(screen.queryByText("message 300")).toBeNull();

		act(() => socket.callbacks.onEvent({
			seq: 20, type: "message.sent", conversation_id: 2, entity_id: 301,
			payload: { author_id: 1, body: "realtime 301", rendered_body: "<p>realtime 301</p>", created_at: baseMessage.created_at },
		}));
		await screen.findByText("realtime 301");
		expect(screen.getByText("message 2")).toBeTruthy();
		expect(document.querySelectorAll("[data-message-id]")).toHaveLength(200);
	});

	it("retains realtime across a delayed full older window and continued pagination", async () => {
		const delayed = deferred<{ messages: Message[]; next_before_id: number }>();
		const history = vi.fn((_id: number, _signal: AbortSignal, before?: number) => {
			if (before === undefined) return Promise.resolve({ messages: Array.from({ length: 100 }, (_, index) => message(600 - index)), next_before_id: 501 });
			if (before === 501) return Promise.resolve({ messages: Array.from({ length: 100 }, (_, index) => message(500 - index)), next_before_id: 401 });
			if (before === 401) return delayed.promise;
			return Promise.resolve({ messages: [message(301), ...Array.from({ length: 99 }, (_, index) => message(300 - index))], next_before_id: 202 });
		});
		const api = fakeApi({ history });
		const socket = socketHarness();
		renderWorkspace(api, socket.factory);
		await screen.findByText("message 600");
		fireEvent.click(screen.getByRole("button", { name: "Load earlier messages" }));
		await screen.findByText("message 401");
		fireEvent.click(screen.getByRole("button", { name: "Load earlier messages" }));
		await waitFor(() => expect(history).toHaveBeenCalledWith(2, expect.any(AbortSignal), 401));

		act(() => socket.callbacks.onEvent({
			seq: 700, type: "message.sent", conversation_id: 2, entity_id: 601,
			payload: { author_id: 1, body: "realtime 601", rendered_body: "<p>realtime 601</p>", created_at: baseMessage.created_at },
		}));
		act(() => delayed.resolve({ messages: Array.from({ length: 100 }, (_, index) => message(400 - index)), next_before_id: 301 }));

		await screen.findByText("message 301");
		expect(screen.getByText("realtime 601")).toBeTruthy();
		expect(document.querySelectorAll("[data-message-id]")).toHaveLength(200);
		fireEvent.click(screen.getByRole("button", { name: "Load earlier messages" }));
		await screen.findByText("message 202");
		expect(history.mock.calls.map((call) => call[2])).toEqual([undefined, 501, 401, 301]);
		expect(screen.getByText("realtime 601")).toBeTruthy();
		expect(document.querySelectorAll("[data-message-id]")).toHaveLength(200);
		expect(new Set([...document.querySelectorAll("[data-message-id]")].map((row) => row.getAttribute("data-message-id"))).size).toBe(200);
	});
});
