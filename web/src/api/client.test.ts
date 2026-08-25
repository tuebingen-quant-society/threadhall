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
});
