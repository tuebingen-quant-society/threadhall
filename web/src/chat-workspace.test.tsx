import { fireEvent, screen, waitFor } from "@testing-library/preact";
import { act } from "preact/test-utils";
import { beforeEach, describe, expect, it, vi } from "vitest";

import type { MessageResult } from "./api/types";
import { baseMessage, deferred, fakeApi, general, installMatchMedia, ready, renderWorkspace, research, researchMessage, socketHarness, user } from "./chat-workspace-test-utils";

beforeEach(() => {
	installMatchMedia();
});

describe("ChatWorkspace history races", () => {
	it("fills deferred history without replacing newer socket and HTTP entities", async () => {
		const initial = deferred<{ messages: (typeof baseMessage)[]; next_before_id?: number }>();
		const socketLive = { ...baseMessage, id: 6, body: "socket live", rendered_body: "<p>socket live</p>" };
		const httpLive = { ...baseMessage, id: 7, body: "HTTP live", rendered_body: "<p>HTTP live</p>" };
		const api = fakeApi({
			history: vi.fn().mockReturnValue(initial.promise),
			sendMessage: vi.fn().mockResolvedValue({
				message: httpLive,
				event: { seq: 9, type: "message.sent", conversation_id: 2, entity_id: 7, payload: {
					author_id: 1, body: httpLive.body, rendered_body: httpLive.rendered_body, created_at: httpLive.created_at,
				} },
			}),
		});
		const socket = socketHarness();
		renderWorkspace(api, socket.factory);
		await waitFor(() => expect(api.history).toHaveBeenCalled());

		act(() => socket.callbacks.onEvent({
			seq: 8, type: "message.sent", conversation_id: 2, entity_id: 6,
			payload: { author_id: 1, body: socketLive.body, rendered_body: socketLive.rendered_body, created_at: socketLive.created_at },
		}));
		const composer = await screen.findByLabelText("Message general");
		fireEvent.input(composer, { target: { value: "HTTP live" } });
		fireEvent.keyDown(composer, { key: "Enter" });
		await screen.findByText("HTTP live");

		initial.resolve({ messages: [
			{ ...socketLive, body: "stale socket", rendered_body: "<p>stale socket</p>" },
			{ ...httpLive, body: "stale HTTP", rendered_body: "<p>stale HTTP</p>" },
			{ ...baseMessage, id: 4, body: "older history", rendered_body: "<p>older history</p>" },
		] });

		await screen.findByText("older history");
		expect(screen.getByText("socket live")).toBeTruthy();
		expect(screen.getByText("HTTP live")).toBeTruthy();
		expect(screen.queryByText("stale socket")).toBeNull();
		expect(screen.queryByText("stale HTTP")).toBeNull();
	});

	it("base-merges older history without reverting live entities", async () => {
		const older = deferred<{ messages: (typeof baseMessage)[] }>();
		const socketLive = { ...baseMessage, body: "socket edit", rendered_body: "<p>socket edit</p>", edited_at: "2026-08-25T10:08:00Z" };
		const httpLive = { ...baseMessage, id: 7, body: "HTTP during older", rendered_body: "<p>HTTP during older</p>" };
		const history = vi.fn((_id: number, _signal: AbortSignal, before?: number) => before === 5
			? older.promise : Promise.resolve({ messages: [baseMessage], next_before_id: 5 }));
		const api = fakeApi({
			history,
			sendMessage: vi.fn().mockResolvedValue({
				message: httpLive,
				event: { seq: 9, type: "message.sent", conversation_id: 2, entity_id: 7, payload: {
					author_id: 1, body: httpLive.body, rendered_body: httpLive.rendered_body, created_at: httpLive.created_at,
				} },
			}),
		});
		const socket = socketHarness();
		renderWorkspace(api, socket.factory);
		await ready();
		fireEvent.click(screen.getByRole("button", { name: "Load earlier messages" }));
		await waitFor(() => expect(history).toHaveBeenCalledWith(2, expect.any(AbortSignal), 5));

		act(() => socket.callbacks.onEvent({
			seq: 8, type: "message.edited", conversation_id: 2, entity_id: 5,
			payload: { body: socketLive.body, rendered_body: socketLive.rendered_body, edited_at: socketLive.edited_at },
		}));
		const composer = screen.getByLabelText("Message general");
		fireEvent.input(composer, { target: { value: httpLive.body } });
		fireEvent.keyDown(composer, { key: "Enter" });
		await screen.findByText(httpLive.body);
		act(() => older.resolve({ messages: [
			{ ...baseMessage, body: "stale edit", rendered_body: "<p>stale edit</p>" },
			{ ...httpLive, body: "stale HTTP", rendered_body: "<p>stale HTTP</p>" },
			{ ...baseMessage, id: 4, body: "older page", rendered_body: "<p>older page</p>" },
		] }));

		await screen.findByText("older page");
		expect(screen.getByText(socketLive.body)).toBeTruthy();
		expect(screen.getByText(httpLive.body)).toBeTruthy();
		expect(screen.queryByText("stale edit")).toBeNull();
		expect(screen.queryByText("stale HTTP")).toBeNull();
	});
});

