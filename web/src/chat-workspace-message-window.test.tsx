import { fireEvent, screen, waitFor } from "@testing-library/preact";
import { act } from "preact/test-utils";
import { beforeEach, describe, expect, it, vi } from "vitest";

import type { Message } from "./api/types";
import { baseMessage, deferred, fakeApi, installMatchMedia, renderWorkspace, socketHarness } from "./chat-workspace-test-utils";

beforeEach(installMatchMedia);

function message(id: number, body = `message ${id}`): Message {
	return { ...baseMessage, id, body, rendered_body: `<p>${body}</p>` };
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
});
