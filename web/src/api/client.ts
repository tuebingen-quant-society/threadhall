import type {
	Conversation,
	ConversationKind,
	ConversationPage,
	MemberPage,
	MessagePage,
	MessageResult,
	ProblemShape,
	Session,
} from "./types";

type Fetcher = (input: RequestInfo | URL, init?: RequestInit) => Promise<Response>;

const API_ROOT = "/api/v1";

export class ApiProblem extends Error implements ProblemShape {
	constructor(public status: number, public code: string, public detail: string) {
		super(detail);
		this.name = "ApiProblem";
	}
}

function csrfToken() {
	const prefix = "threadhall_csrf=";
	const value = document.cookie.split(";").map((part) => part.trim()).find((part) => part.startsWith(prefix));
	return value ? decodeURIComponent(value.slice(prefix.length)) : "";
}

async function responseProblem(response: Response): Promise<ApiProblem> {
	try {
		const value = await response.json() as Partial<ProblemShape>;
		if (typeof value.status === "number" && typeof value.code === "string" && typeof value.detail === "string") {
			return new ApiProblem(value.status, value.code, value.detail);
		}
	} catch {
		// The stable client error below deliberately hides malformed server bodies.
	}
	return new ApiProblem(response.status, "invalid_response", "The server returned an invalid error response.");
}

export function errorDetail(error: unknown) {
	return error instanceof Error ? error.message : "The request could not be completed.";
}

export class ApiClient {
	constructor(private readonly fetcher: Fetcher = fetch) {}

	private async request<T>(path: string, init: RequestInit = {}, signal?: AbortSignal): Promise<T> {
		const method = init.method ?? "GET";
		const headers: Record<string, string> = { Accept: "application/json" };
		if (init.body !== undefined) headers["Content-Type"] = "application/json";
		if (method !== "GET" && method !== "HEAD") headers["X-CSRF-Token"] = csrfToken();
		let response: Response;
		try {
			response = await this.fetcher(`${API_ROOT}${path}`, {
				...init,
				credentials: "same-origin",
				headers: { ...headers, ...(init.headers as Record<string, string> | undefined) },
				signal,
			});
		} catch (error) {
			if (error instanceof DOMException && error.name === "AbortError") throw error;
			throw new ApiProblem(0, "network_error", "Threadhall could not reach the server.");
		}
		if (!response.ok) throw await responseProblem(response);
		if (response.status === 204) return undefined as T;
		try {
			return await response.json() as T;
		} catch {
			throw new ApiProblem(response.status, "invalid_response", "The server returned an invalid response.");
		}
	}

	getSession(signal?: AbortSignal) { return this.request<Session>("/session", {}, signal); }
	login(username: string, password: string, signal?: AbortSignal) {
		return this.request<Session>("/session", { method: "POST", body: JSON.stringify({ username, password }) }, signal);
	}
	register(inviteToken: string, username: string, password: string, signal?: AbortSignal) {
		return this.request<Session>("/users", {
			method: "POST", body: JSON.stringify({ invite_token: inviteToken, username, password }),
		}, signal);
	}
	logout(signal?: AbortSignal) { return this.request<void>("/session", { method: "DELETE" }, signal); }

	listConversations(signal?: AbortSignal) {
		return this.request<ConversationPage>("/conversations?limit=100", {}, signal);
	}
	conversation(id: number, signal?: AbortSignal) {
		return this.request<Conversation>(`/conversations/${id}`, {}, signal);
	}
	members(id: number, signal?: AbortSignal) {
		return this.request<MemberPage>(`/conversations/${id}/members?limit=100`, {}, signal);
	}
	createChannel(kind: Exclude<ConversationKind, "dm">, name: string, key: string, signal?: AbortSignal) {
		return this.request<Conversation>("/conversations", {
			method: "POST", body: JSON.stringify({ kind, name, idempotency_key: key }),
		}, signal);
	}
	createDM(otherUserId: number, key: string, signal?: AbortSignal) {
		return this.request<Conversation>("/conversations", {
			method: "POST", body: JSON.stringify({ kind: "dm", other_user_id: otherUserId, idempotency_key: key }),
		}, signal);
	}

	history(conversationId: number, signal?: AbortSignal, beforeId?: number) {
		const before = beforeId === undefined ? "" : `&before_id=${beforeId}`;
		return this.request<MessagePage>(`/conversations/${conversationId}/messages?limit=100${before}`, {}, signal);
	}
	sendMessage(conversationId: number, body: string, key: string, signal?: AbortSignal) {
		return this.request<MessageResult>(`/conversations/${conversationId}/messages`, {
			method: "POST", body: JSON.stringify({ body, idempotency_key: key }),
		}, signal);
	}
	editMessage(messageId: number, body: string, key: string, signal?: AbortSignal) {
		return this.request<MessageResult>(`/messages/${messageId}`, {
			method: "PATCH", body: JSON.stringify({ body, idempotency_key: key }),
		}, signal);
	}
	deleteMessage(messageId: number, key: string, signal?: AbortSignal) {
		return this.request<MessageResult>(`/messages/${messageId}`, {
			method: "DELETE", body: JSON.stringify({ idempotency_key: key }),
		}, signal);
	}
}