describe("ChatWorkspace send resync races", () => {
	it("keeps a rejected in-flight send retryable under the same key across resync", async () => {
		const firstAttempt = deferred<MessageResult>();
		const committed = { ...baseMessage, id: 8, body: "retry me", rendered_body: "<p>retry me</p>" };
		const sendMessage = vi.fn().mockReturnValueOnce(firstAttempt.promise).mockResolvedValueOnce({
			message: committed,
			event: { seq: 10, type: "message.sent", conversation_id: 2, entity_id: 8, payload: {
				author_id: 1, body: committed.body, rendered_body: committed.rendered_body, created_at: committed.created_at,
			} },
		});
		const api = fakeApi({ sendMessage });
		const socket = socketHarness();
		renderWorkspace(api, socket.factory);
		await ready();

		const composer = screen.getByLabelText("Message general") as HTMLTextAreaElement;
		fireEvent.input(composer, { target: { value: "retry me" } });
		fireEvent.keyDown(composer, { key: "Enter" });
		await waitFor(() => expect(sendMessage).toHaveBeenCalledTimes(1));
		const resync = socket.callbacks.onResync?.();
		expect(screen.getByText("Sending…")).toBeTruthy();
		await resync;
		expect(screen.getByText("Sending…")).toBeTruthy();

		act(() => firstAttempt.reject(new Error("delivery uncertain")));
		await screen.findByRole("alert");
		expect(composer.value).toBe("retry me");
		fireEvent.keyDown(composer, { key: "Enter" });
		await screen.findByText("retry me");

		expect(sendMessage.mock.calls[1][2]).toBe(sendMessage.mock.calls[0][2]);
	});

	it("merges a committed in-flight send after authoritative resync", async () => {
		const attempt = deferred<MessageResult>();
		const committed = { ...baseMessage, id: 9, body: "committed during resync", rendered_body: "<p>committed during resync</p>" };
		const api = fakeApi({ sendMessage: vi.fn().mockReturnValue(attempt.promise) });
		const socket = socketHarness();
		renderWorkspace(api, socket.factory);
		await ready();

		const composer = screen.getByLabelText("Message general");
		fireEvent.input(composer, { target: { value: committed.body } });
		fireEvent.keyDown(composer, { key: "Enter" });
		await socket.callbacks.onResync?.();
		act(() => attempt.resolve({
			message: committed,
			event: { seq: 11, type: "message.sent", conversation_id: 2, entity_id: 9, payload: {
				author_id: 1, body: committed.body, rendered_body: committed.rendered_body, created_at: committed.created_at,
			} },
		}));

		await screen.findByText("committed during resync");
		expect(screen.queryByText("Sending…")).toBeNull();
	});
});

