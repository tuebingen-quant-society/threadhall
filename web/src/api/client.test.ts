import { beforeEach, describe, expect, it, vi } from "vitest";

import { ApiClient, ApiProblem } from "./client";

function response(body: unknown, status = 200, type = "application/json") {
	return new Response(body === undefined ? null : JSON.stringify(body), {
		status,
		headers: { "Content-Type": type },
	});
}

describe("ApiClient", () => {
	beforeEach(() => {
		document.cookie = "threadhall_csrf=csrf-token; path=/";
		vi.restoreAllMocks();
	});

	it("sends login and invite registration through the credentialed CSRF path", async () => {
		const fetcher = vi.fn()
			.mockResolvedValueOnce(response({ user: { id: 1, username: "ada", admin: true, created_at: "2026-08-25T12:00:00Z" }, expires_at: "2026-08-26T12:00:00Z" }))
			.mockResolvedValueOnce(response({ user: { id: 2, username: "lin", admin: false, created_at: "2026-08-25T12:00:00Z" }, expires_at: "2026-08-26T12:00:00Z" }, 201));
		const api = new ApiClient(fetcher);

		await api.login("ada", "correct horse battery staple");
		await api.register("invite-token", "lin", "correct horse battery staple");

		expect(fetcher).toHaveBeenCalledTimes(2);
		for (const [, init] of fetcher.mock.calls) {
			expect(init.credentials).toBe("same-origin");
			expect(init.headers["X-CSRF-Token"]).toBe("csrf-token");
		}
		expect(JSON.parse(fetcher.mock.calls[1][1].body).invite_token).toBe("invite-token");
	});

	it("creates public/private channels and canonical DMs with exact payloads", async () => {
		const fetcher = vi.fn().mockImplementation(() => Promise.resolve(response({ id: 7, kind: "channel", name: "research", created_by: 1, created_at: "2026-08-25T12:00:00Z" }, 201)));
		const api = new ApiClient(fetcher);

		await api.createChannel("private", "research", "channel-key");
		await api.createDM(42, "dm-key");

		expect(JSON.parse(fetcher.mock.calls[0][1].body)).toEqual({ kind: "private", name: "research", idempotency_key: "channel-key" });
		expect(JSON.parse(fetcher.mock.calls[1][1].body)).toEqual({ kind: "dm", other_user_id: 42, idempotency_key: "dm-key" });
	});

	it("maps stable server problems and message edits/deletes", async () => {
		const fetcher = vi.fn()
			.mockResolvedValueOnce(response({ status: 409, code: "idempotency_conflict", detail: "request conflicts with an earlier operation" }, 409, "application/problem+json"))
			.mockResolvedValueOnce(response({ message: { id: 9 }, event: { seq: 8 } }))
			.mockResolvedValueOnce(response({ message: { id: 9 }, event: { seq: 9 } }));
		const api = new ApiClient(fetcher);

		await expect(api.sendMessage(3, "hello", "duplicate-key")).rejects.toMatchObject({
			status: 409,
			code: "idempotency_conflict",
		});
		await api.editMessage(9, "changed", "edit-key");
		await api.deleteMessage(9, "delete-key");

		expect(ApiProblem.prototype).toBeInstanceOf(Error);
		expect(fetcher.mock.calls[1][1].method).toBe("PATCH");
		expect(fetcher.mock.calls[2][1].method).toBe("DELETE");
	});

	it("sends an optional linked reply reference", async () => {
		const fetcher = vi.fn().mockResolvedValue(response({ message: { id: 10 }, event: { seq: 10 } }, 201));
		const api = new ApiClient(fetcher);

		await api.sendMessage(3, "linked reply", "reply-key", undefined, 9);

		expect(JSON.parse(fetcher.mock.calls[0][1].body)).toEqual({
			body: "linked reply", idempotency_key: "reply-key", reply_to_message_id: 9,
		});
	});

	it("preserves conversation and member pagination cursors", async () => {
		const fetcher = vi.fn().mockImplementation(() => Promise.resolve(response({ conversations: [], members: [] })));
		const api = new ApiClient(fetcher);

		await api.listConversations(undefined, 80);
		await api.members(7, undefined, 40);

		expect(fetcher.mock.calls[0][0]).toBe("/api/v1/conversations?limit=100&before_id=80");
		expect(fetcher.mock.calls[1][0]).toBe("/api/v1/conversations/7/members?limit=100&before_id=40");
	});

	it("searches the bounded member directory with encoded usernames", async () => {
		const fetcher = vi.fn().mockResolvedValue(response({ users: [{ id: 2, username: "lin" }] }));
		const api = new ApiClient(fetcher);

		await expect(api.findUsers("lin & team")).resolves.toEqual({ users: [{ id: 2, username: "lin" }] });
		expect(fetcher.mock.calls[0][0]).toBe("/api/v1/users?query=lin%20%26%20team&limit=20");
	});

	it("loads thread replies and creates reference-based forks", async () => {
		const fetcher = vi.fn()
			.mockResolvedValueOnce(response({ root: { id: 7 }, replies: [] }))
			.mockResolvedValueOnce(response({ conversation: { id: 9, kind: "private", name: "follow-up" }, source_conversation_id: 3, source_root_message_id: 7 }, 201));
		const api = new ApiClient(fetcher);

		await api.thread(3, 7, undefined, 8);
		await api.forkConversation(3, 7, "private", "follow-up", "fork-key");

		expect(fetcher.mock.calls[0][0]).toBe("/api/v1/conversations/3/threads/7?limit=100&after_id=8");
		expect(JSON.parse(fetcher.mock.calls[1][1].body)).toEqual({ source_message_id: 7, kind: "private", name: "follow-up", idempotency_key: "fork-key" });
	});

	it("loads only the selected conversation's agent capabilities", async () => {
		const page = { capabilities: [{ kind: "plugin", id: "drive", name: "Google Drive", description: "Search files" }] };
		const fetcher = vi.fn().mockResolvedValue(response(page));

		await expect(new ApiClient(fetcher).capabilities(7)).resolves.toEqual(page);
		expect(fetcher.mock.calls[0][0]).toBe("/api/v1/conversations/7/capabilities");
	});

	it("invokes the fetch implementation without an ApiClient receiver", async () => {
		const receivers: unknown[] = [];
		const fetcher = function (this: unknown) {
			receivers.push(this);
			return Promise.resolve(response({ user: { id: 1 } }));
		};

		await new ApiClient(fetcher).getSession();

		expect(receivers).toEqual([undefined]);
	});
});
