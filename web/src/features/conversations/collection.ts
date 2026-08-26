import type { Conversation, ConversationPage, Member, MemberPage } from "../../api/types";

export const MAX_CLIENT_CONVERSATIONS = 500;
export const MAX_CLIENT_MEMBERS = 500;

function uniqueBy<T>(items: T[], identifier: (item: T) => number, limit: number) {
	const unique = new Map<number, T>();
	for (const item of items) unique.set(identifier(item), item);
	return [...unique.values()].sort((left, right) => identifier(right) - identifier(left)).slice(0, limit);
}

export function appendConversationPage(existing: Conversation[], page: ConversationPage) {
	const items = uniqueBy([...existing, ...page.conversations], (item) => item.id, MAX_CLIENT_CONVERSATIONS);
	return {
		items,
		nextBeforeId: items.length >= MAX_CLIENT_CONVERSATIONS ? undefined : page.next_before_id,
	};
}

export function appendMemberPage(existing: Member[], page: MemberPage) {
	const items = uniqueBy([...existing, ...page.members], (item) => item.user_id, MAX_CLIENT_MEMBERS);
	return {
		items,
		nextBeforeId: items.length >= MAX_CLIENT_MEMBERS ? undefined : page.next_before_id,
	};
}