describe("ChatWorkspace selection generations", () => {
	it("does not merge history from a conversation switched away during load", async () => {
		const oldHistory = deferred<{ messages: (typeof baseMessage)[] }>();
		const api = fakeApi({ history: vi.fn((id: number) => id === 2 ? oldHistory.promise : Promise.resolve({ messages: [researchMessage] })) });
		const socket = socketHarness();
		renderWorkspace(api, socket.factory);
		await waitFor(() => expect(api.history).toHaveBeenCalledWith(2, expect.any(AbortSignal)));

		fireEvent.click(screen.getByRole("button", { name: "research, public channel" }));
		expect(screen.queryByText("general note")).toBeNull();
		oldHistory.resolve({ messages: [baseMessage] });
		await screen.findByText("research note");
		expect(screen.queryByText("general note")).toBeNull();
	});

	for (const operation of ["send", "edit", "delete"] as const) {
		it(`ignores a stale ${operation} completion after selection changes`, async () => {
			const result = deferred<MessageResult>();
			const api = fakeApi({ [`${operation}Message`]: vi.fn().mockReturnValue(result.promise) });
			const socket = socketHarness();
			renderWorkspace(api, socket.factory);
			await ready();
			if (operation === "send") {
				const composer = screen.getByLabelText("Message general");
				fireEvent.input(composer, { target: { value: "late send" } });
				fireEvent.keyDown(composer, { key: "Enter" });
			} else if (operation === "edit") {
				fireEvent.click(screen.getByLabelText("Message actions"));
				fireEvent.click(screen.getByRole("button", { name: "Edit message" }));
				fireEvent.input(screen.getByLabelText("Edit message text"), { target: { value: "late edit" } });
				fireEvent.click(screen.getByRole("button", { name: "Save edit" }));
			} else {
				fireEvent.click(screen.getByLabelText("Message actions"));
				fireEvent.click(screen.getByRole("button", { name: "Delete message" }));
			}
			await waitFor(() => expect(api[`${operation}Message`]).toHaveBeenCalled());

			fireEvent.click(screen.getByRole("button", { name: "research, public channel" }));
			result.resolve({ message: { ...baseMessage, body: "stale result", rendered_body: "<p>stale result</p>" }, event: { seq: 9, type: `message.${operation}`, conversation_id: 2, entity_id: 5, payload: {} } });
			await screen.findByText("research note");
			expect(screen.queryByText("stale result")).toBeNull();
			expect(screen.queryByText("Sending…")).toBeNull();
		});
	}

	it("ignores an older-history page completed after switching", async () => {
		const older = deferred<{ messages: (typeof baseMessage)[] }>();
		const history = vi.fn((id: number, _signal: AbortSignal, before?: number) => before ? older.promise : Promise.resolve({ messages: id === 2 ? [baseMessage] : [researchMessage], next_before_id: id === 2 ? 5 : undefined }));
		const api = fakeApi({ history });
		const socket = socketHarness();
		renderWorkspace(api, socket.factory);
		await ready();
		fireEvent.click(screen.getByRole("button", { name: "Load earlier messages" }));
		await waitFor(() => expect(history).toHaveBeenCalledWith(2, expect.any(AbortSignal), 5));
		fireEvent.click(screen.getByRole("button", { name: "research, public channel" }));
		older.resolve({ messages: [{ ...baseMessage, id: 4, body: "stale older", rendered_body: "<p>stale older</p>" }] });
		await screen.findByText("research note");
		expect(screen.queryByText("stale older")).toBeNull();
	});

	it("ignores an older member page completed after switching", async () => {
		const older = deferred<{ members: { user_id: number; username: string; joined_at: string }[] }>();
		const members = vi.fn((id: number, _signal: AbortSignal, before?: number) => before ? older.promise : Promise.resolve({ members: [{ user_id: 1, username: "ada", joined_at: user.created_at }], next_before_id: id === 2 ? 1 : undefined }));
		const api = fakeApi({ members });
		const socket = socketHarness();
		renderWorkspace(api, socket.factory);
		await ready();
		fireEvent.click(screen.getByRole("button", { name: "Load more members" }));
		await waitFor(() => expect(members).toHaveBeenCalledWith(2, expect.any(AbortSignal), 1));
		fireEvent.click(screen.getByRole("button", { name: "research, public channel" }));
		older.resolve({ members: [{ user_id: 9, username: "stale-member", joined_at: user.created_at }] });
		await screen.findByText("research note");
		expect(screen.queryByText("stale-member")).toBeNull();
	});
});

describe("ChatWorkspace conversation invalidation", () => {
	it("coalesces bursts and clears a selected conversation removed by refresh", async () => {
		const invalidation = deferred<{ conversations: (typeof general)[] }>();
		const list = vi.fn().mockResolvedValueOnce({ conversations: [general] }).mockReturnValueOnce(invalidation.promise).mockResolvedValue({ conversations: [] });
		const api = fakeApi({ listConversations: list });
		const socket = socketHarness();
		renderWorkspace(api, socket.factory);
		await ready();

		act(() => { for (let index = 0; index < 100; index += 1) socket.callbacks.onEvent({ seq: index + 1, type: "conversation.member_removed", conversation_id: 2, entity_id: 2, payload: [2, 1] }); });
		expect(list).toHaveBeenCalledTimes(2);
		invalidation.resolve({ conversations: [general] });
		await waitFor(() => expect(list).toHaveBeenCalledTimes(3));
		await screen.findByText("Select a conversation");
		expect(screen.queryByLabelText("Message general")).toBeNull();
		expect(screen.queryByText("general note")).toBeNull();
	});

	it("rejects failed authoritative resync after showing the error", async () => {
		const list = vi.fn().mockResolvedValueOnce({ conversations: [general] }).mockRejectedValueOnce(new Error("reload unavailable"));
		const api = fakeApi({ listConversations: list });
		const socket = socketHarness();
		renderWorkspace(api, socket.factory);
		await ready();

		await expect(socket.callbacks.onResync?.()).rejects.toThrow("reload unavailable");
		expect((await screen.findAllByText("Resync failed: reload unavailable")).length).toBeGreaterThan(0);
	});
});
