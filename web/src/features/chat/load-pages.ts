import type { ApiClient } from "../../api/client";
import { appendConversationPage, appendMemberPage, MAX_CLIENT_CONVERSATIONS, MAX_CLIENT_MEMBERS } from "../conversations/collection";

export async function loadConversationPages(api: ApiClient, target: number, signal: AbortSignal) {
	let result = { items: [] as Awaited<ReturnType<typeof api.listConversations>>["conversations"], nextBeforeId: undefined as number | undefined };
	do {
		const page = await api.listConversations(signal, result.nextBeforeId);
		result = appendConversationPage(result.items, page);
	} while (result.nextBeforeId !== undefined && result.items.length < Math.min(target, MAX_CLIENT_CONVERSATIONS));
	return result;
}

export async function loadMemberPages(api: ApiClient, conversationId: number, target: number, signal: AbortSignal) {
	let result = { items: [] as Awaited<ReturnType<typeof api.members>>["members"], nextBeforeId: undefined as number | undefined };
	do {
		const page = await api.members(conversationId, signal, result.nextBeforeId);
		result = appendMemberPage(result.items, page);
	} while (result.nextBeforeId !== undefined && result.items.length < Math.min(target, MAX_CLIENT_MEMBERS));
	return result;
}
