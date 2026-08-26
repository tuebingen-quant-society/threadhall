import { render, screen } from "@testing-library/preact";
import { vi } from "vitest";

import type { ApiClient } from "./api/client";
import { SessionProvider } from "./auth/session";
import { ChatWorkspace, type WorkspaceSocketFactory } from "./chat-workspace";
import type { SocketCallbacks } from "./realtime/socket";

export const user = { id: 1, username: "ada", admin: true, created_at: "2026-08-25T10:00:00Z" };
export const general = { id: 2, kind: "channel" as const, name: "general", created_by: 1, created_at: "2026-08-25T10:00:00Z" };
export const research = { id: 1, kind: "channel" as const, name: "research", created_by: 1, created_at: "2026-08-25T09:00:00Z" };
export const baseMessage = { id: 5, conversation_id: 2, author_id: 1, body: "general note", rendered_body: "<p>general note</p>", created_at: "2026-08-25T10:05:00Z" };
export const researchMessage = { ...baseMessage, id: 3, conversation_id: 1, body: "research note", rendered_body: "<p>research note</p>" };

export function deferred<T>() {
	let resolve!: (value: T) => void;
	let reject!: (error: unknown) => void;
	const promise = new Promise<T>((done, fail) => { resolve = done; reject = fail; });
	return { promise, resolve, reject };
}

export function fakeApi(overrides: Record<string, unknown> = {}) {
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

export function socketHarness() {
	let callbacks!: SocketCallbacks;
	const factory: WorkspaceSocketFactory = (next) => {
		callbacks = next;
		return { start: vi.fn(), stop: vi.fn() };
	};
	return { factory, get callbacks() { return callbacks; } };
}

export function renderWorkspace(api: ReturnType<typeof fakeApi>, factory: WorkspaceSocketFactory) {
	const client = api as unknown as ApiClient;
	return render(<SessionProvider api={client}><ChatWorkspace api={client} socketFactory={factory} /></SessionProvider>);
}

export async function ready() {
	await screen.findByText("general note");
}

export function installMatchMedia() {
	Object.defineProperty(window, "matchMedia", { configurable: true, value: vi.fn(() => ({
		matches: false, media: "", addEventListener: vi.fn(), removeEventListener: vi.fn(),
	})) });
}
