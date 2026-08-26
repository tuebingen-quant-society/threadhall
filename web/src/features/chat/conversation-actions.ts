import type { ApiClient } from "../../api/client";
import type { Conversation } from "../../api/types";
import type { ConversationDraft } from "../conversations/create";
import { ListCoordinator, staleRequest } from "./list-coordinator";
import { loadConversationPages } from "./load-pages";

const key = (operation: string) => `${operation}-${crypto.randomUUID()}`;

export function conversationActions(
	api: ApiClient,
	lists: ListCoordinator,
	current: () => Conversation[],
	replace: (items: Conversation[], cursor?: number) => void,
	select: (id?: number) => void,
	cursor: () => number | undefined,
	setError: (value: string) => void,
) {
	async function create(draft: ConversationDraft) {
		setError("");
		const created = draft.kind === "dm" ? await api.createDM(draft.otherUserId, key("dm"))
			: await api.createChannel(draft.kind, draft.name, key("channel"), draft.kind === "private" ? draft.memberIds : []);
		await lists.refresh(async (ticket) => {
			const result = await loadConversationPages(api, Math.max(100, current().length), ticket.controller.signal);
			if (ticket.controller.signal.aborted) throw staleRequest();
			replace(result.items, result.nextBeforeId); select(created.id);
		});
	}

	async function remove(id?: number) {
		if (id === undefined) return;
		setError("");
		await api.deleteConversation(id);
		const items = current().filter((item) => item.id !== id);
		replace(items, cursor()); select(items[0]?.id);
	}

	return { create, remove };
}
