import { fireEvent, render, screen, waitFor } from "@testing-library/preact";
import { act } from "preact/test-utils";
import { beforeEach, describe, expect, it, vi } from "vitest";

import type { ApiClient } from "./api/client";
import type { MessageResult } from "./api/types";
import { SessionProvider } from "./auth/session";
import { ChatWorkspace, type WorkspaceSocketFactory } from "./chat-workspace";
import type { SocketCallbacks } from "./realtime/socket";

const user = { id: 1, username: "ada", admin: true, created_at: "2026-08-25T10:00:00Z" };
const general = { id: 2, kind: "channel" as const, name: "general", created_by: 1, created_at: "2026-08-25T10:00:00Z" };
const research = { id: 1, kind: "channel" as const, name: "research", created_by: 1, created_at: "2026-08-25T09:00:00Z" };
const baseMessage = { id: 5, conversation_id: 2, author_id: 1, body: "general note", rendered_body: "<p>general note</p>", created_at: "2026-08-25T10:05:00Z" };
const researchMessage = { ...baseMessage, id: 3, conversation_id: 1, body: "research note", rendered_body: "<p>research note</p>" };

function deferred<T>() {
	let resolve!: (value: T) => void;
	let reject!: (error: unknown) => void;
	const promise = new Promise<T>((done, fail) => { resolve = done; reject = fail; });
	return { promise, resolve, reject };
}

function fakeApi(overrides: Record<string, unknown> = {}) {
	return {
		getSession: vi.fn().mockResolvedValue({ user, expires_at: "2026-09-25T10:00:00Z" }),
		logout: vi.fn().mockResolvedValue(undefined),
		listConversations: vi.fn().mockResolvedValue({ conversations: [general, research] }),
		conversation: vi.fn((id: number) => Promise.resolve(id === 2 ? general : research)),
		members: vi.fn().mockResolvedValue({ members: [{ user_id: 1, username: "ada", joined_at: user.created_at }] }),
		history: vi.fn((id: number) => Promise.resolve({ messages: id === 2 ? [baseMessage] : [researchMessage] })),
		createChannel: vi.fn(), createDM: vi.fn(),
		sendMessage: vi.fn(), editMessage: vi.fn(), deleteMessage: vi.fn(),
		...overrides,
	} as Record<string, ReturnType<typeof vi.fn>>;
}

function socketHarness() {
	let callbacks!: SocketCallbacks;
	const factory: WorkspaceSocketFactory = (next) => {
		callbacks = next;
		return { start: vi.fn(), stop: vi.fn() };
	};
	return { factory, get callbacks() { return callbacks; } };
}

function renderWorkspace(api: ReturnType<typeof fakeApi>, factory: WorkspaceSocketFactory) {
	const client = api as unknown as ApiClient;
	return render(<SessionProvider api={client}><ChatWorkspace api={client} socketFactory={factory} /></SessionProvider>);
}

async function ready() {
	await screen.findByText("general note");
}

beforeEach(() => {
	Object.defineProperty(window, "matchMedia", { configurable: true, value: vi.fn(() => ({
		matches: false, media: "", addEventListener: vi.fn(), removeEventListener: vi.fn(),
	})) });
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
				fireEvent.click(screen.getByRole("button", { name: "Edit message" }));
				fireEvent.input(screen.getByLabelText("Edit message text"), { target: { value: "late edit" } });
				fireEvent.click(screen.getByRole("button", { name: "Save edit" }));
			} else fireEvent.click(screen.getByRole("button", { name: "Delete message" }));
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
